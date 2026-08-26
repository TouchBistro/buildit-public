package resource

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/TouchBistro/buildit/client"
	"github.com/TouchBistro/buildit/util"
	"github.com/TouchBistro/goutils/color"
	"github.com/aws/aws-sdk-go-v2/aws"
	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

const (
	LoadBalancerSchemeInternal       = string(elbv2types.LoadBalancerSchemeEnumInternal)       //"internal"
	LoadBalancerSchemeInternetFacing = string(elbv2types.LoadBalancerSchemeEnumInternetFacing) //"internet-facing"
)

const (
	LoadBalancerTypeApplication = string(elbv2types.LoadBalancerTypeEnumApplication) //"application"
	LoadBalancerTypeNetwork     = string(elbv2types.LoadBalancerTypeEnumNetwork)     //"network"
	LoadBalancerTypeGateway     = string(elbv2types.LoadBalancerTypeEnumGateway)     //"gateway"
)

// LoadBalancer resource,
// Currently only Application Load Balancers (ALB) & IPV4 address type are supported
type LoadBalancer struct {
	BaseResource `yaml:",inline"`
	Name               string            `yaml:"-"`
	SubnetNames        []string          `yaml:"subnetNames"`
	SecurityGroupNames []string          `yaml:"securityGroupNames"`
	Scheme             *string           `yaml:"scheme"` //internal, internet-facing (default)
	Type               *string           `yaml:"type"`
	DependsOn          []Key             `yaml:"dependsOn"`
	Listeners          []*LBListener     `yaml:"listeners"` //default listeners, made in natural order
	Attributes         map[string]string `yaml:"attributes"`
	GlobalTags         map[string]string `yaml:"-"`
	Tags               map[string]string `yaml:"tags"`
}

// Key returns the unique key for the resource for this buildit context
func (lb LoadBalancer) Key() Key {
	return NewKey(lb.Context.ProviderName, lb.Identifier())
}

// Identifier returns the id for the resource
func (lb LoadBalancer) Identifier() string {
	return lb.Name
}

// Normalize will set any default values or sanitize/clean up any necessary fields.
func (lb *LoadBalancer) Normalize(ctx context.Context) {

	// scheme
	if lb.Scheme == nil {
		lb.Scheme = util.ToStringPtr(string(elbv2types.LoadBalancerSchemeEnumInternetFacing))
	}
	lb.Scheme = util.ToStringPtr(strings.ToLower(*lb.Scheme))

	// type
	if lb.Type == nil {
		lb.Type = util.ToStringPtr(string(elbv2types.LoadBalancerTypeEnumApplication))
	}
	lb.Type = util.ToStringPtr(strings.ToLower(*lb.Type))

	// Merge globalTags to Load Balancer tags, if key is not already present
	// later we'll use lb.Tags to add/update tags
	if lb.Tags == nil {
		lb.Tags = make(map[string]string)
	}
	ResourceTags(lb.Tags).Merge(lb.GlobalTags)

	// load balancer attributes; start with defaults and override any specified in yaml
	var defaultAttributes = lb.getDefaultAttributes()

	if lb.Attributes == nil {
		lb.Attributes = make(map[string]string)
	}
	configAttrs := lb.Attributes
	for k, v := range configAttrs {
		defaultAttributes[k] = v
	}
	lb.Attributes = defaultAttributes

	// normalize the listeners
	for _, listener := range lb.Listeners {
		listener.Normalize(ctx, lb.Context)
	}
}

// Validate the input
func (lb LoadBalancer) Validate(ctx context.Context) error {

	var errMessages []string

	switch *lb.Scheme {
	case LoadBalancerSchemeInternal, LoadBalancerSchemeInternetFacing:
	default:
		msg := fmt.Sprintf("invalid scheme, allowed values are %v, %v", LoadBalancerSchemeInternal, LoadBalancerSchemeInternetFacing)
		errMessages = append(errMessages, msg)
	}

	switch *lb.Type {
	case LoadBalancerTypeNetwork, LoadBalancerTypeApplication:
	default:
		msg := fmt.Sprintf("invalid type, allowed values are %v, %v", LoadBalancerTypeApplication, LoadBalancerTypeNetwork)
		errMessages = append(errMessages, msg)
	}

	if len(lb.Identifier()) > 32 {
		msg := fmt.Sprintf("name cannot be longer than 32 characters, current length: %d", len(lb.Identifier()))
		errMessages = append(errMessages, msg)
	}

	//check if at least 2 subnets are specified
	if len(lb.SubnetNames) < 2 {
		errMessages = append(errMessages, "at least 2 subnets must be specified for an ALB")
	}

	subnets, err := awsw.NewEC2(ctx, lb.Context.ProviderName).SubnetByNames(ctx, lb.SubnetNames)
	if err != nil {
		errMessages = append(errMessages, "error looking up subnets by name")
	}

	//check if all subnets are valid
	if len(subnets) != len(lb.SubnetNames) {
		errMessages = append(errMessages, "cannot find all subnets")
	}

	//check if at least 2 AZs are used
	azMap := make(map[string]string)
	for _, sub := range subnets {
		az := *sub.AvailabilityZoneId
		if _, ok := azMap[az]; ok {
			errMessages = append(errMessages, "only 1 subnet per availability zone must be specified")
		}
		azMap[az] = az
	}

	if len(azMap) < 2 {
		errMessages = append(errMessages, "specified subnets for an ALB must lie in at least 2 availability zones")
	}

	if len(lb.Listeners) == 0 {
		errMessages = append(errMessages, "at least 1 listener must be specified")
	}

	//security groups
	if len(lb.SecurityGroupNames) == 0 {
		errMessages = append(errMessages, "at least 1 security group must be specified")
	}

	//check listener ports
	portMap := make(map[int32]int32)
	for _, lis := range lb.Listeners {
		p := lis.Port
		if _, ok := portMap[p]; ok {
			errMessages = append(errMessages, "cannot specify duplicate listner ports")
		} else {
			if p < 1 || p > 65535 {
				errMessages = append(errMessages, "port must be in the range from 1 to 65535")
			}
			portMap[p] = p
		}

		err := lis.Validate(ctx)
		if err != nil {
			t, ok := err.(*ValidationError)
			if ok {
				errMessages = append(errMessages, t.Messages...)
			}
		}
	}

	if len(errMessages) == 0 {
		return nil
	}

	return &ValidationError{
		ResourceIdentifier: lb.Identifier(),
		ResourceType:       "Load Balancer",
		Messages:           errMessages,
	}
}

// Apply makes or updates the resource
func (lb LoadBalancer) Apply(ctx context.Context) error {

	log.Debugf("creating load balancer %v", lb.Identifier())

	diffs, err := lb.Compare(ctx)
	if err != nil {
		return err
	}

	if diffs == nil {
		log.WithFields(log.Fields{
			"Name": lb.Identifier(),
		}).Info("no updates required")
		return nil
	}

	//if diff found & existing resource exists...
	if diffs.AWSResource() != nil {
		log.WithField("Name", lb.Identifier()).Info("load balancer already exists, updating")
		for _, d := range diffs.Differences() {
			log.Debug(d)
		}
		err = lb.applyDiffs(ctx, diffs)
		if err != nil {
			return errors.Wrapf(err, "failed to update load balancer %v", lb.Identifier())
		}
		return nil
	}

	return lb.apply(ctx)
}

// Destroy deletes a load balancer, it's listener & rules
func (lb LoadBalancer) Destroy(ctx context.Context) error {

	log.Debugf("destroying load balancer %v", lb.Identifier())

	existing, err := lb.fetchExisting(ctx)
	if err != nil {
		return errors.Wrapf(err, "error finding load balancer %s", lb.Identifier())
	}

	if existing == nil {
		log.WithFields(log.Fields{
			"Name": lb.Identifier(),
		}).Info("load balancer does not exist, nothing to destroy, skippping ")
		return nil
	}

	elbClient := client.ELB(ctx, lb.Context.ProviderName)

	//delete listeners to avoid issues with dependencies.
	listeners, err := elbClient.DescribeListeners(ctx, &elbv2.DescribeListenersInput{
		LoadBalancerArn: existing.LoadBalancerArn,
	})

	if err != nil {
		return errors.Wrapf(err, "error describing load balancer listeners for %v", lb.Identifier())
	}

	for _, listener := range listeners.Listeners {

		_, err = elbClient.DeleteListener(ctx, &elbv2.DeleteListenerInput{
			ListenerArn: listener.ListenerArn,
		})

		if err != nil {
			return errors.Wrapf(err, "error deleting load balancer listener for %v", lb.Identifier())
		}
	}

	log.Debugf("deleted all listeners for load balancer %v", lb.Identifier())

	//now delete the load balancer
	_, err = elbClient.DeleteLoadBalancer(ctx, &elbv2.DeleteLoadBalancerInput{
		LoadBalancerArn: aws.String(*existing.LoadBalancerArn),
	})

	if err != nil {
		return errors.Wrapf(err, "error deleting load balancer %s", lb.Identifier())
	}

	log.WithFields(log.Fields{
		"Name": lb.Identifier(),
	}).Infof("waiting for load balancer to be deleted")

	waiter := elbv2.NewLoadBalancersDeletedWaiter(elbClient, func(opts *elbv2.LoadBalancersDeletedWaiterOptions) {
		opts.MinDelay = 15 * time.Second
		opts.MaxDelay = 120 * time.Second
	})

	//wait for max 15m, with 15-120s check delays...
	if err = waiter.Wait(ctx, &elbv2.DescribeLoadBalancersInput{
		LoadBalancerArns: []string{*existing.LoadBalancerArn},
	}, 10*time.Minute); err != nil {
		return errors.Wrap(err, "could not verify load balancer status, or it failed to provision")
	}

	log.WithFields(log.Fields{
		"Name": lb.Identifier(),
	}).Info(color.Red("load balancer destroyed"))

	return nil
}

// LoadBalancerDiff
type LoadBalancerDiff struct {
	BaseResourceDiff

	loadbalancerArn    string
	schemeDiff         bool
	typeDiff           bool
	subnetsDiff        bool
	subnetIds          []string
	securityGroupsDiff bool
	securityGroupIds   []string
	attributesDiffs    bool
	attributesToAdd    map[string]string
	listenerDiffs      bool
	listenersToAdd     map[string]LBListener
	listenersToDelete  map[string]elbv2types.Listener
	listenersToUpdate  map[*LBListener]*LoadBalancerListenerDiff
	tagsDiff           bool
	tagDiff            util.TagDiffResult
}

// Compare fetches the existing load balancer, and if it exists
// checks if this resource is equal to the corresponding AWS LoadBalancer
func (lb LoadBalancer) Compare(ctx context.Context) (ResourceDiff, error) {

	existing, err := lb.fetchExisting(ctx)
	if err != nil {
		return nil, err
	}

	diffs := &LoadBalancerDiff{}
	if existing == nil { //not found
		diffs.Messages = append(diffs.Messages, "load balancer does not exist")
		return diffs, nil
	}

	diffs.Resource = &existing

	diff := false
	elbClient := client.ELB(ctx, lb.Context.ProviderName)
	diffs.loadbalancerArn = *existing.LoadBalancerArn

	// scheme
	if existing.Scheme != elbv2types.LoadBalancerSchemeEnum(*lb.Scheme) {
		diff = true
		diffs.schemeDiff = true
		diffs.Messages = append(diffs.Messages, "load balancer schema is different")
	}

	// type
	if existing.Type != elbv2types.LoadBalancerTypeEnum(*lb.Type) {
		diff = true
		diffs.typeDiff = true
		diffs.Messages = append(diffs.Messages, "load balancer type is different")
	}

	// subnet Ids
	subnetIds, err := awsw.NewEC2(ctx, lb.Context.ProviderName).SubnetIdsByNames(ctx, lb.SubnetNames)
	if err != nil {
		return nil, errors.Wrap(err, "failed listing security group ids for lb")
	}

	existingSubnetIds := make([]*string, 0)
	for _, az := range existing.AvailabilityZones {
		existingSubnetIds = append(existingSubnetIds, az.SubnetId)
	}

	diffSubnets := util.DiffStringPtrSlices(existingSubnetIds, aws.StringSlice(subnetIds))
	if diffSubnets {
		diff = true
		diffs.subnetsDiff = true
		diffs.subnetIds = subnetIds
	}

	// security groups
	existingSecurityGroupIds := existing.SecurityGroups
	securityGroupIds, err := awsw.NewEC2(ctx, lb.Context.ProviderName).SecurityGroupIdsByNames(ctx, nil, lb.SecurityGroupNames)
	if err != nil {
		return nil, errors.Wrap(err, "error listing security group ids")
	}

	diffSgs := util.DiffStringSlices(existingSecurityGroupIds, securityGroupIds)
	if diffSgs {
		diff = true
		diffs.securityGroupsDiff = true
		diffs.securityGroupIds = securityGroupIds
	}

	//Attributes
	attr, err := elbClient.DescribeLoadBalancerAttributes(ctx, &elbv2.DescribeLoadBalancerAttributesInput{
		LoadBalancerArn: existing.LoadBalancerArn,
	})
	if err != nil {
		return nil, errors.Wrap(err, "error fetching load balancer attributes")
	}

	existingAttrs := attr.Attributes
	existingAttrsHash := make(map[string]string)

	for _, attr := range existingAttrs {
		existingAttrsHash[*attr.Key] = *attr.Value
	}

	diffs.attributesToAdd = loadBalancerAttributeUpserts(existingAttrsHash, lb.Attributes)
	if len(diffs.attributesToAdd) > 0 {
		diff = true
		diffs.attributesDiffs = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("%v attributes to be updated for load balancer %v - %v", len(diffs.attributesToAdd), lb.Identifier(), diffs.attributesToAdd))
	}

	//listeners
	// The strategy to update listeners is as follows:
	// first we do shallow diff between existing & new listeners, this diff only looks at
	// protocol & port. The rules & order for the update are:
	//
	//  A - the listeners that are not found in the new list, will be deleted.
	//  B - the matching listeners in both lists will be diff'ed for default action & listener
	//      rules and updated as necessary
	//  C - the listeners that are not in existing list, will be added.
	//
	respListener, err := elbClient.DescribeListeners(ctx, &elbv2.DescribeListenersInput{
		LoadBalancerArn: existing.LoadBalancerArn,
	})

	if err != nil {
		return nil, errors.Wrap(err, "error fetching listeners for the load balancer")
	}

	del := make(map[string]elbv2types.Listener) //list of listeners to delete
	add := make(map[string]LBListener)          //list of listeners to add
	upd := make(map[*LBListener]*LoadBalancerListenerDiff)
	updKeys := make([]string, 0)

	for _, l := range respListener.Listeners {
		k := fmt.Sprintf("%v:%v", strings.ToUpper(string(l.Protocol)), *l.Port)
		del[k] = l
	}
	for _, l := range lb.Listeners {
		k := fmt.Sprintf("%v:%v", strings.ToUpper(l.Protocol), l.Port)
		add[k] = *l
	}
	for k, old := range del {
		if new, ok := add[k]; ok {
			newRef := &new
			// if a listener already exists, we check if it's the same
			new.LoadBalancerARN = *old.LoadBalancerArn
			new.LoadBalancerName = lb.Name
			new.GlobalTags = InheritedTags(lb.Tags)
			diff, err := new.equals(ctx, lb.Context, &old)
			if err != nil {
				return nil, err
			}
			if diff != nil {
				listenerDiff, ok := diff.(*LoadBalancerListenerDiff)
				if !ok {
					return nil, errors.Errorf("unexpected listener diff type %T", diff)
				}
				//add the listeners diff msgs to this
				diffs.Messages = append(diffs.Messages, formatListenerDiffMessages(k, listenerDiff.Differences())...)
				upd[newRef] = listenerDiff
				updKeys = append(updKeys, k)
			}
			delete(del, k)
			delete(add, k)
		}
	}

	diffs.listenerDiffs = len(del) > 0 || len(add) > 0 || len(upd) > 0
	if diffs.listenerDiffs {
		diff = true
		diffs.listenersToAdd = add
		diffs.listenersToDelete = del
		diffs.listenersToUpdate = upd
		if len(add) > 0 {
			keys := sortedListenerKeys(add)
			diffs.Messages = append(diffs.Messages, fmt.Sprintf("%v listener(s) to be added to load balancer %v: %v", len(add), lb.Identifier(), strings.Join(keys, ", ")))
		}
		if len(upd) > 0 {
			slices.Sort(updKeys)
			diffs.Messages = append(diffs.Messages, fmt.Sprintf("%v listener(s) to be updated to load balancer %v: %v", len(upd), lb.Identifier(), strings.Join(updKeys, ", ")))
		}
		if len(del) > 0 {
			keys := sortedListenerKeys(del)
			diffs.Messages = append(diffs.Messages, fmt.Sprintf("%v listener(s) to be removed from load balancer %v: %v", len(del), lb.Identifier(), strings.Join(keys, ", ")))
		}
	}

	// tags
	awsTags, err := awsw.NewELB(ctx, lb.Context.ProviderName).GetResourceTags(ctx, *existing.LoadBalancerArn)
	if err != nil {
		return nil, err
	}
	if tagDiff := TagDiffForContext(ctx, awsTags, lb.Tags); tagDiff.HasChanges() {
		diff = true
		diffs.tagsDiff = true
		diffs.tagDiff = tagDiff
		diffs.Messages = append(diffs.Messages, TagDiffSummary(awsTags, diffs.tagDiff)...)
	}

	if !diff {
		return nil, nil
	}

	return diffs, nil
}

// fetchExisting checks if the load balancer already exists
func (lb LoadBalancer) fetchExisting(ctx context.Context) (*elbv2types.LoadBalancer, error) {

	elbv2Client := client.ELB(ctx, lb.Context.ProviderName)
	resp, err := elbv2Client.DescribeLoadBalancers(ctx, &elbv2.DescribeLoadBalancersInput{
		Names: []string{lb.Identifier()},
	})

	if err != nil {
		var lbnfe *elbv2types.LoadBalancerNotFoundException
		if errors.As(err, &lbnfe) {
			return nil, nil
		}
		return nil, errors.Wrapf(err, "error looking up Load Banacer: %v", lb.Identifier())
	}
	return &resp.LoadBalancers[0], nil
}

// apply makes or updates the resource
func (lb LoadBalancer) apply(ctx context.Context) error {

	elbClient := client.ELB(ctx, lb.Context.ProviderName)

	subnetIDs, err := awsw.NewEC2(ctx, lb.Context.ProviderName).SubnetIdsByNames(ctx, lb.SubnetNames)
	if err != nil {
		return errors.Wrap(err, "failed listing subnet ids for lb")
	}

	securityGroupIDs, err := awsw.NewEC2(ctx, lb.Context.ProviderName).SecurityGroupIdsByNames(ctx, nil, lb.SecurityGroupNames)
	if err != nil {
		return errors.Wrap(err, "failed listing security group ids for lb")
	}

	tags := make([]elbv2types.Tag, 0)
	for k, v := range lb.Tags {
		tag := elbv2types.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		}
		tags = append(tags, tag)
	}

	// Create an internet facing application load balancer
	out, err := elbClient.CreateLoadBalancer(ctx, &elbv2.CreateLoadBalancerInput{
		IpAddressType:  elbv2types.IpAddressTypeIpv4, //TODO: support dualstack
		Name:           aws.String(lb.Identifier()),
		Scheme:         elbv2types.LoadBalancerSchemeEnum(*lb.Scheme),
		Type:           elbv2types.LoadBalancerTypeEnum(*lb.Type),
		Tags:           tags,
		Subnets:        subnetIDs,
		SecurityGroups: securityGroupIDs,
	})

	if err != nil {
		return errors.Wrap(err, "could not create load balancer")
	}

	if len(out.LoadBalancers) == 0 {
		return errors.New("could not create load balancer")
	}

	var loadBalancerArn = out.LoadBalancers[0].LoadBalancerArn

	//check status before updating attributes, making listeners & rules
	log.WithFields(log.Fields{
		"Name": lb.Identifier(),
	}).Infof("waiting for load balancer to reach `active` state")
	waiter := elbv2.NewLoadBalancerAvailableWaiter(elbClient, func(opts *elbv2.LoadBalancerAvailableWaiterOptions) {
		opts.MinDelay = 15 * time.Second
		opts.MaxDelay = 120 * time.Second
	})

	//wait for max 15m, with 15-120s check delays...
	if err = waiter.Wait(ctx, &elbv2.DescribeLoadBalancersInput{
		LoadBalancerArns: []string{*loadBalancerArn},
	}, 15*time.Minute); err != nil {
		return errors.Wrap(err, "could not verify load balancer status, or it failed to provision")
	}

	log.WithFields(log.Fields{
		"Name": lb.Identifier(),
	}).Info(color.Green("load balancer created"))

	// modify load balancer attributes
	if len(lb.Attributes) > 0 {
		err := lb.modifyLoadBalancerAttributes(ctx, loadBalancerArn)
		if err != nil {
			return errors.Wrapf(err, "error updating load balancer attributes")
		}
	}

	//Make listeners and add to the load balancer
	for _, listener := range lb.Listeners {
		listener.LoadBalancerARN = *loadBalancerArn
		listener.LoadBalancerName = lb.Identifier()
		listener.GlobalTags = InheritedTags(lb.Tags)
		err := listener.apply(ctx, lb.Context)
		if err != nil {
			log.WithField("Name", listener.Name).Error("error creating listener ")
			return err
		}
	}

	return nil
}

// updates the load balancer, it's listeners & rules
// func (lb LoadBalancer) update(ctx context.Context, diffs *LoadBalancerDiff) error {
func (lb LoadBalancer) applyDiffs(ctx context.Context, diffs ResourceDiff) error {

	if diffs == nil {
		log.WithFields(log.Fields{
			"Name": lb.Identifier(),
		}).Info("no updates required for load balancer")
		return nil
	}

	lbDiffs, ok := diffs.(*LoadBalancerDiff)
	if !ok {
		return errors.New("invalid diff type supplied")
	}

	elbClient := client.ELB(ctx, lb.Context.ProviderName)

	// scheme
	if lbDiffs.schemeDiff {
		return errors.New("cannot update load balancer scheme")
	}

	// type
	if lbDiffs.typeDiff {
		return errors.New("cannot update load balancer type")
	}

	// subnet Ids
	if lbDiffs.subnetsDiff {
		_, err := elbClient.SetSubnets(ctx, &elbv2.SetSubnetsInput{
			LoadBalancerArn: aws.String(lbDiffs.loadbalancerArn),
			Subnets:         lbDiffs.subnetIds,
		})
		if err != nil {
			return errors.Wrap(err, "error updating subnets for load balancer")
		}
		log.Info("updated subnets/availability zones")

	}

	if lbDiffs.securityGroupsDiff {
		_, err := elbClient.SetSecurityGroups(ctx, &elbv2.SetSecurityGroupsInput{
			LoadBalancerArn: aws.String(lbDiffs.loadbalancerArn),
			SecurityGroups:  lbDiffs.securityGroupIds,
		})
		if err != nil {
			return errors.Wrap(err, "error updating security groups for load balancer")
		}
		log.Info("updated security groups")
	}

	// Aatributes
	if lbDiffs.attributesDiffs {
		// add & delete attributes here...
		// TODO:
		err := lb.modifyLoadBalancerAttributes(ctx, &lbDiffs.loadbalancerArn)
		if err != nil {
			return errors.Wrapf(err, "error updating load balancer attributes")
		}
		log.Info("updated load balancer attributes")
	}

	// update/delete the listeners
	if lbDiffs.listenerDiffs {

		// delete
		for k, listener := range lbDiffs.listenersToDelete {
			if err := (LBListener{
				LoadBalancerName:        lb.Identifier(),
				LoadBalancerListenerArn: *listener.ListenerArn,
			}.Destroy(ctx, lb.Context)); err != nil {
				return errors.Wrapf(err, "error deleting listener %v", k)
			}
		}

		// update
		for new, diff := range lbDiffs.listenersToUpdate {
			new.GlobalTags = InheritedTags(lb.Tags)
			err := new.applyDiffs(ctx, lb.Context, diff)
			if err != nil {
				return errors.Wrap(err, "error updating listener")
			}
		}

		// add
		for k, listener := range lbDiffs.listenersToAdd {
			listener.LoadBalancerARN = lbDiffs.loadbalancerArn
			listener.LoadBalancerName = lb.Identifier()
			listener.GlobalTags = InheritedTags(lb.Tags)
			err := listener.apply(ctx, lb.Context)
			if err != nil {
				return errors.Wrapf(err, "error adding listener %v", k)
			}
		}
	}

	// tags diff
	if lbDiffs.tagsDiff {
		upserts := lbDiffs.tagDiff.Upserts()
		if len(upserts) > 0 {
			if err := awsw.NewELB(ctx, lb.Context.ProviderName).AddResourceTags(ctx, lbDiffs.loadbalancerArn, upserts); err != nil {
				return errors.Wrapf(err, "error updating load balancer tags for %v", lb.Identifier())
			}
		}
		if len(lbDiffs.tagDiff.Deleted) > 0 {
			if err := awsw.NewELB(ctx, lb.Context.ProviderName).DeleteResourceTags(ctx, lbDiffs.loadbalancerArn, lbDiffs.tagDiff.Deleted); err != nil {
				return errors.Wrapf(err, "error deleting load balancer tags for %v", lb.Identifier())
			}
		}
	}

	log.WithFields(log.Fields{
		"Name": lb.Identifier(),
	}).Info(color.Yellow("Load balancer updated"))

	return nil
}

// modifyLoadBalancerAttributes modifies the attributes for the specified load balancers
func (lb LoadBalancer) modifyLoadBalancerAttributes(ctx context.Context, arn *string) error {

	if len(lb.Attributes) == 0 {
		return nil
	}

	//make a copy
	attrs := make(map[string]string)
	for k, v := range lb.Attributes {
		attrs[k] = v
	}

	//some data cleansing.
	if attrs["access_logs.s3.enabled"] != "true" {
		delete(attrs, "access_logs.s3.bucket")
		delete(attrs, "access_logs.s3.prefix")
	}

	var loadBalancerAttributes []elbv2types.LoadBalancerAttribute
	for key, value := range attrs {
		loadBalancerAttributes = append(loadBalancerAttributes, elbv2types.LoadBalancerAttribute{
			Key:   aws.String(key),
			Value: aws.String(value),
		})
	}

	client := client.ELB(ctx, lb.Context.ProviderName)
	_, err := client.ModifyLoadBalancerAttributes(ctx, &elbv2.ModifyLoadBalancerAttributesInput{
		LoadBalancerArn: arn,
		Attributes:      loadBalancerAttributes,
	})

	if err != nil {
		return errors.Wrapf(err, "error modifying load balancer attributes")
	}

	log.WithFields(log.Fields{
		"Name": lb.Identifier(),
		"ARN":  *arn,
	}).Info(color.Yellow("load balancer attributes updated"))

	return nil
}

// loadBalancerAttributeUpserts returns only attributes explicitly defined in desired that
// are missing or different in existing. Extra keys present only in existing are ignored.
func loadBalancerAttributeUpserts(existing map[string]string, desired map[string]string) map[string]string {
	upserts := make(map[string]string)

	for key, desiredValue := range desired {
		existingValue, exists := existing[key]
		if !exists || existingValue != desiredValue {
			upserts[key] = desiredValue
		}
	}

	return upserts
}

func formatListenerDiffMessages(listenerKey string, messages []string) []string {
	formatted := make([]string, 0, len(messages))
	for _, msg := range messages {
		formatted = append(formatted, fmt.Sprintf("listener %v: %v", listenerKey, msg))
	}

	return formatted
}

func sortedListenerKeys[T any](listeners map[string]T) []string {
	keys := make([]string, 0, len(listeners))
	for key := range listeners {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	return keys
}

// getDefaultAttributes returns a default lb attributes hash map based on lb type
func (lb LoadBalancer) getDefaultAttributes() map[string]string {

	var attr map[string]string

	// for application load balancer
	if lb.Type == nil || (lb.Type != nil && *lb.Type == string(elbv2types.LoadBalancerTypeEnumApplication)) {
		attr = map[string]string{
			"access_logs.s3.enabled":                                   "false",
			"access_logs.s3.prefix":                                    "",
			"access_logs.s3.bucket":                                    "",
			"client_keep_alive.seconds":                                "3600",
			"deletion_protection.enabled":                              "true",
			"idle_timeout.timeout_seconds":                             "60",
			"routing.http.desync_mitigation_mode":                      "defensive",
			"routing.http.drop_invalid_header_fields.enabled":          "false",
			"routing.http.preserve_host_header.enabled":                "false",
			"routing.http.x_amzn_tls_version_and_cipher_suite.enabled": "false",
			"routing.http.xff_client_port.enabled":                     "false",
			"routing.http.xff_header_processing.mode":                  "append",
			"routing.http2.enabled":                                    "true",
			"waf.fail_open.enabled":                                    "false",
			"connection_logs.s3.prefix":                                "",
			"connection_logs.s3.enabled":                               "false",
			"connection_logs.s3.bucket":                                "",
			"load_balancing.cross_zone.enabled":                        "true",
			"zonal_shift.config.enabled":                               "false",
		}
	} else {
		attr = map[string]string{
			"access_logs.s3.enabled":            "false",
			"access_logs.s3.prefix":             "",
			"access_logs.s3.bucket":             "",
			"deletion_protection.enabled":       "false",
			"load_balancing.cross_zone.enabled": "false",
			"dns_record.client_routing_policy":  "any_availability_zone",
		}
	}

	// TODO: default attributes supported only when ipv6/dual-stack enabled
	// if lb.ip_address_type == ipv6/dualstack {
	// 	attr["ipv6.deny_all_igw_traffic"] = "false"
	// }

	return attr
}
