package resource

import (
	"cmp"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/netip"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/servicediscovery"
	sdtypes "github.com/aws/aws-sdk-go-v2/service/servicediscovery/types"
	"github.com/pkg/errors"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/TouchBistro/buildit/client"
	"github.com/TouchBistro/buildit/util"
	"github.com/TouchBistro/goutils/color"
	log "github.com/sirupsen/logrus"
)

type SDDnsRecord struct {
	TTL    int64    `yaml:"ttl"`
	Type   string   `yaml:"type"`
	Values []string `yaml:"values"`
}

// SDService represents a virtual service in Cloud Map
type SDService struct {
	BaseResource `yaml:",inline"`
	Name          string            `yaml:"-"`
	DiscoveryName string            `yaml:"discoveryName"`
	Description   string            `yaml:"description"`
	Namespace     string            `yaml:"namespace"`
	RoutingPolicy string            `yaml:"routingPolicy"`
	Records       []SDDnsRecord     `yaml:"records"`
	TTL           *int64            `yaml:"ttl"` // Deprecated
	DependsOn     []Key             `yaml:"dependsOn"`
	GlobalTags    map[string]string `yaml:"-"`
	Tags          map[string]string `yaml:"tags"`
}

var sdServiceDnsRecordTypes = []string{
	"A",
	"AAAA",
	"CNAME",
	"SRV",
}
var sdServiceRoutingPolicies = []string{
	"MULTIVALUE",
	"WEIGHTED",
}

const (
	sdInstanceIDPrefix      = "buildit-static-"
	sdAttributeIPv4         = "AWS_INSTANCE_IPV4"
	sdAttributeIPv6         = "AWS_INSTANCE_IPV6"
	sdAttributeCNAME        = "AWS_INSTANCE_CNAME"
	sdSupportedSRVMessage   = "SRV records are not supported with values; use a dedicated resource for explicit SRV instance attributes"
	sdDestroyWaitTimeout    = 60 * time.Second
	sdDestroyWaitInterval   = 2 * time.Second
	sdOperationWaitTimeout  = 60 * time.Second
	sdOperationWaitInterval = 2 * time.Second
)

var dnsNamePattern = regexp.MustCompile(`(?i)^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)*\.?$`)

type sdStaticInstance struct {
	InstanceID   string
	RecordType   string
	AttributeKey string
	Value        string
}

// Key returns the unique key for the resource for this buildit context
func (s SDService) Key() Key {
	return NewKey(s.Context.ProviderName, s.Identifier())
}

// Identifier returns the unique ID
func (s SDService) Identifier() string {
	return s.Name
}

// Normalize will set any default values or sanitize/clean up any necessary fields.
func (s *SDService) Normalize(ctx context.Context) {
	//Merge globalTags to Service Discovery tags, if key is not already present
	//later we'll use s.Tags to add/update tags
	if s.Tags == nil {
		s.Tags = make(map[string]string)
	}

	if s.TTL != nil && len(s.Records) > 0 {
		log.Warnf("SD Service (%s) defines TTL which will be ignored due to dnsRecords configuration", s.Identifier())
	}

	if s.TTL != nil && len(s.Records) == 0 {
		s.Records = append(s.Records, SDDnsRecord{
			TTL:  *s.TTL,
			Type: "A",
		})
	}

	if len(s.Records) == 0 {
		s.Records = append(s.Records, SDDnsRecord{
			TTL:  0,
			Type: "A",
		})
	}
	hasCNAMERecord := false
	for k := range s.Records {
		s.Records[k].Type = strings.ToUpper(s.Records[k].Type)
		if s.Records[k].Type == "CNAME" {
			hasCNAMERecord = true
		}
	}

	if s.RoutingPolicy == "" {
		if hasCNAMERecord {
			s.RoutingPolicy = sdServiceRoutingPolicies[1]
		} else {
			s.RoutingPolicy = sdServiceRoutingPolicies[0]
		}
	}
	s.RoutingPolicy = strings.ToUpper(s.RoutingPolicy)

	ResourceTags(s.Tags).Merge(s.GlobalTags)
}

// Validate checks that the SDService has a valid configuration.
func (s SDService) Validate(ctx context.Context) error {
	var errMessages []string

	if s.DiscoveryName == "" {
		errMessages = append(errMessages, "no service discovery name specified")
	}

	for _, v := range s.Records {
		if v.TTL < 0 {
			errMessages = append(errMessages, fmt.Sprintf("%s ttl must be a valid, non-negative number", v.Type))
		}
		if !slices.Contains(sdServiceDnsRecordTypes, v.Type) {
			errMessages = append(errMessages, fmt.Sprintf("%s is not a valid DnsRecord type", v.Type))
		}
		if v.Type == "CNAME" && s.RoutingPolicy != "WEIGHTED" {
			errMessages = append(errMessages, "CNAME records require routingPolicy WEIGHTED")
		}
		for _, value := range v.Values {
			err := validateSDServiceRecordValue(v.Type, value)
			if err != nil {
				errMessages = append(errMessages, fmt.Sprintf("%s value %q is invalid: %v", v.Type, value, err))
			}
		}
	}

	if !slices.Contains(sdServiceRoutingPolicies, s.RoutingPolicy) {
		errMessages = append(errMessages, fmt.Sprintf("%s is not a valid RoutingPolicy", s.RoutingPolicy))
	}

	if errMessages == nil {
		return nil
	}

	return &ValidationError{
		ResourceIdentifier: s.Identifier(),
		ResourceType:       "SD Service",
		Messages:           errMessages,
	}
}

// Apply builds the CloudMap service
func (s SDService) Apply(ctx context.Context) error {
	log.Debugf("creating service discovery service %v", s.Identifier())

	diffs, err := s.Compare(ctx)
	if err != nil {
		return err
	}

	if diffs == nil {
		log.WithFields(log.Fields{
			"Name": s.Identifier(),
		}).Info("no updates required")
		return nil
	}

	//if diff found & existing resource exists...
	if diffs.AWSResource() != nil {
		log.WithField("Name", s.Identifier()).Info("service discovery service already exists, updating")
		for _, d := range diffs.Differences() {
			log.Debug(d)
		}
		err = s.applyDiffs(ctx, diffs)
		if err != nil {
			return errors.Wrapf(err, "failed to update service discovery service %v", s.Identifier())
		}
		return nil
	}

	return s.apply(ctx)
}

// Destroy deletes the existing service registry
func (s SDService) Destroy(ctx context.Context) error {

	sdClient := client.ServiceDiscovery(ctx, s.Context.ProviderName)
	log.WithField("Name", s.Name).Debug("checking if service discovery service exists")
	awsSvc, err := s.fetchExisting(ctx)
	if err != nil {
		return errors.Wrapf(err, "failed to check if service discovery service %s exists", s.Name)
	}
	if awsSvc == nil || awsSvc.Id == nil {
		log.WithField("Name", s.Name).Info("service discovery service does not exist, nothing to destroy")
		return nil
	}

	managedInstances, err := s.fetchManagedInstances(ctx, *awsSvc.Id)
	if err != nil {
		return errors.Wrapf(err, "failed to fetch static service discovery instances for %s", s.Name)
	}
	if err := s.syncManagedInstances(ctx, *awsSvc.Id, managedInstances, map[string]sdStaticInstance{}); err != nil {
		return errors.Wrapf(err, "failed to remove static service discovery values for %s", s.Name)
	}
	if err := s.waitForManagedInstancesCleared(ctx, *awsSvc.Id); err != nil {
		return errors.Wrapf(err, "failed waiting for static service discovery values to be removed for %s", s.Name)
	}

	log.WithField("Name", s.Name).Debug("destroying service discovery Service")
	_, err = sdClient.DeleteService(ctx, &servicediscovery.DeleteServiceInput{
		Id: awsSvc.Id,
	})
	if err != nil {
		return errors.Wrapf(err, "failed to delete service discovery service %s", s.Name)
	}

	log.WithFields(log.Fields{
		"Name": s.Name,
		"ARN":  aws.ToString(awsSvc.Arn),
	}).Info(color.Red("service discovery service destroyed"))
	return nil
}

type SDServiceDiff struct {
	BaseResourceDiff

	ttlDiff         bool
	descriptionDiff bool
	valuesDiff      bool

	tagsDiff bool
	tagDiff  util.TagDiffResult

	valuesToRegister   []sdStaticInstance
	valuesToDeregister []string
}

// Compare fetches the existing service discovery service, and if it exists,
// compares to this definition & returns any diffs
func (s SDService) Compare(ctx context.Context) (ResourceDiff, error) {
	existing, err := s.fetchExisting(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "error fetching aws resource for %v", s.Identifier())
	}

	diffs := &SDServiceDiff{}

	if existing == nil {
		diffs.Messages = append(diffs.Messages, "service discovery service does not exist")
		return diffs, nil
	}

	diffs.Resource = existing

	found := false

	if existing.Id == nil {
		return nil, errors.Errorf("existing service discovery service %v is missing service id", s.Identifier())
	}
	if existing.DnsConfig == nil {
		return nil, errors.Errorf("existing service discovery service %v has no dns config", s.Identifier())
	}

	// we cannot change routing type on sd
	if existing.DnsConfig.RoutingPolicy != sdtypes.RoutingPolicy(s.RoutingPolicy) {
		return nil, errors.New("service discovery routing policy cannot be modified")
	}

	// we cannot change records on sd
	if len(existing.DnsConfig.DnsRecords) != len(s.Records) {
		return nil, errors.New("service discovery records entries cannot be added or removed")
	}

	awsRecords := slices.Clone(existing.DnsConfig.DnsRecords)
	slices.SortFunc(awsRecords, func(a, b sdtypes.DnsRecord) int {
		return cmp.Compare(a.Type, b.Type)
	})

	desiredRecords := slices.Clone(s.Records)
	slices.SortFunc(desiredRecords, func(a, b SDDnsRecord) int {
		return cmp.Compare(a.Type, b.Type)
	})

	for k := range awsRecords {
		if string(awsRecords[k].Type) != desiredRecords[k].Type {
			return nil, errors.New("service discovery records types cannot be modified")
		}
		if awsRecords[k].TTL == nil {
			return nil, errors.Errorf("existing service discovery %s record has no ttl", desiredRecords[k].Type)
		}
		if *awsRecords[k].TTL != desiredRecords[k].TTL {
			found = true
			diffs.ttlDiff = true
			diffs.Messages = append(diffs.Messages, fmt.Sprintf("service discovery %s ttl is not the same", desiredRecords[k].Type))
		}
	}

	// description
	if s.Description != aws.ToString(existing.Description) {
		found = true
		diffs.descriptionDiff = true
		diffs.Messages = append(diffs.Messages, "service discovery service description is not the same")
	}

	desiredInstances, err := s.desiredStaticInstances()
	if err != nil {
		return nil, errors.Wrapf(err, "invalid static values definition for service discovery service %v", s.Identifier())
	}
	existingInstances, err := s.fetchManagedInstances(ctx, *existing.Id)
	if err != nil {
		return nil, errors.Wrapf(err, "error fetching static values for service discovery service %v", s.Identifier())
	}

	for id, desired := range desiredInstances {
		existingInstance, ok := existingInstances[id]
		if !ok || existingInstance.Attributes[desired.AttributeKey] != desired.Value {
			diffs.valuesToRegister = append(diffs.valuesToRegister, desired)
		}
	}
	for id := range existingInstances {
		if _, ok := desiredInstances[id]; !ok {
			diffs.valuesToDeregister = append(diffs.valuesToDeregister, id)
		}
	}
	sort.Slice(diffs.valuesToRegister, func(i, j int) bool {
		return diffs.valuesToRegister[i].InstanceID < diffs.valuesToRegister[j].InstanceID
	})
	sort.Strings(diffs.valuesToDeregister)
	if len(diffs.valuesToRegister) > 0 || len(diffs.valuesToDeregister) > 0 {
		found = true
		diffs.valuesDiff = true
		if len(diffs.valuesToRegister) > 0 {
			diffs.Messages = append(diffs.Messages, fmt.Sprintf("%v static values to be registered", len(diffs.valuesToRegister)))
		}
		if len(diffs.valuesToDeregister) > 0 {
			diffs.Messages = append(diffs.Messages, fmt.Sprintf("%v static values to be deregistered", len(diffs.valuesToDeregister)))
		}
	}

	// tags
	if existing.Arn == nil {
		return nil, errors.Errorf("existing service discovery service %v is missing arn", s.Identifier())
	}
	awsTags, err := awsw.NewServiceDiscovery(ctx, s.Context.ProviderName).GetResourceTags(ctx, *existing.Arn)
	if err != nil {
		return nil, errors.Wrapf(err, "error fetching tags for service discovery service %v", s.Identifier())
	}
	if tagDiff := TagDiffForContext(ctx, awsTags, s.Tags); tagDiff.HasChanges() {
		found = true
		diffs.tagsDiff = true
		diffs.tagDiff = tagDiff
		diffs.Messages = append(diffs.Messages, TagDiffSummary(awsTags, diffs.tagDiff)...)
	}

	if !found {
		return nil, nil
	}

	return diffs, nil
}

// apply provisions a new service discovery service
func (s SDService) apply(ctx context.Context) error {

	namespaceId, err := awsw.NewServiceDiscovery(ctx, s.Context.ProviderName).NameSpaceIdFromName(ctx, s.Namespace)
	if err != nil {
		return errors.Wrap(err, "error looking up service discovery namespace")
	}

	sdClient := client.ServiceDiscovery(ctx, s.Context.ProviderName)

	var tags []sdtypes.Tag
	for k, v := range s.Tags {
		tags = append(tags, sdtypes.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}

	log.WithField("Name", s.Name).Debug("creating service discovery service")

	var dnsRecords []sdtypes.DnsRecord
	for _, v := range s.Records {
		dnsRecords = append(dnsRecords, sdtypes.DnsRecord{
			Type: sdtypes.RecordType(v.Type),
			TTL:  &v.TTL,
		})
	}
	out, err := sdClient.CreateService(ctx, &servicediscovery.CreateServiceInput{
		Name:        aws.String(s.DiscoveryName),
		Description: aws.String(s.Description),
		DnsConfig: &sdtypes.DnsConfig{
			RoutingPolicy: sdtypes.RoutingPolicy(s.RoutingPolicy),
			DnsRecords:    dnsRecords,
		},
		NamespaceId: namespaceId,
		HealthCheckCustomConfig: &sdtypes.HealthCheckCustomConfig{
			FailureThreshold: aws.Int32(1), //TODO: deprecated and always 1. need to review and update later
		},
		Tags: tags,
	})
	if err != nil {
		return errors.Wrap(err, "failed to create discovery service record")
	}

	serviceID := ""
	if out.Service != nil && out.Service.Id != nil {
		serviceID = *out.Service.Id
	} else {
		existing, err := s.fetchExisting(ctx)
		if err != nil {
			return errors.Wrapf(err, "service discovery service created but failed to retrieve service details for %v", s.Identifier())
		}
		if existing == nil || existing.Id == nil {
			return errors.Errorf("service discovery service created but failed to determine service id for %v", s.Identifier())
		}
		serviceID = *existing.Id
	}

	desiredInstances, err := s.desiredStaticInstances()
	if err != nil {
		return errors.Wrapf(err, "failed to process static values for service discovery service %v", s.Identifier())
	}
	if err := s.syncManagedInstances(ctx, serviceID, map[string]sdtypes.InstanceSummary{}, desiredInstances); err != nil {
		return errors.Wrapf(err, "failed to register static values for service discovery service %v", s.Identifier())
	}

	log.WithFields(log.Fields{
		"Name": s.Name,
	}).Info(color.Green("service discovery created"))

	return nil
}

// applyDiffs applies the supplied diffs to the existing service discovery service
func (s SDService) applyDiffs(ctx context.Context, diffs ResourceDiff) error {

	if diffs == nil {
		log.WithFields(log.Fields{
			"Name": s.Identifier(),
		}).Info("no updates required for service discovery service")
		return nil
	}

	svcDiffs, ok := diffs.(*SDServiceDiff)
	if !ok {
		return errors.New("invalid diff type supplied")
	}

	// fetch existing service discovery service
	existing, ok := svcDiffs.Resource.(*sdtypes.ServiceSummary)
	if !ok {
		return errors.Errorf("cannot retrieve existing service discovery service")
	}
	if existing.Id == nil {
		return errors.Errorf("cannot update service discovery service %v with missing service id", s.Identifier())
	}

	if svcDiffs.descriptionDiff || svcDiffs.ttlDiff {
		var dnsRecords []sdtypes.DnsRecord
		for _, v := range s.Records {
			dnsRecords = append(dnsRecords, sdtypes.DnsRecord{
				Type: sdtypes.RecordType(v.Type),
				TTL:  &v.TTL,
			})
		}

		_, err := client.ServiceDiscovery(ctx, s.Context.ProviderName).UpdateService(ctx, &servicediscovery.UpdateServiceInput{
			Id: existing.Id,
			Service: &sdtypes.ServiceChange{
				Description: aws.String(s.Description),
				DnsConfig: &sdtypes.DnsConfigChange{
					DnsRecords: dnsRecords,
				},
			},
		})

		if err != nil {
			return errors.Wrapf(err, "error updating service discovery service %v", s.Identifier())
		}
	}

	// tags
	var err error
	if svcDiffs.tagsDiff {
		if existing.Arn == nil {
			return errors.Errorf("cannot update service discovery service %v tags with missing arn", s.Identifier())
		}
		upserts := svcDiffs.tagDiff.Upserts()
		if len(upserts) > 0 {
			err = awsw.NewServiceDiscovery(ctx, s.Context.ProviderName).AddResourceTags(ctx, *existing.Arn, upserts)
			if err != nil {
				return errors.Wrapf(err, "error updating service discovery service tags for %v", s.Identifier())
			}
		}
		if len(svcDiffs.tagDiff.Deleted) > 0 {
			err = awsw.NewServiceDiscovery(ctx, s.Context.ProviderName).DeleteResourceTags(ctx, *existing.Arn, svcDiffs.tagDiff.Deleted)
			if err != nil {
				return errors.Wrapf(err, "error deleting service discovery service tags for %v", s.Identifier())
			}
		}
	}

	if svcDiffs.valuesDiff {
		currentInstances, err := s.fetchManagedInstances(ctx, *existing.Id)
		if err != nil {
			return errors.Wrapf(err, "error fetching existing static values for %v", s.Identifier())
		}
		desiredInstances, err := s.desiredStaticInstances()
		if err != nil {
			return errors.Wrapf(err, "error processing desired static values for %v", s.Identifier())
		}
		if err := s.syncManagedInstances(ctx, *existing.Id, currentInstances, desiredInstances); err != nil {
			return errors.Wrapf(err, "error syncing static values for %v", s.Identifier())
		}
	}

	log.WithFields(log.Fields{
		"Name":      s.Identifier(),
		"Namespace": s.Namespace,
	}).Info(color.Yellow("service discovery service updated"))

	return nil
}

// fetchExisting fetches the existing object from AWS
func (s SDService) fetchExisting(ctx context.Context) (*sdtypes.ServiceSummary, error) {

	namespaceId, err := awsw.NewServiceDiscovery(ctx, s.Context.ProviderName).NameSpaceIdFromName(ctx, s.Namespace)
	if err != nil {
		return nil, errors.Wrap(err, "error looking up service discovery namespace")
	}

	done := false
	var token *string
	sdClient := client.ServiceDiscovery(ctx, s.Context.ProviderName)

	for !done {
		if err := ctx.Err(); err != nil {
			return nil, errors.Wrap(err, "context cancelled while listing service discovery services")
		}

		out, err := sdClient.ListServices(ctx, &servicediscovery.ListServicesInput{
			NextToken: token,
			Filters: []sdtypes.ServiceFilter{
				{
					Condition: sdtypes.FilterConditionEq,
					Name:      sdtypes.ServiceFilterNameNamespaceId,
					Values:    []string{*namespaceId},
				},
			},
		})
		if err != nil {
			return nil, errors.Wrap(err, "failed to look up SD services")
		}

		for _, svc := range out.Services {
			if svc.Name != nil && *svc.Name == s.DiscoveryName {
				return &svc, nil
			}
		}

		token = out.NextToken
		done = token == nil
	}

	return nil, nil
}

func (s SDService) desiredStaticInstances() (map[string]sdStaticInstance, error) {
	instances := make(map[string]sdStaticInstance)

	for _, record := range s.Records {
		if len(record.Values) == 0 {
			continue
		}

		attributeKey, err := sdRecordAttributeKey(record.Type)
		if err != nil {
			return nil, err
		}

		for _, value := range record.Values {
			instanceID := sdStaticInstanceID(record.Type, value)
			instances[instanceID] = sdStaticInstance{
				InstanceID:   instanceID,
				RecordType:   record.Type,
				AttributeKey: attributeKey,
				Value:        value,
			}
		}
	}

	return instances, nil
}

func (s SDService) fetchManagedInstances(ctx context.Context, serviceID string) (map[string]sdtypes.InstanceSummary, error) {
	instances := make(map[string]sdtypes.InstanceSummary)
	var nextToken *string
	sdClient := client.ServiceDiscovery(ctx, s.Context.ProviderName)

	for {
		if err := ctx.Err(); err != nil {
			return nil, errors.Wrap(err, "context cancelled while listing service discovery instances")
		}

		out, err := sdClient.ListInstances(ctx, &servicediscovery.ListInstancesInput{
			ServiceId: aws.String(serviceID),
			NextToken: nextToken,
		})
		if err != nil {
			return nil, err
		}

		for _, instance := range out.Instances {
			if instance.Id == nil || !strings.HasPrefix(*instance.Id, sdInstanceIDPrefix) {
				continue
			}
			instances[*instance.Id] = instance
		}

		if out.NextToken == nil {
			break
		}
		nextToken = out.NextToken
	}

	return instances, nil
}

func (s SDService) syncManagedInstances(ctx context.Context, serviceID string, current map[string]sdtypes.InstanceSummary, desired map[string]sdStaticInstance) error {
	sdClient := client.ServiceDiscovery(ctx, s.Context.ProviderName)
	serviceName := s.Name
	if serviceName == "" {
		serviceName = serviceID
	}

	desiredIDs := make([]string, 0, len(desired))
	for id := range desired {
		desiredIDs = append(desiredIDs, id)
	}
	sort.Strings(desiredIDs)

	for _, id := range desiredIDs {
		if err := ctx.Err(); err != nil {
			return errors.Wrap(err, "context cancelled while registering static service discovery values")
		}
		static := desired[id]

		existing, ok := current[id]
		if ok && existing.Attributes[static.AttributeKey] == static.Value {
			continue
		}

		out, err := sdClient.RegisterInstance(ctx, &servicediscovery.RegisterInstanceInput{
			ServiceId:  aws.String(serviceID),
			InstanceId: aws.String(id),
			Attributes: map[string]string{
				static.AttributeKey: static.Value,
			},
		})
		if err != nil {
			return errors.Wrapf(err, "failed to register static value %q for %s", static.Value, static.RecordType)
		}
		if out.OperationId == nil {
			return errors.Errorf("register static value operation id missing for value %q", static.Value)
		}
		if err := s.waitForOperation(ctx, *out.OperationId); err != nil {
			return errors.Wrapf(err, "failed waiting for registration of static value %q", static.Value)
		}
		log.Info(color.Green(fmt.Sprintf("instance registered %s to service discovery %s", static.Value, serviceName)))
	}

	currentIDs := make([]string, 0, len(current))
	for id := range current {
		currentIDs = append(currentIDs, id)
	}
	sort.Strings(currentIDs)

	for _, id := range currentIDs {
		if err := ctx.Err(); err != nil {
			return errors.Wrap(err, "context cancelled while deregistering static service discovery values")
		}
		if _, ok := desired[id]; ok {
			continue
		}

		out, err := sdClient.DeregisterInstance(ctx, &servicediscovery.DeregisterInstanceInput{
			ServiceId:  aws.String(serviceID),
			InstanceId: aws.String(id),
		})
		if err != nil {
			return errors.Wrapf(err, "failed to deregister static value instance %s", id)
		}
		value := sdInstanceValue(current[id].Attributes, id)
		if out.OperationId == nil {
			return errors.Errorf("deregister static value operation id missing for value %q", value)
		}
		if err := s.waitForOperation(ctx, *out.OperationId); err != nil {
			return errors.Wrapf(err, "failed waiting for deregistration of static value %q", value)
		}
		log.Info(color.Red(fmt.Sprintf("instance deregistered %s from service discovery %s", value, serviceName)))
	}

	return nil
}

func (s SDService) waitForOperation(ctx context.Context, operationID string) error {
	sdClient := client.ServiceDiscovery(ctx, s.Context.ProviderName)
	deadline := time.Now().Add(sdOperationWaitTimeout)

	for {
		if err := ctx.Err(); err != nil {
			return errors.Wrap(err, "context cancelled while waiting for service discovery operation")
		}

		out, err := sdClient.GetOperation(ctx, &servicediscovery.GetOperationInput{
			OperationId: aws.String(operationID),
		})
		if err != nil {
			return errors.Wrapf(err, "failed to fetch service discovery operation %s", operationID)
		}
		if out.Operation == nil {
			return errors.Errorf("service discovery operation %s returned no operation details", operationID)
		}

		switch out.Operation.Status {
		case sdtypes.OperationStatusSuccess:
			return nil
		case sdtypes.OperationStatusFail:
			return errors.Errorf("service discovery operation %s failed: %s", operationID, aws.ToString(out.Operation.ErrorMessage))
		}

		if time.Now().After(deadline) {
			return errors.Errorf("timed out waiting for service discovery operation %s to complete", operationID)
		}

		if err := util.SleepWithContext(ctx, sdOperationWaitInterval); err != nil {
			return errors.Wrap(err, "context cancelled while waiting for service discovery operation")
		}
	}
}

func sdInstanceValue(attributes map[string]string, fallback string) string {
	if value, ok := attributes[sdAttributeCNAME]; ok && value != "" {
		return value
	}
	if value, ok := attributes[sdAttributeIPv4]; ok && value != "" {
		return value
	}
	if value, ok := attributes[sdAttributeIPv6]; ok && value != "" {
		return value
	}
	return fallback
}

func (s SDService) waitForManagedInstancesCleared(ctx context.Context, serviceID string) error {
	deadline := time.Now().Add(sdDestroyWaitTimeout)

	for {
		if err := ctx.Err(); err != nil {
			return errors.Wrap(err, "context cancelled while waiting for service discovery instance deregistration")
		}

		instances, err := s.fetchManagedInstances(ctx, serviceID)
		if err != nil {
			return errors.Wrap(err, "failed to list service discovery instances while waiting for deregistration")
		}
		if len(instances) == 0 {
			return nil
		}

		if time.Now().After(deadline) {
			return errors.Errorf("timed out waiting for %d service discovery instances to be deregistered", len(instances))
		}

		if err := util.SleepWithContext(ctx, sdDestroyWaitInterval); err != nil {
			return errors.Wrap(err, "context cancelled while waiting for service discovery instance deregistration")
		}
	}
}

func validateSDServiceRecordValue(recordType string, value string) error {
	switch recordType {
	case "A":
		addr, err := netip.ParseAddr(value)
		if err != nil || !addr.Is4() {
			return errors.New("must be a valid IPv4 address")
		}
	case "AAAA":
		addr, err := netip.ParseAddr(value)
		if err != nil || !addr.Is6() {
			return errors.New("must be a valid IPv6 address")
		}
	case "CNAME":
		if !isValidDNSName(value) {
			return errors.New("must be a valid DNS name")
		}
	case "SRV":
		return errors.New(sdSupportedSRVMessage)
	default:
		return errors.Errorf("unsupported record type %q", recordType)
	}
	return nil
}

func sdRecordAttributeKey(recordType string) (string, error) {
	switch recordType {
	case "A":
		return sdAttributeIPv4, nil
	case "AAAA":
		return sdAttributeIPv6, nil
	case "CNAME":
		return sdAttributeCNAME, nil
	case "SRV":
		return "", errors.New(sdSupportedSRVMessage)
	default:
		return "", errors.Errorf("unsupported record type %q", recordType)
	}
}

func sdStaticInstanceID(recordType string, value string) string {
	hash := sha1.Sum([]byte(strings.ToUpper(recordType) + "|" + value))
	return sdInstanceIDPrefix + strings.ToLower(recordType) + "-" + hex.EncodeToString(hash[:8])
}

func isValidDNSName(value string) bool {
	if len(value) == 0 || len(value) > 253 {
		return false
	}
	return dnsNamePattern.MatchString(value)
}
