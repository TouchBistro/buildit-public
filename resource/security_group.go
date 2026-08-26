package resource

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/TouchBistro/buildit/client"
	"github.com/TouchBistro/buildit/util"
	"github.com/TouchBistro/goutils/color"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

const (
	staleSecurityGroupUserID = "0000000000"
)

const (
	cidrRange   = "cidr"
	sgReference = "sg"
)

const protocolAll = "-1"

// SecurityGroup represents a security group resource
type SecurityGroup struct {
	BaseResource `yaml:",inline"`
	Description   string              `yaml:"description"`
	VPCName       string              `yaml:"vpcName"`
	InboundRules  []SecurityGroupRule `yaml:"inboundRules"`
	OutboundRules []SecurityGroupRule `yaml:"outboundRules"`
	DependsOn     []Key               `yaml:"dependsOn"`
	Name          string              `yaml:"-"`
	Tags          map[string]string   `yaml:"tags"`
	GlobalTags    map[string]string   `yaml:"-"`
}

// SecurityGroupRule represents a inbound or outbound rule
type SecurityGroupRule struct {
	IpProtocol           string                    `yaml:"ipProtocol"`
	PortRange            string                    `yaml:"portRange"`
	CidrBlocks           []SecurityGroupRuleSource `yaml:"cidrBlocks"`
	SourceSecurityGroups []SecurityGroupRuleSource `yaml:"securityGroups"`
}

// SecurityGroupRuleSource represents the source cidr/security group for each rule
type SecurityGroupRuleSource struct {
	Value       string `yaml:"value"` //cidr or security group name
	Description string `yaml:"description"`
	accountId   string `yaml:"-"`
}

// Key returns the unique key for the resource for this buildit context
func (sg SecurityGroup) Key() Key {
	return NewKey(sg.Context.ProviderName, sg.Identifier())
}

// Identifier returns the unique id for the resource
func (sg SecurityGroup) Identifier() string {
	return sg.Name
}

// Normalize will set any default values or sanitize/clean up any necessary fields
func (sg *SecurityGroup) Normalize(ctx context.Context) {

	// Merge globalTags to security group tags, if key is not already present
	// later we'll use sg.Tags to add/update tags
	if sg.Tags == nil {
		sg.Tags = make(map[string]string)
	}
	ResourceTags(sg.Tags).Merge(sg.GlobalTags)

	// Name tag
	if !ResourceTags(sg.Tags).Contains("Name") {
		sg.Tags["Name"] = sg.Identifier()
	}
}

// mergeSecurityGroupRules merges the other [] of security group rules into this
// and returns the result or error in case parsing of rules fails
// we use the port-range & ipProtocol fields as keys, if a matching
// rule already exists, then the cidrBlocks & securityGroup References
// are merged into that rule, else the overriden rule is added to
// the security group.
func mergeSecurityGroupRules(this, other []SecurityGroupRule) ([]SecurityGroupRule, error) {

	imap := make(map[sgRule]SecurityGroupRule)
	var mergedRules []SecurityGroupRule

	// build a map of existing rules (from this []SecurityGroupRule)
	for _, ir := range this {
		from, to, err := parsePortRange(ir.PortRange)
		if err != nil {
			return nil, err
		}
		or := sgRule{from: from, to: to, protocol: ir.IpProtocol}
		if tr, ok := imap[or]; !ok {
			imap[or] = ir
		} else {
			p_tr := &tr
			p_tr.CidrBlocks = append(tr.CidrBlocks, ir.CidrBlocks...)
			p_tr.SourceSecurityGroups = append(tr.SourceSecurityGroups, ir.SourceSecurityGroups...)
			imap[or] = *p_tr // update existing rule
		}
	}

	// walk the other []SecurityGroupRule & add or update as required
	for _, r := range other {
		from, to, err := parsePortRange(r.PortRange)
		if err != nil {
			return nil, err
		}
		or := sgRule{from: from, to: to, protocol: r.IpProtocol}
		if tr, ok := imap[or]; !ok {
			imap[or] = r // add new rule
		} else {
			p_tr := &tr
			p_tr.CidrBlocks = append(tr.CidrBlocks, r.CidrBlocks...)
			p_tr.SourceSecurityGroups = append(tr.SourceSecurityGroups, r.SourceSecurityGroups...)
			imap[or] = *p_tr // update existing rule
		}
	}

	// convert the map to []SecurityGroupRules
	for _, r := range imap {
		mergedRules = append(mergedRules, r)
	}
	return mergedRules, nil
}

// Merge the supplied security group spec into this config
func (sg *SecurityGroup) Merge(other SecurityGroup) error {

	// description
	if other.Description != "" {
		sg.Description = other.Description
	}

	// vpc name
	if other.VPCName != "" {
		sg.VPCName = other.VPCName
	}

	// inbound rules
	var err error
	if len(other.InboundRules) > 0 {
		sg.InboundRules, err = mergeSecurityGroupRules(sg.InboundRules, other.InboundRules)
		if err != nil {
			return err
		}
	}
	// outboud rules
	if len(other.OutboundRules) > 0 {
		sg.OutboundRules, err = mergeSecurityGroupRules(sg.OutboundRules, other.OutboundRules)
		if err != nil {
			return err
		}
	}

	// tags
	if len(other.Tags) > 0 {
		if sg.Tags == nil {
			sg.Tags = make(map[string]string)
		}
		ResourceTags(sg.Tags).Merge(other.Tags)
	}

	// depends on
	sg.DependsOn = append(sg.DependsOn, other.DependsOn...)

	return nil
}

// Validate the security group input
func (sg SecurityGroup) Validate(ctx context.Context) error {

	var errorMsgs []string

	if sg.Identifier() == "" {
		errorMsgs = append(errorMsgs, "security group name cannot be empty")
	}

	if sg.VPCName == "" {
		errorMsgs = append(errorMsgs, "vpc name must be supplied to create a security group")
	}

	if errorMsgs == nil {
		return nil
	}

	return &ValidationError{
		ResourceIdentifier: sg.Identifier(),
		ResourceType:       "Security Group",
		Messages:           errorMsgs,
	}
}

// Apply builds the security group
func (sg SecurityGroup) Apply(ctx context.Context) error {

	log.Debugf("creating security group %v", sg.Identifier())

	diffs, err := sg.Compare(ctx)
	if err != nil {
		return err
	}

	if diffs == nil {
		log.WithFields(log.Fields{
			"Name": sg.Identifier(),
		}).Info("no updates required")
		return nil
	}

	//if diff found & existing resource exists...
	if diffs.AWSResource() != nil {
		log.WithField("Name", sg.Identifier()).Info("security group already exists, updating")
		for _, d := range diffs.Differences() {
			log.Debug(d)
		}
		err = sg.applyDiffs(ctx, diffs)
		if err != nil {
			return errors.Wrapf(err, "failed to update security group %v", sg.Identifier())
		}
		return nil
	}

	return sg.apply(ctx)
}

// Destroy the resource
func (sg SecurityGroup) Destroy(ctx context.Context) error {

	log.Debugf("destroying security group %v", sg.Identifier())

	existing, err := sg.fetchExisting(ctx)
	if err != nil {
		return errors.Wrapf(err, "error finding security group %v", sg.Identifier())
	}

	if existing == nil {
		log.WithFields(log.Fields{
			"Name": sg.Identifier(),
		}).Info("security group does not exist, nothing to destroy, skippping ")
		return nil
	}

	ec2Client := client.EC2(ctx, sg.Context.ProviderName)
	log.Debugf("waiting until the security group %v has no associations", sg.Identifier())
	err = waitUntilSecurityGroupDisassociated(ctx, sg.Context, *existing.GroupId, 15) //15 retries, 15s apart
	if err != nil {
		return errors.Wrapf(err, "security group (%v) is associated with a network interface, cannot delete", sg.Identifier())
	}

	_, err = ec2Client.DeleteSecurityGroup(ctx, &ec2.DeleteSecurityGroupInput{
		GroupId: existing.GroupId,
	})
	if err != nil {
		return errors.Wrapf(err, "error deleting security group %v", sg.Identifier())
	}

	log.WithFields(log.Fields{
		"Name": sg.Identifier(),
	}).Info(color.Red("security group destroyed"))
	return nil
}

// SecurityGroupDiff respresnts diffs between security group definition & AWS representation
type SecurityGroupDiff struct {
	BaseResourceDiff

	// private diff details
	ingressDiff   bool
	ingressAdd    []sgRule
	ingressRemove []sgRule
	egressDiff    bool
	egressAdd     []sgRule
	egressRemove  []sgRule
	tagsDiff      bool
	tagDiff       util.TagDiffResult
}

// Compare fetches the existing security group, and if it exists, checks if this
// resource is equal to the corresponding AWS SecurityGroup
func (sg SecurityGroup) Compare(ctx context.Context) (ResourceDiff, error) {

	existing, err := sg.fetchExisting(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "error fetching aws resource for %v", sg.Identifier())
	}

	diffs := &SecurityGroupDiff{}

	if existing == nil {
		diffs.Messages = append(diffs.Messages, "security group does not exist")
		return diffs, nil
	}

	diffs.Resource = existing

	// comparing description

	if sg.Description != *existing.Description {
		diffs.Messages = append(diffs.Messages, "security group description is not the same")
	}

	// comparing vpc id
	sgVpcId, err := awsw.NewEC2(ctx, sg.Context.ProviderName).VpcIdByName(ctx, sg.VPCName)
	if err != nil {
		return nil, err
	}

	if *sgVpcId != *existing.VpcId {
		diffs.Messages = append(diffs.Messages, "security group vpcid is different")
	}

	// comparing ingress rules
	new, err := flattenSecurityGroupRules(ctx, sg.Context, sg.InboundRules)
	if err != nil {
		return nil, errors.Wrap(err, "error normalizing new security group rules")
	}
	old, err := flattenAwsIpPermission(existing.IpPermissions)
	if err != nil {
		return nil, errors.Wrap(err, "error normalizing existing security group rules")
	}

	ingressDiff, ingressAdd, ingressRemove := diffRules(new, old)

	if ingressDiff {
		diffs.Messages = append(diffs.Messages, "ingress rules are not the same")
		for _, r := range ingressAdd {
			diffs.Messages = append(diffs.Messages, fmt.Sprintf("security group rule to be added port range:%v-%v, protocol:%v, src:%v, type:%v", r.from, r.to, r.protocol, r.src.Value, r.typ))
		}
		for _, r := range ingressRemove {
			diffs.Messages = append(diffs.Messages, fmt.Sprintf("security group rule to be removed port range:%v-%v, protocol:%v, src:%v, type:%v", r.from, r.to, r.protocol, r.src.Value, r.typ))
		}

		diffs.ingressDiff = true
		diffs.ingressAdd = ingressAdd
		diffs.ingressRemove = ingressRemove
	}

	// comparing egress rules

	new, err = flattenSecurityGroupRules(ctx, sg.Context, sg.OutboundRules)
	if err != nil {
		return nil, errors.Wrap(err, "error normalizing new egress security group rules")
	}
	old, err = flattenAwsIpPermission(existing.IpPermissionsEgress)
	if err != nil {
		return nil, errors.Wrap(err, "error normalizing existing egress security group rules")
	}

	egressDiff, egressAdd, egressRemove := diffRules(new, old)
	if egressDiff {
		diffs.Messages = append(diffs.Messages, "egress rules are not the same")
		diffs.egressDiff = egressDiff
		diffs.egressAdd = egressAdd
		diffs.egressRemove = egressRemove
	}

	// tags
	var tagsDiff bool
	awsTags, err := awsw.NewEC2(ctx, sg.Context.ProviderName).GetResourceTags(ctx, *existing.GroupId)
	if err != nil {
		return nil, err
	}
	if tagDiff := TagDiffForContext(ctx, awsTags, sg.Tags); tagDiff.HasChanges() {
		tagsDiff = true
		diffs.tagsDiff = true
		diffs.tagDiff = tagDiff
		diffs.Messages = append(diffs.Messages, TagDiffSummary(awsTags, diffs.tagDiff)...)
	}

	if !ingressDiff && !egressDiff && !tagsDiff {
		return nil, nil
	}

	return diffs, nil
}

// fetchExisting fetches security group if exists
func (sg SecurityGroup) fetchExisting(ctx context.Context) (*ec2types.SecurityGroup, error) {
	group, err := securityGroupByName(ctx, sg.Context, sg.Identifier())
	if err != nil {
		return nil, errors.Wrap(err, "error while retrieving security group details")
	}
	if group == nil {
		return nil, nil
	}
	return group, nil
}

// apply provisions a new security group
func (sg SecurityGroup) apply(ctx context.Context) error {

	ec2Client := client.EC2(ctx, sg.Context.ProviderName)

	// faltten & validate ingress & egress rules to make sure there are not duplicates
	// before we even create the security group
	ingress, err := flattenSecurityGroupRules(ctx, sg.Context, sg.InboundRules)
	if err != nil {
		return errors.Wrapf(err, "error parsing ingress rules for %v", sg.Identifier())
	}
	egress, err := flattenSecurityGroupRules(ctx, sg.Context, sg.OutboundRules)
	if err != nil {
		return errors.Wrapf(err, "error parsing egress rules for %v", sg.Identifier())
	}

	ingressPermissions := sgRulesToIpPermissions(ingress)
	egressPermissions := sgRulesToIpPermissions(egress)

	// fetch vpc id
	vpcid, err := awsw.NewEC2(ctx, sg.Context.ProviderName).VpcIdByName(ctx, sg.VPCName)
	if err != nil {
		return errors.Wrap(err, "failed looking up vpc id")
	}

	// provision security group
	respCreateSecurityGroup, err := ec2Client.CreateSecurityGroup(ctx, &ec2.CreateSecurityGroupInput{
		GroupName:   aws.String(sg.Identifier()),
		Description: aws.String(sg.Description),
		VpcId:       vpcid,
	})
	if err != nil {
		return errors.Wrap(err, "failed creating security group")
	}

	log.WithFields(log.Fields{
		"Name":     sg.Identifier(),
		"Group ID": *respCreateSecurityGroup.GroupId,
	}).Info(color.Green("security group created"))

	// wait for the creation
	err = ec2.NewSecurityGroupExistsWaiter(ec2Client).Wait(ctx, &ec2.DescribeSecurityGroupsInput{
		GroupIds: []string{*respCreateSecurityGroup.GroupId},
	}, 30*time.Second)
	if err != nil {
		return errors.Wrap(err, "timed out waiting for security group")
	}

	// at this point we first remove the default egress rule which is created for each sg.
	_, err = ec2Client.RevokeSecurityGroupEgress(ctx, &ec2.RevokeSecurityGroupEgressInput{
		GroupId: respCreateSecurityGroup.GroupId,
		IpPermissions: []ec2types.IpPermission{{
			FromPort:   nil,
			ToPort:     nil,
			IpProtocol: aws.String(protocolAll),
			IpRanges: []ec2types.IpRange{{
				CidrIp:      aws.String("0.0.0.0/0"),
				Description: nil,
			}},
		}},
	})

	if err != nil {
		return errors.Wrap(err, "error removing default egress rule")
	}

	// Add tags
	err = awsw.NewEC2(ctx, sg.Context.ProviderName).AddResourceTags(ctx, *respCreateSecurityGroup.GroupId, sg.Tags)

	if err != nil {
		return errors.Wrapf(err, "error adding tags to security group %v", sg.Identifier())
	}

	// add ingress & egress rules/permissions
	if ingressPermissions != nil {
		log.Debug("creating sg ingress")
		_, err = ec2Client.AuthorizeSecurityGroupIngress(ctx, &ec2.AuthorizeSecurityGroupIngressInput{
			GroupId:       respCreateSecurityGroup.GroupId,
			IpPermissions: ingressPermissions,
		})
		if err != nil {
			return errors.Wrapf(err, "error creating ingress rules for %v", sg.Identifier())
		}
		log.WithFields(log.Fields{
			"Name": sg.Identifier(),
		}).Infof("%v ingress rules created for security group", len(ingressPermissions))
	}

	if egressPermissions != nil {
		log.Debug("creating sg egress")
		_, err = ec2Client.AuthorizeSecurityGroupEgress(ctx, &ec2.AuthorizeSecurityGroupEgressInput{
			GroupId:       respCreateSecurityGroup.GroupId,
			IpPermissions: egressPermissions,
		})
		if err != nil {
			return errors.Wrapf(err, "error creating egress rules for %v", sg.Identifier())
		}
		log.WithFields(log.Fields{
			"Name": sg.Identifier(),
		}).Infof("%v egress rules created for security group", len(egressPermissions))
	}

	return nil
}

// applyDiffs applies diffs to an existing security group
func (sg SecurityGroup) applyDiffs(ctx context.Context, diffs ResourceDiff) error {

	if diffs == nil {
		log.WithFields(log.Fields{
			"Name": sg.Identifier(),
		}).Info("no updates required for security group")
		return nil
	}

	sgDiffs, ok := diffs.(*SecurityGroupDiff)
	if !ok {
		return errors.New("invalid diff type supplied")
	}

	// fetch existing sg
	existing, ok := sgDiffs.Resource.(*ec2types.SecurityGroup)
	if !ok {
		return errors.Errorf("cannot retrieve existing security group")
	}

	var err error
	client := client.EC2(ctx, sg.Context.ProviderName)

	//ingress rules
	if sgDiffs.ingressDiff {
		//deletes
		if len(sgDiffs.ingressRemove) > 0 {
			_, err = client.RevokeSecurityGroupIngress(ctx, &ec2.RevokeSecurityGroupIngressInput{
				GroupId:       existing.GroupId,
				IpPermissions: sgRulesToIpPermissions(sgDiffs.ingressRemove),
			})
			if err != nil {
				return errors.Wrapf(err, "error revoking ingress rules from security group %v", sg.Identifier())
			}
			log.WithFields(log.Fields{
				"Name": sg.Identifier(),
			}).Infof("%v ingress rules removed for security group", len(sgDiffs.ingressRemove))
		}
		//updates
		if len(sgDiffs.ingressAdd) > 0 {
			_, err = client.AuthorizeSecurityGroupIngress(ctx, &ec2.AuthorizeSecurityGroupIngressInput{
				GroupId:       existing.GroupId,
				IpPermissions: sgRulesToIpPermissions(sgDiffs.ingressAdd),
			})
			if err != nil {
				return errors.Wrapf(err, "error adding ingress rules to security group %v", sg.Identifier())
			}
			log.WithFields(log.Fields{
				"Name": sg.Identifier(),
			}).Infof("%v ingress rules added for security group", len(sgDiffs.ingressAdd))
		}
	}

	//egress rules
	if sgDiffs.egressDiff {
		//deletes
		if len(sgDiffs.egressRemove) > 0 {
			_, err = client.RevokeSecurityGroupEgress(ctx, &ec2.RevokeSecurityGroupEgressInput{
				GroupId:       existing.GroupId,
				IpPermissions: sgRulesToIpPermissions(sgDiffs.egressRemove),
			})
			if err != nil {
				return errors.Wrapf(err, "error revoking egress rules from security group %v", sg.Identifier())
			}
			log.WithFields(log.Fields{
				"Name": sg.Identifier(),
			}).Infof("%v egress rules removed for security group", len(sgDiffs.egressRemove))
		}
		//updates
		if len(sgDiffs.egressAdd) > 0 {
			_, err = client.AuthorizeSecurityGroupEgress(ctx, &ec2.AuthorizeSecurityGroupEgressInput{
				GroupId:       existing.GroupId,
				IpPermissions: sgRulesToIpPermissions(sgDiffs.egressAdd),
			})
			if err != nil {
				return errors.Wrapf(err, "error adding egress rules from security group %v", sg.Identifier())
			}
			log.WithFields(log.Fields{
				"Name": sg.Identifier(),
			}).Infof("%v egress rules added for security group", len(sgDiffs.egressAdd))
		}
	}

	// tags
	if sgDiffs.tagsDiff {
		upserts := sgDiffs.tagDiff.Upserts()

		if len(upserts) > 0 {
			err = awsw.NewEC2(ctx, sg.Context.ProviderName).AddResourceTags(ctx, *existing.GroupId, upserts)
			if err != nil {
				return errors.Wrapf(err, "error updating security group tags for %v", sg.Identifier())
			}
		}

		if len(sgDiffs.tagDiff.Deleted) > 0 {
			err = awsw.NewEC2(ctx, sg.Context.ProviderName).DeleteResourceTags(ctx, *existing.GroupId, sgDiffs.tagDiff.Deleted)
			if err != nil {
				return errors.Wrapf(err, "error deleting security group tags for %v", sg.Identifier())
			}
		}
	}

	log.WithFields(log.Fields{
		"Name":     sg.Identifier(),
		"Group ID": *existing.GroupId,
	}).Info(color.Yellow("security group updated"))

	return nil
}

// waitUntilSecurityGroupDisassociated will wait until the specified security group is associated with
// a network interface. the call will fail after retrying the specified times.
func waitUntilSecurityGroupDisassociated(ctx context.Context, rctx Context, id string, maxRetries int64) error {
	ec2Client := client.EC2(ctx, rctx.ProviderName)

	for tries := 0; int64(tries) < maxRetries; tries++ {
		respInterface, err := ec2Client.DescribeNetworkInterfaces(ctx, &ec2.DescribeNetworkInterfacesInput{
			Filters: []ec2types.Filter{{
				Name:   aws.String("group-id"),
				Values: []string{id},
			}},
		})
		if err != nil {
			return errors.Wrapf(err, "error listing network interfaces that depend on security group id: %v", id)
		}
		if len(respInterface.NetworkInterfaces) == 0 {
			return nil
		}

		time.Sleep(15 * time.Second)
	}
	return errors.Errorf("security group %v is still associated with a network interface after %v retries", id, maxRetries)
}

// internal struct for comparing rules;
// holds a flat structure with a single cidr or sg for from,to ports and protocol,
// the type is indicated by `typ` field
type sgRule struct {
	from     int32
	to       int32
	protocol string
	src      SecurityGroupRuleSource
	typ      string
}

// this function will flatten sg rules by from/to port/protocol into a 1 rule per sgRule
func flattenSecurityGroupRules(ctx context.Context, rctx Context, rules []SecurityGroupRule) ([]sgRule, error) {
	var narr []sgRule
	rmap := make(map[sgRule]struct{})

	for _, r := range rules {
		f, t, err := parsePortRange(r.PortRange)
		if err != nil {
			return nil, errors.Wrap(err, "error parsing port range")
		}

		//both ports not provided & protocol is TCP or UDP
		if (f == 0 && t == 0) &&
			(strings.EqualFold(r.IpProtocol, string(ec2types.ProtocolTcp)) ||
				strings.EqualFold(r.IpProtocol, string(ec2types.ProtocolUdp))) {
			return nil, errors.New("port range must be explicitly provided when protocol is TCP/UDP")
		}

		if r.IpProtocol == "" {
			r.IpProtocol = protocolAll
		}

		for _, cidr := range r.CidrBlocks {
			val := sgRule{from: f, to: t, protocol: r.IpProtocol}
			val.typ = cidrRange
			_, _, err = net.ParseCIDR(cidr.Value)
			if err != nil {
				return nil, errors.Wrapf(err, "error parsing CIDR %s for security group rule", cidr.Value)
			}
			val.src.Value = cidr.Value
			val.src.Description = cidr.Description
			narr = append(narr, val)
			// check for duplicates
			err := isDuplicate(val, rmap)
			if err != nil {
				return nil, err
			}
		}
		for _, sg := range r.SourceSecurityGroups {
			val := sgRule{from: f, to: t, protocol: r.IpProtocol}
			val.typ = sgReference
			group, err := securityGroupByName(ctx, rctx, sg.Value)
			if err != nil {
				return nil, errors.Wrap(err, "error finding security group id")
			}
			if group == nil {
				return nil, errors.Errorf("security group %v does not exist", sg.Value)
			}
			val.src.Value = *group.GroupId
			val.src.Description = sg.Description
			val.src.accountId = *group.OwnerId
			narr = append(narr, val)
			// check for duplicates
			err = isDuplicate(val, rmap)
			if err != nil {
				return nil, err
			}
		}
	}

	return narr, nil
}

// isDuplicate creates a copy, by exlucding a security group rule description & checks if a
// matching rule already exists, else just adds it to the set of rules
func isDuplicate(rule sgRule, rmap map[sgRule]struct{}) error {
	copy := sgRule{rule.from, rule.to, rule.protocol, SecurityGroupRuleSource{rule.src.Value, "", rule.src.accountId}, rule.typ}
	if _, ok := rmap[copy]; ok {
		log.Warnf("duplicate rule defined with portRange: %v-%v,  Protocol: %v, Source:%v", rule.from, rule.to, rule.protocol, rule.src)
	} else {
		rmap[copy] = struct{}{}
	}
	return nil
}

// flattenAwsIpPermission will flatten AWS sg rules by from/to port/protocol into a 1 rule per group as []sgRule
func flattenAwsIpPermission(rules []ec2types.IpPermission) ([]sgRule, error) {

	narr := make([]sgRule, 0)

	for _, r := range rules {
		var f, t int32
		if r.FromPort != nil {
			f = *r.FromPort
		}
		if r.ToPort != nil {
			t = *r.ToPort
		}
		p := *r.IpProtocol

		for _, cidr := range r.IpRanges {
			val := sgRule{from: f, to: t, protocol: p}
			val.typ = cidrRange
			val.src.Value = *cidr.CidrIp
			if cidr.Description != nil {
				val.src.Description = *cidr.Description
			}
			narr = append(narr, val)
		}

		for _, cidr := range r.Ipv6Ranges {
			val := sgRule{from: f, to: t, protocol: p}
			val.typ = cidrRange
			val.src.Value = *cidr.CidrIpv6
			if cidr.Description != nil {
				val.src.Description = *cidr.Description
			}
			narr = append(narr, val)
		}

		for _, sg := range r.UserIdGroupPairs {
			if sg.UserId == nil {
				log.Warnf("an existing security group reference %v is stale, will not be considered in diff and will be removed during update ", *sg.GroupId)
				sg.UserId = aws.String(staleSecurityGroupUserID)
			}
			val := sgRule{from: f, to: t, protocol: p}
			val.typ = sgReference
			val.src.Value = *sg.GroupId
			val.src.accountId = *sg.UserId
			if sg.Description != nil {
				val.src.Description = *sg.Description
			}
			narr = append(narr, val)
		}
	}

	return narr, nil
}

// diffRules finds the diffs between 2 sgRule's, returns if both rules the same, non-matching rules in left and right respectively
func diffRules(left []sgRule, right []sgRule) (bool, []sgRule, []sgRule) {

	lmap := make(map[sgRule]bool)
	for _, r := range left {
		lmap[r] = true

	}
	rmap := make(map[sgRule]bool)
	for _, r := range right {
		rmap[r] = true
	}

	lres := make([]sgRule, 0)
	rres := make([]sgRule, 0)
	for k := range lmap {
		if _, ok := rmap[k]; ok { //if l.key in r.key remove both
			delete(rmap, k)
			delete(lmap, k)
		} else {
			//if not, keep l.key in result
			lres = append(lres, k)
		}
	}
	//collect what remains in r
	for k := range rmap {
		rres = append(rres, k)
	}

	return len(lres) != 0 || len(rres) != 0, lres, rres
}

// sgRulesToIpPermissions converts []sgRule to []*ec2.IpPermissions
func sgRulesToIpPermissions(rules []sgRule) []ec2types.IpPermission {
	if len(rules) == 0 {
		return nil
	}

	var permissions []ec2types.IpPermission
	for _, rule := range rules {
		var from, to *int32
		if rule.from >= 0 {
			from = aws.Int32(rule.from)
		}
		if rule.to >= 0 {
			to = aws.Int32(rule.to)
		}

		ipRanges, ipv6Ranges := sgRuleToIpRanges(rule)
		permissions = append(permissions, ec2types.IpPermission{
			FromPort:         from,
			ToPort:           to,
			IpProtocol:       aws.String(rule.protocol),
			IpRanges:         ipRanges, //sgRuleToIpRanges(rule),
			Ipv6Ranges:       ipv6Ranges,
			UserIdGroupPairs: sgRuleToUserIdGroupPairs(rule),
		})
	}
	return permissions
}

// parsePortRange is a helper method to parse a port range
// string of the form `n` or `n - m` as a start, end & error
// if there is a problem parsing the port range, a non-nil
// error is returned
func parsePortRange(portRange string) (int32, int32, error) {
	if len(portRange) == 0 {
		return 0, 0, nil
	}

	index := strings.Index(portRange, "-")
	if index > -1 {
		from := portRange[:index]
		to := portRange[index+1:]
		fromPort, err := strconv.Atoi(strings.TrimSpace(from))
		if err != nil {
			return 0, 0, errors.Wrap(err, "invalid port range provided")
		}
		toPort, err := strconv.Atoi(strings.TrimSpace(to))
		if err != nil {
			return 0, 0, errors.Wrap(err, "invalid port range provided")
		}
		return int32(fromPort), int32(toPort), nil
	}

	port, err := strconv.Atoi(strings.TrimSpace(portRange))
	if err != nil {
		return 0, 0, errors.Wrap(err, "invalid port range provided")
	}
	return int32(port), int32(port), nil
}

// sgRulesToIpRanges convert sgRule to ec2.IpRange's
func sgRuleToIpRanges(s sgRule) ([]ec2types.IpRange, []ec2types.Ipv6Range) {

	if s.typ != cidrRange {
		return nil, nil
	}

	var description *string
	if s.src.Description != "" {
		description = aws.String(s.src.Description)
	}

	if isIpv6(s.src.Value) {
		return nil, []ec2types.Ipv6Range{{
			CidrIpv6:    aws.String(s.src.Value),
			Description: description,
		}}
	}

	return []ec2types.IpRange{{
		CidrIp:      aws.String(s.src.Value),
		Description: description,
	}}, nil
}

// isIpv6 returns true if the supplied string is a valid ipv6
func isIpv6(ipString string) bool {
	_, _, err := net.ParseCIDR(ipString)
	return err == nil && strings.Contains(ipString, ":")
}

// convert sgRule to ec2.UserIdGroupPair's
func sgRuleToUserIdGroupPairs(s sgRule) []ec2types.UserIdGroupPair {
	if s.typ != sgReference {
		return nil
	}
	var description *string
	if s.src.Description != "" {
		description = aws.String(s.src.Description)
	}
	return []ec2types.UserIdGroupPair{{
		GroupId:     aws.String(s.src.Value),
		Description: description,
		UserId:      aws.String(s.src.accountId),
	}}
}

// lookup security group by name, returns:
// group: security group when found, nil when error or if group doesn't exist
// error: when exception occurs, nil when found or doesn't exist
func securityGroupByName(ctx context.Context, rctx Context, name string) (*ec2types.SecurityGroup, error) {

	// TODO: this func supports legacy provider scopoing for security group names, esp references
	// thus we check first if the name includes a provider with a / delim, if yes, we use that instead
	// if no, we use the rctx.providerName value.
	//
	providerName := rctx.ProviderName // use the resource context provider as default
	sgName := name                    // assume the name is the security group name

	// if the name was in provider/resource_id format, then we update the above...
	if i := strings.IndexRune(name, '/'); i != -1 {
		providerName = name[:i]
		sgName = name[i+1:]
	}

	ec2Client := client.EC2(ctx, providerName)
	out, err := ec2Client.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{
		Filters: []ec2types.Filter{{
			Name:   aws.String("group-name"),
			Values: []string{sgName},
		}},
	})
	if err != nil {
		return nil, errors.Wrapf(err, "error finding security group by name %v", sgName)
	}

	if len(out.SecurityGroups) == 0 {
		return nil, nil
	}
	return &out.SecurityGroups[0], nil
}
