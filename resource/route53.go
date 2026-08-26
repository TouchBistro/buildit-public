package resource

import (
	"context"
	"fmt"
	"strings"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/TouchBistro/buildit/client"
	"github.com/TouchBistro/buildit/util"
	"github.com/TouchBistro/goutils/color"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

const (
	RecordTypeA     = string(route53types.RRTypeA)
	RecordTypeAAAA  = string(route53types.RRTypeAaaa)
	RecordTypeCNAME = string(route53types.RRTypeCname)
	RecordTypeCAA   = string(route53types.RRTypeCaa)
	RecordTypeMX    = string(route53types.RRTypeMx)
	RecordTypeNS    = string(route53types.RRTypeNs)
	RecordTypeSOA   = string(route53types.RRTypeSoa)
	RecordTypeTXT   = string(route53types.RRTypeTxt)
)

const (
	AliasTypeLoadBalancer           = "load-balancer"
	AliasTypeCloudfrontDistribution = "cloudfront-distribution"
)

// Route53RecordRoutingPolicy represents a routing policy for the route 53 DNS record, if anything but simple DNS policy is
type Route53RecordRoutingPolicy struct {
	SetIdentifier string `yaml:"identifier"` // unique values per resource name/type, this is used when weighted (or any other in the future) routing policy
	Weight        int64  `yaml:"weight"`     // weight 0-255 if this is a weighted record
}

// Route53Record represents the DNS record to be created.
//
// Note:
// Since route53-record resource name is considered the subdomain (DNS record name portion) of the
// fqdn, we have an additional optional attribute recordName that may be used as the record name if
// supplied.
// This is important when a single buildit.yml file has more than one route53-record resources
// defined with the same record name e.g my.example.com and my.domain.com where it's impossible to
// use a unique resource name. In this case the resource name in yaml descriptor can be
// my-example and my-domain respectively, while the 'recordName' attirbute would supply the
// DNS record name `my` for both.
// If the resource needs to represent the apex record (no sub-domain), the `recordName` attribute
// must be used and must be set to `@`. This is a special case which represents the apex/root DNS
// record for a domain, e.g example.com itself.
//
// if 'RecordName' attribute is supplied, the Name attribute is ignored, and only used as the identifier
// for logging
//
// The name or recordName for this resource supports the format <provider>/<name> which means you can
// use a non-main buildit provider when working with a route53-record
type Route53Record struct {
	BaseResource `yaml:",inline"`
	Name         string                      `yaml:"-"`
	RecordName   *string                     `yaml:"recordName"`    // use this instead of the name, this is the DNS record name part, excluding the namespace; use '@' when apex record (top node)
	HostedZone   string                      `yaml:"hostedZone"`    // hosted-zone or the DNS namespace e.g for a record x.y.example.com, example.com is the hosted zone
	Type         *string                     `yaml:"recordType"`    // only 'A' & 'CNAME' supported
	AliasType    *string                     `yaml:"aliasType"`     // indicates the record is an alias if non-nil, Only load-balancer type Alias Route53 records supported
	Destinations []string                    `yaml:"destinations"`  // destination, answer part of the DNS record; or alias target
	TTL          *int64                      `yaml:"ttl"`           // record ttl in seconds, used for non-alias records
	Policy       *Route53RecordRoutingPolicy `yaml:"routingPolicy"` // routing policy details if non-simple routing is required
	DependsOn    []Key                       `yaml:"dependsOn"`
	dnsName      string                      // Parsed from Name
	hostedZoneId *string                     // fetched during normalize
	//TODO: implement tags
}

// Key returns the unique key for the resource for this buildit context
func (r Route53Record) Key() Key {
	return NewKey(r.Context.ProviderName, r.Identifier())
}

// Identifier returns the unique ID for the record.
func (r Route53Record) Identifier() string {
	return r.Name
}

// Normalize will set any default values.
func (r *Route53Record) Normalize(ctx context.Context) {

	recordName := r.Identifier()
	if r.RecordName != nil {
		recordName = *r.RecordName
	}

	_, r.dnsName = ParseName(recordName) // we've already determined the provider value in internal_config.go; here we just fetch the record name part of it

	// if the recordName is the fqdn, like sub.example.com & hostZone domain is example.com, then we
	// strip the domain name with the leading period (.example.com) part away & the recordName will be "sub"
	if strings.HasSuffix(r.dnsName, r.HostedZone) {
		r.dnsName = strings.TrimSuffix(r.dnsName, fmt.Sprintf(".%v", r.HostedZone))
	}

	// add . after the end of the hostedzone / namespace
	if !strings.HasSuffix(r.HostedZone, ".") {
		r.HostedZone += "."
	}

	// set A as default record type if not supplied
	if r.Type == nil {
		r.Type = aws.String(RecordTypeA)
	}

	// populate hosted zone to avoid multiple lookups
	hostedZoneId, err := awsw.NewRoute53(ctx, r.Context.ProviderName).FindHostedZoneIdForDomain(ctx, r.HostedZone)
	if err == nil {
		r.hostedZoneId = hostedZoneId
	}

	if r.TTL == nil {
		r.TTL = aws.Int64(60)
	}
}

// Validate checks that the Route53Record has a valid configuration.
func (r Route53Record) Validate(ctx context.Context) error {
	var msgs []string

	if r.RecordName != nil && *r.RecordName == "" {
		msgs = append(msgs, "record name is required")
	}

	if r.Name == "" {
		msgs = append(msgs, "name is required")
	}

	if r.HostedZone == "" {
		msgs = append(msgs, "hostedZone is required")
	}

	if r.hostedZoneId == nil {
		msgs = append(msgs, "invalid hosted zone, lookup failed")
	}

	switch *r.Type {
	case RecordTypeA, RecordTypeCNAME, RecordTypeAAAA, RecordTypeCAA, RecordTypeMX, RecordTypeNS, RecordTypeSOA, RecordTypeTXT:
	default:
		msg := fmt.Sprintf("invalid record type %q", *r.Type)
		msgs = append(msgs, msg)
	}

	// Make sure AliasType is valid based on rules
	if r.AliasType != nil {
		switch *r.AliasType {
		case AliasTypeLoadBalancer, AliasTypeCloudfrontDistribution:
		default:
			msg := fmt.Sprintf("invalid alias type %q, only %v or %v are supported", *r.AliasType, AliasTypeLoadBalancer, AliasTypeCloudfrontDistribution)
			msgs = append(msgs, msg)
		}
	}

	// Alias type requires an A record (or empty record type which is defaulted to A)
	if r.AliasType != nil && *r.Type != RecordTypeA {
		msg := fmt.Sprintf("invalid record type %s used with alias record; record type must be A or omitted when alias type is provided", *r.Type)
		msgs = append(msgs, msg)
	}

	// Make sure destinations is valid
	if len(r.Destinations) == 0 {
		msgs = append(msgs, "at least one destination is required")
	}
	// Certain cases only allow a single destination
	if len(r.Destinations) > 1 {
		// You can only alias a single AWS resource
		if r.AliasType != nil {
			msgs = append(msgs, "only 1 destination is allowed when an alias destination is used")
		} else if *r.Type == RecordTypeCNAME {
			msgs = append(msgs, "only 1 destination is allowed when a CNAME record is used")
		}
	}

	if r.isWeightedRouting() && (r.Policy.Weight < 0 || r.Policy.Weight > 255) {
		msgs = append(msgs, fmt.Sprintf("invalid weight supplied for weighted policy %v -> %v, expected 0-255", r.Policy.SetIdentifier, r.Policy.Weight))
	}

	if msgs == nil {
		return nil
	}
	return &ValidationError{
		ResourceIdentifier: r.Identifier(),
		ResourceType:       "Route53Record",
		Messages:           msgs,
	}
}

// Apply creates or updates the route53-record resource
func (r Route53Record) Apply(ctx context.Context) error {
	log.Debugf("creating route53 record %v", r.Identifier())

	diffs, err := r.Compare(ctx)
	if err != nil {
		return err
	}

	if diffs == nil {
		log.WithFields(log.Fields{
			"Name": r.Identifier(),
		}).Info("no updates required")
		return nil
	}

	//if diff found & existing resource exists...
	if diffs.AWSResource() != nil {
		log.WithField("Name", r.Identifier()).Info("route53 record already exists, updating")
		for _, d := range diffs.Differences() {
			log.Debug(d)
		}
		err = r.applyDiffs(ctx, diffs)
		if err != nil {
			return errors.Wrapf(err, "failed to update route53 record %v", r.Identifier())
		}
		return nil
	}

	return r.apply(ctx)
}

// Destroy removes the route53 record
func (r Route53Record) Destroy(ctx context.Context) error {

	log.WithField("Name", r.Identifier()).Debug("checking if route53 record exists")
	existings, err := r.fetchExisting(ctx)
	if err != nil {
		return errors.Wrapf(err, "failed to check if route53 record %s exists", r.Identifier())
	}

	if len(existings) == 0 {
		log.WithField("Name", r.Identifier()).Info("route53 record does not exist, nothing to destroy")
		return nil
	}

	existing := &existings[0]
	log.WithField("Name", r.Identifier()).Debug("Destroying route53 record")
	if err = r.destroy(ctx, existing); err != nil {
		return errors.Wrap(err, "failed to delete resource record set")
	}

	log.WithFields(log.Fields{
		"Name": r.Identifier(),
	}).Info(color.Red("route53 record destroyed"))
	return nil
}

// destroy the supplied route53 types/ResourceRecordset
func (r Route53Record) destroy(ctx context.Context, existing *route53types.ResourceRecordSet) error {
	log.Debugf("destroying existing route53 record %v", r.Identifier())
	route53Client := client.Route53(ctx, r.Context.ProviderName)
	_, err := route53Client.ChangeResourceRecordSets(ctx, &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: r.hostedZoneId,
		ChangeBatch: &route53types.ChangeBatch{
			Comment: aws.String("Removing DNS record from hosted zone"),
			Changes: []route53types.Change{
				{
					Action:            route53types.ChangeActionDelete,
					ResourceRecordSet: existing,
				},
			},
		},
	})
	return err
}

type Route53RecordDiff struct {
	BaseResourceDiff

	routingPolicyDiff bool
	typeDiff          bool
	aliasDiff         bool
	aliasValueDiff    bool
	destDiff          bool
	ttlDiff           bool
	weightDiff        bool
}

func (r Route53Record) isSimpleRouting() bool {
	return r.Policy == nil // simple routing
}
func (r Route53Record) isWeightedRouting() bool {
	return r.Policy != nil // weighted routing
}

// Compare the route53 record spec with the existing record, if it exists, & returns diffs
func (r Route53Record) Compare(ctx context.Context) (ResourceDiff, error) {

	existings, err := r.fetchExisting(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "error fetching aws resource for %v", r.Identifier())
	}

	diffs := &Route53RecordDiff{}

	// existing not found, then we have a diff, record doesn't exist...
	if existings == nil {
		diffs.Messages = append(diffs.Messages, "route53 record does not exist")
		return diffs, nil
	}

	// now we check if record routing is being updated. In that case we really don't need to
	// check any further diffs as that would not have any impact on the apply() logic

	// switching from weighted to simple
	if r.isSimpleRouting() {
		if len(existings) >= 1 && existings[0].Weight != nil {
			diffs.Resource = existings // this points the Compare() code that some existing object(s) were found...
			diffs.Messages = append(diffs.Messages, "existing record has weighted routing, switching to simple")
			diffs.routingPolicyDiff = true
			return diffs, nil
		}
	}

	// switching from simple to weighted
	if r.isWeightedRouting() {
		if len(existings) == 1 && existings[0].Weight == nil {
			diffs.Resource = existings
			diffs.Messages = append(diffs.Messages, "existing record has simple routing, switching to weighted")
			diffs.routingPolicyDiff = true
			return diffs, nil
		}
	}

	// if we've come this far, its either:
	//  1- both existing & defined have simple routing
	//  2- both existing & defined have weighted routing
	//
	// in either case, we should only have 1 existing record that matches
	// the defined record & we need to get that for comparisons.
	var existing *route53types.ResourceRecordSet
	if len(existings) > 0 {
		existing = &existings[0]
	} else {
		panic(fmt.Errorf("this error indicates a bad return values fetchExisting() was returned"))
	}

	diffs.Resource = existings // for consistency, we always put an []types/RRSet for the existing

	// rr type
	if *r.Type != string(existing.Type) {
		diffs.typeDiff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("record type is different %v -> %v", string(existing.Type), *r.Type))
		return diffs, nil // we ignore any further diffs, since it diesn't, since Record Type diff requires delete & recreate
	}

	var found bool

	// alias
	if r.AliasType == nil && existing.AliasTarget != nil {
		found = true
		diffs.aliasDiff = true
		diffs.Messages = append(diffs.Messages, "record alias target is being removed")
	} else if r.AliasType != nil && existing.AliasTarget == nil {
		found = true
		diffs.aliasDiff = true
		diffs.Messages = append(diffs.Messages, "record alias target is being added")
	} else if r.AliasType != nil && existing.AliasTarget != nil {
		trg, err := r.convertToAliasTarget(ctx)
		if err != nil {
			return nil, errors.Wrapf(err, "error converting referenced resource to alias target")
		}
		if *trg.DNSName != *existing.AliasTarget.DNSName ||
			*trg.HostedZoneId != *existing.AliasTarget.HostedZoneId ||
			trg.EvaluateTargetHealth != existing.AliasTarget.EvaluateTargetHealth {
			found = true
			diffs.aliasValueDiff = true
			diffs.Messages = append(diffs.Messages, fmt.Sprintf("record alias target is different %v/%v(%v) -> %v/%v(%v)",
				*existing.AliasTarget.HostedZoneId, *existing.AliasTarget.DNSName, existing.AliasTarget.EvaluateTargetHealth,
				*trg.HostedZoneId, *trg.DNSName, trg.EvaluateTargetHealth))
		}
	} else {
		// TTL & destination diffs only come into play if neither defined, nor existing records are not alias records

		// ttl
		if *r.TTL != *existing.TTL {
			found = true
			diffs.ttlDiff = true
			diffs.Messages = append(diffs.Messages, fmt.Sprintf("record alias ttl is different %v -> %v", *existing.TTL, *r.TTL))
		}

		// destination
		if len(r.Destinations) != len(existing.ResourceRecords) {
			found = true
			diffs.destDiff = true
			diffs.Messages = append(diffs.Messages, "record destinations are different")
		} else {
			for i, d := range r.Destinations {
				if d != *existing.ResourceRecords[i].Value {
					found = true
					diffs.destDiff = true
					diffs.Messages = append(diffs.Messages, "record destinations are different")
					break
				}
			}
		}
	}

	// policy diff?
	// weighted -> weighted
	if r.isWeightedRouting() {
		if r.Policy.Weight != util.CoalesceComparable(existing.Weight, 0) {
			// weight changed
			found = true
			diffs.weightDiff = true
			diffs.Messages = append(diffs.Messages, fmt.Sprintf("record routing policy weight is different %v -> %v",
				util.CoalesceComparable(existing.Weight, 0), r.Policy.Weight))
		}

		if r.Policy.SetIdentifier != util.Coalesce(existing.SetIdentifier, "") {
			// this must never happen, since if defined & existing were both weighted routing,
			// then the fetchExisting() should only returns when setIdentifier is the same
			log.Warnf("invalid diff found, report this bug in route53 resource fetchExisting() logic")
			panic(r)
		}
	}

	if !found {
		return nil, nil
	}

	return diffs, nil
}

// fetchExisting, fetches the existing record(s) that match the current definition.
// there is some additional logic carried out here since it is possible to return
// multiple reocrds matching the definition
//
// ResourceRecords are searched for matching record name (pattern) & then we loop through
// the results to collect all matching results based on what is defined. any prefix matched
// record names ( i.e startRecordName => "test" matches both test.example.com
// & test1.example.com) that do not match with the defined name are dropped from the results
// as they were not intended ones.
//
// if no match is found, a nil is returned to indicate that.
//
// if the definition is simple routing, then all matching resuts (existing weighted) are
// returned as an []types/RRSet type. if no match found, a nil is returned.
//
// if the definition is weighted routing, then an additional filter of `setIdentifier` is
// added & only a single match is returned as []types/RRSet. If a matching `setIdentifer` doesn't
// exist, it means no existing record matches the definition & a nil result is returned.
func (r Route53Record) fetchExisting(ctx context.Context) ([]route53types.ResourceRecordSet, error) {

	route53Client := client.Route53(ctx, r.Context.ProviderName)
	recordName := r.fullRecordName()
	out, err := route53Client.ListResourceRecordSets(ctx, &route53.ListResourceRecordSetsInput{
		HostedZoneId:    r.hostedZoneId,
		MaxItems:        aws.Int32(50),          // TODO we need to loop & check for more
		StartRecordName: aws.String(recordName), // only filter by startRecordName, type may be different
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to list route53 records")
	}

	// when no RRSets found, then there is no existing records
	if len(out.ResourceRecordSets) == 0 {
		return nil, nil
	}

	// collect & filter results...
	matching := make([]route53types.ResourceRecordSet, 0)
	for _, recordSet := range out.ResourceRecordSets {
		awsRecordSetName := *recordSet.Name
		awsRecordSetName = strings.ReplaceAll(awsRecordSetName, "\\052", "*") //replace all octals for `*` to chart
		if recordName == awsRecordSetName {
			matching = append(matching, recordSet)
		}
	}

	// if nothing matches, then we return nil
	if len(matching) == 0 {
		return nil, nil
	}

	// if defined record has SimpleRouting, then we return all matching records as []types/RRSet
	if r.Policy == nil {
		return matching, nil
	}

	// if a single existing record matches && it has SimpleRouting, then we return that
	// this is important, as either if defined is simple or weighted, this will
	// be used to make a diff
	if len(matching) == 1 && matching[0].Weight == nil {
		return matching, nil
	}

	// if defined record has WeightedRouting, then we try to match the setIdentifier as well..
	for _, recordSet := range matching {
		if r.Policy.SetIdentifier == util.Coalesce(recordSet.SetIdentifier, "") {
			return []route53types.ResourceRecordSet{recordSet}, nil
		}
	}

	return nil, nil // not found at all
}

// apply creates a new route53 record
func (r Route53Record) apply(ctx context.Context) error {

	recordSet := &route53types.ResourceRecordSet{
		Name: aws.String(r.fullRecordName()),
		Type: route53types.RRType(*r.Type),
	}

	// if alias record
	if r.AliasType != nil {
		tg, err := r.convertToAliasTarget(ctx)
		if err != nil {
			return errors.Wrapf(err, "error converting referenced resource to alias target")
		}
		recordSet.AliasTarget = tg
	} else {
		recordSet.TTL = r.TTL
		for _, d := range r.Destinations {
			rr := route53types.ResourceRecord{Value: aws.String(d)}
			recordSet.ResourceRecords = append(recordSet.ResourceRecords, rr)
		}
	}

	// check if weight
	if r.isWeightedRouting() {
		recordSet.Weight = aws.Int64(r.Policy.Weight)
		recordSet.SetIdentifier = aws.String(r.Policy.SetIdentifier)
	}

	route53Client := client.Route53(ctx, r.Context.ProviderName)
	_, err := route53Client.ChangeResourceRecordSets(ctx, &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: r.hostedZoneId,
		ChangeBatch: &route53types.ChangeBatch{
			Comment: aws.String("Attaching DNS record to hosted zone"),
			Changes: []route53types.Change{
				{
					Action:            route53types.ChangeActionUpsert,
					ResourceRecordSet: recordSet,
				},
			},
		},
	})
	if err != nil {
		return errors.Wrap(err, "failed to upsert resource record set")
	}

	log.WithFields(log.Fields{
		"Name": r.Identifier(),
	}).Info(color.Green("route53 record upserted"))
	return nil
}

// applyDiffs applies the diffs, for route53 record, if the RRType is different,
// we destroy & create a new record, for all other changes, even when alias type
// is different, but same RRType, we call apply() to upsert the record
func (r Route53Record) applyDiffs(ctx context.Context, diffs ResourceDiff) error {

	if diffs == nil {
		log.WithFields(log.Fields{
			"Name": r.Identifier(),
		}).Info("no updates required for route53 record")
		return nil
	}

	r53Diffs, ok := diffs.(*Route53RecordDiff)
	if !ok {
		return errors.New("invalid diff type supplied")
	}

	// destroy & recreate if the RRType || routingPolicy is not the same;
	// these are not valid UPSERT operations for the AWS Route53 API
	if r53Diffs.typeDiff || r53Diffs.routingPolicyDiff {
		//[]types.RRSet type existing
		if existings, ok := r53Diffs.Resource.([]route53types.ResourceRecordSet); !ok {
			return errors.Errorf("cannot retrieve existing route53-record(s)")
		} else {
			for _, existing := range existings {
				err := r.destroy(ctx, &existing)
				if err != nil {
					return errors.Wrapf(err, "error destroying old records before recreating")
				}
			}
		}
	}

	// applies the diff, as the apply() logic upserts the records
	return r.apply(ctx)
}

// convertToAliasTarget converts the AliasType && Destination combination to a ablv2/types/AliasTarget, or error
func (r Route53Record) convertToAliasTarget(ctx context.Context) (*route53types.AliasTarget, error) {
	//load-balancer
	switch *r.AliasType {
	case AliasTypeLoadBalancer:
		log.Debug("resolving load balancer by name")
		provider, lbName := ParseName(r.Destinations[0])

		lb, err := awsw.NewELB(ctx, provider).FindLoadBalancerForName(ctx, lbName)
		if err != nil {
			return nil, errors.Wrapf(err, "couldn't find the referenced load balancer %v", r.Destinations[0])
		}

		dnsName := strings.ToLower(*lb.DNSName)
		if !strings.HasSuffix(dnsName, ".") {
			dnsName += "."
		}

		return &route53types.AliasTarget{
			HostedZoneId:         lb.CanonicalHostedZoneId, // Magic hosted Zone ID for us-east-1 ALBs. See: https://docs.aws.amazon.com/general/latest/gr/elb.html
			DNSName:              aws.String(dnsName),      // DNS name of the load balancer, eg "dualstack.example-lb.us-east-1.elb.amazonaws.com."
			EvaluateTargetHealth: false,
		}, nil
	case AliasTypeCloudfrontDistribution:
		log.Debug("resolving load balancer by name")

		provider, distId := ParseName(r.Destinations[0])
		_, _ = provider, distId

		return &route53types.AliasTarget{
			HostedZoneId:         aws.String("Z2FDTNDATAQYW2"),  // this is a fix hosted-zone id for the cloudfront distribution
			DNSName:              aws.String(r.Destinations[0]), // DNS name, or the custom dns name of the cloudfront distribution
			EvaluateTargetHealth: false,
		}, nil

	}
	return nil, errors.Errorf("cannot convert to a supported route53 alias target")
}

// fullRecordName returns the full dns name including the record name & the domain name portion
// for apex records, e.g @.example.com, it returns the domain name portion only, i.e example.com
func (r Route53Record) fullRecordName() string {
	if r.dnsName == "@" { //apex
		return r.HostedZone
	}
	return r.dnsName + "." + r.HostedZone
}
