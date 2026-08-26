package awsw

import (
	"context"

	"fmt"
	"strings"

	"github.com/TouchBistro/awesome/providers"
	"github.com/TouchBistro/buildit/client"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

type EC2 struct {
	*ec2.Client
}

// NewEC2 creates a new instance of EC2 wrapper
func NewEC2(ctx context.Context, providerName string) EC2 {
	return EC2{client.EC2(ctx, providerName)}
}

// VpcArnForIdentifier resolves a VPC ARN from an identifier (ARN, ID, or Name tag).
func (e EC2) VpcArnForIdentifier(ctx context.Context, identifier string) (*string, error) {
	resource, provider := ParseIdentifier(identifier)
	if strings.HasPrefix(resource, "arn:") {
		return &resource, nil
	}

	ec2Service := e
	if provider != "" {
		if _, err := providers.Get(provider); err != nil {
			return nil, fmt.Errorf("provider %q not found: %w", provider, err)
		}
		ec2Service = NewEC2(ctx, provider)
	}

	// Tier 1: Try by ID
	if strings.HasPrefix(resource, "vpc-") {
		out, err := ec2Service.DescribeVpcs(ctx, &ec2.DescribeVpcsInput{
			VpcIds: []string{resource},
		})
		if err == nil && len(out.Vpcs) == 1 {
			vpc := out.Vpcs[0]
			region := ec2Service.Options().Region
			arn := fmt.Sprintf("arn:aws:ec2:%s:%s:vpc/%s", region, aws.ToString(vpc.OwnerId), aws.ToString(vpc.VpcId))
			return &arn, nil
		}
	}

	// Tier 2: Try by Name Tag
	out, err := ec2Service.DescribeVpcs(ctx, &ec2.DescribeVpcsInput{
		Filters: []ec2types.Filter{
			{
				Name:   aws.String("tag:Name"),
				Values: []string{resource},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe VPCs by name %q: %w", resource, err)
	}

	if len(out.Vpcs) == 0 {
		return nil, fmt.Errorf("no VPC found for identifier %q", resource)
	}
	if len(out.Vpcs) > 1 {
		return nil, fmt.Errorf("ambiguous VPC identifier %q: found %d matches", resource, len(out.Vpcs))
	}

	vpc := out.Vpcs[0]
	region := ec2Service.Options().Region
	arn := fmt.Sprintf("arn:aws:ec2:%s:%s:vpc/%s", region, aws.ToString(vpc.OwnerId), aws.ToString(vpc.VpcId))
	return &arn, nil
}

// SubnetArnForIdentifier resolves a subnet ARN from an identifier (ARN, ID, or Name tag).
func (e EC2) SubnetArnForIdentifier(ctx context.Context, identifier string) (*string, error) {
	resource, provider := ParseIdentifier(identifier)
	if strings.HasPrefix(resource, "arn:") {
		return &resource, nil
	}

	ec2Service := e
	if provider != "" {
		if _, err := providers.Get(provider); err != nil {
			return nil, fmt.Errorf("provider %q not found: %w", provider, err)
		}
		ec2Service = NewEC2(ctx, provider)
	}

	// Tier 1: Try by ID
	if strings.HasPrefix(resource, "subnet-") {
		out, err := ec2Service.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{
			SubnetIds: []string{resource},
		})
		if err == nil && len(out.Subnets) == 1 {
			subnet := out.Subnets[0]
			region := ec2Service.Options().Region
			arn := fmt.Sprintf("arn:aws:ec2:%s:%s:subnet/%s", region, aws.ToString(subnet.OwnerId), aws.ToString(subnet.SubnetId))
			return &arn, nil
		}
	}

	// Tier 2: Try by Name Tag
	out, err := ec2Service.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{
		Filters: []ec2types.Filter{
			{
				Name:   aws.String("tag:Name"),
				Values: []string{resource},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe subnets by name %q: %w", resource, err)
	}

	if len(out.Subnets) == 0 {
		return nil, fmt.Errorf("no subnet found for identifier %q", resource)
	}
	if len(out.Subnets) > 1 {
		return nil, fmt.Errorf("ambiguous subnet identifier %q: found %d matches", resource, len(out.Subnets))
	}

	subnet := out.Subnets[0]
	region := ec2Service.Options().Region
	arn := fmt.Sprintf("arn:aws:ec2:%s:%s:subnet/%s", region, aws.ToString(subnet.OwnerId), aws.ToString(subnet.SubnetId))
	return &arn, nil
}

// SecurityGroupArnForIdentifier resolves a security group ARN from an identifier (ARN, ID, GroupName, or Name tag).
func (e EC2) SecurityGroupArnForIdentifier(ctx context.Context, identifier string) (*string, error) {
	resource, provider := ParseIdentifier(identifier)
	if strings.HasPrefix(resource, "arn:") {
		return &resource, nil
	}

	ec2Service := e
	if provider != "" {
		if _, err := providers.Get(provider); err != nil {
			return nil, fmt.Errorf("provider %q not found: %w", provider, err)
		}
		ec2Service = NewEC2(ctx, provider)
	}

	// Tier 1: Try by ID
	if strings.HasPrefix(resource, "sg-") {
		out, err := ec2Service.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{
			GroupIds: []string{resource},
		})
		if err == nil && len(out.SecurityGroups) == 1 {
			sg := out.SecurityGroups[0]
			region := ec2Service.Options().Region
			arn := fmt.Sprintf("arn:aws:ec2:%s:%s:security-group/%s", region, aws.ToString(sg.OwnerId), aws.ToString(sg.GroupId))
			return &arn, nil
		}
	}

	// Tier 2: Try by Group Name
	out, err := ec2Service.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{
		Filters: []ec2types.Filter{
			{
				Name:   aws.String("group-name"),
				Values: []string{resource},
			},
		},
	})
	if err == nil && len(out.SecurityGroups) == 1 {
		sg := out.SecurityGroups[0]
		region := ec2Service.Options().Region
		arn := fmt.Sprintf("arn:aws:ec2:%s:%s:security-group/%s", region, aws.ToString(sg.OwnerId), aws.ToString(sg.GroupId))
		return &arn, nil
	}

	// Tier 3: Try by Name Tag
	out, err = ec2Service.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{
		Filters: []ec2types.Filter{
			{
				Name:   aws.String("tag:Name"),
				Values: []string{resource},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe security groups by name %q: %w", resource, err)
	}

	if len(out.SecurityGroups) == 0 {
		return nil, fmt.Errorf("no security group found for identifier %q", resource)
	}
	if len(out.SecurityGroups) > 1 {
		return nil, fmt.Errorf("ambiguous security group identifier %q: found %d matches", resource, len(out.SecurityGroups))
	}

	sg := out.SecurityGroups[0]
	region := ec2Service.Options().Region
	arn := fmt.Sprintf("arn:aws:ec2:%s:%s:security-group/%s", region, aws.ToString(sg.OwnerId), aws.ToString(sg.GroupId))
	return &arn, nil
}

// GetResourceTags returns the tags for the ECS resource or error
func (e EC2) GetResourceTags(ctx context.Context, resourceId string) (map[string]string, error) {

	var done bool
	var token *string
	t := make(map[string]string)

	for !done {
		out, err := e.DescribeTags(ctx, &ec2.DescribeTagsInput{
			NextToken: token,
			Filters: []ec2types.Filter{
				{
					Name:   aws.String("resource-id"),
					Values: []string{resourceId},
				},
			},
		})

		if err != nil {
			return nil, err
		}

		for _, tag := range out.Tags {
			if tag.Key != nil && tag.Value != nil {
				t[*tag.Key] = *tag.Value
			}
		}

		done = out.NextToken == nil
		token = out.NextToken
	}

	return t, nil
}

// AddResourceTags tags the EC2 resourse with the supplied tag keys/value, returns error if thhe
// operation fails
func (e EC2) AddResourceTags(ctx context.Context, resourceId string, tags map[string]string) error {

	if len(tags) > 0 {

		var awsTags []ec2types.Tag
		for k, v := range tags {
			awsTags = append(awsTags, ec2types.Tag{
				Key:   aws.String(k),
				Value: aws.String(v),
			})
		}
		_, err := e.CreateTags(ctx, &ec2.CreateTagsInput{
			Resources: []string{resourceId},
			Tags:      awsTags,
		})
		if err != nil {
			return errors.Wrapf(err, "failed to update tags for resource %s", resourceId)
		}
		log.WithFields(log.Fields{
			"ResourceId": resourceId,
		}).Infof("%v tags added to EC2 resouce", len(tags))
	}

	return nil
}

// DeleteResourceTags removes the supplied tag keys from the EC2 resource, or returns an error
// if the oepration fails
func (e EC2) DeleteResourceTags(ctx context.Context, resourceId string, tags map[string]string) error {

	if len(tags) > 0 {
		var awsTags []ec2types.Tag
		for k := range tags {
			awsTags = append(awsTags, ec2types.Tag{
				Key: aws.String(k),
			})
		}
		_, err := e.DeleteTags(ctx, &ec2.DeleteTagsInput{
			Resources: []string{resourceId},
			Tags:      awsTags,
		})
		if err != nil {
			return errors.Wrapf(err, "failed to update tags for resource %s", resourceId)
		}
		log.WithFields(log.Fields{
			"ResourceId": resourceId,
		}).Infof("%v tags deleted from resource", len(tags))
	}
	return nil
}

// utility methods for the EC2 aws client

// DescribeVpcByName returns vpc id for the supplied Vpc name, else a non-nil error
func (e EC2) VpcIdByName(ctx context.Context, name string) (*string, error) {
	out, err := e.DescribeVpcs(ctx, &ec2.DescribeVpcsInput{
		Filters: []ec2types.Filter{
			{
				Name:   aws.String("tag:Name"),
				Values: []string{name},
			},
		},
	})

	if err != nil {
		return nil, errors.Wrap(err, "failed listing vpc by name")
	}

	if len(out.Vpcs) != 1 {
		return nil, errors.Errorf("describing vpc %v by name had zero or more than one match", name)
	}

	return out.Vpcs[0].VpcId, nil
}

// VpcIdBySubnetName returns the vpc id for the supplied subnet name, else a non-nil error
func (e EC2) VpcIdBySubnetName(ctx context.Context, subnetName string) (*string, error) {

	out, err := e.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{
		Filters: []ec2types.Filter{
			{
				Name:   aws.String("tag:Name"),
				Values: []string{subnetName},
			},
		},
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed listing subnets")
	}

	subnets := out.Subnets
	if len(subnets) != 1 {
		return nil, errors.New("describing subnets did not match names")
	}

	return subnets[0].VpcId, nil
}

// VpcNameById returns vpc name from supplied id, else a non-nil error
func (e EC2) VpcNameById(ctx context.Context, id string) (*string, error) {
	out, err := e.DescribeVpcs(ctx, &ec2.DescribeVpcsInput{
		VpcIds: []string{id},
	})

	if err != nil {
		return nil, errors.Wrap(err, "failed listing vpc by id")
	}

	if len(out.Vpcs) != 1 {
		return nil, errors.Errorf("describing vpc %v by name had zero or more than one match", id)
	}

	vpcName := *out.Vpcs[0].VpcId // start as Ids
	vpcTags := out.Vpcs[0].Tags
	for _, t := range vpcTags {
		if *t.Key == "Name" {
			vpcName = *t.Value
			break

		}
	}
	return &vpcName, nil
}

// SubnetIdsByName returns the subnet ids for names
func (e EC2) SubnetIdsByNames(ctx context.Context, names []string) ([]string, error) {
	if len(names) == 0 {
		return nil, nil
	}

	subnets, err := e.SubnetByNames(ctx, names)
	if err != nil {
		return nil, err
	}

	if len(subnets) != len(names) {
		return nil, errors.New("describing subnets did not match names")
	}

	var subnetIds []string
	for _, s := range subnets {
		if s.SubnetId != nil {
			subnetIds = append(subnetIds, *s.SubnetId)
		}
	}
	return subnetIds, nil
}

// SubnetByNames returns the subnets for names
func (e EC2) SubnetByNames(ctx context.Context, names []string) ([]ec2types.Subnet, error) {
	if len(names) == 0 {
		return nil, nil
	}

	var nextToken *string
	var subnets []ec2types.Subnet
	for {
		out, err := e.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{
			NextToken: nextToken,
			Filters: []ec2types.Filter{
				{
					Name:   aws.String("tag:Name"),
					Values: names,
				},
			},
		})
		if err != nil {
			return nil, errors.Wrap(err, "failed listing subnets")
		}

		subnets = append(subnets, out.Subnets...)
		if out.NextToken == nil {
			break
		}
		nextToken = out.NextToken
	}

	return subnets, nil
}

// SubnetNamesByIds returns subnet names from the ids
func (e EC2) SubnetNamesByIds(ctx context.Context, ids []string) ([]string, error) {

	if len(ids) == 0 {
		return nil, nil
	}

	var nextToken *string
	var subnets []ec2types.Subnet
	for {
		out, err := e.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{
			NextToken: nextToken,
			SubnetIds: ids,
		})
		if err != nil {
			return nil, errors.Wrap(err, "failed listing subnets")
		}

		subnets = append(subnets, out.Subnets...)
		if out.NextToken == nil {
			break
		}
		nextToken = out.NextToken
	}
	if len(subnets) != len(ids) {
		return nil, errors.New("describing subnets did not match names")
	}

	var subnetNames []string
	for _, s := range subnets {
		subnetName := aws.ToString(s.SubnetId)
		for _, t := range s.Tags {
			if aws.ToString(t.Key) == "Name" {
				subnetName = aws.ToString(t.Value)
				break
			}
		}
		subnetNames = append(subnetNames, subnetName)
	}
	return subnetNames, nil
}

// SecurityGroupIdsByNames returns security group Ids for the supplied vpc & security group names
func (e EC2) SecurityGroupIdsByNames(ctx context.Context, vpcId *string, names []string) ([]string, error) {

	if len(names) == 0 {
		return nil, nil
	}

	filters := []ec2types.Filter{
		{
			Name:   aws.String("group-name"),
			Values: names,
		},
	}

	if vpcId != nil {
		filters = append(filters, ec2types.Filter{
			Name:   aws.String("vpc-id"),
			Values: []string{*vpcId},
		})
	}

	var nextToken *string
	var securityGroups []ec2types.SecurityGroup
	for {
		out, err := e.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{
			NextToken: nextToken,
			Filters:   filters,
		})

		if err != nil {
			return nil, errors.Wrap(err, "failed listing security groups")
		}

		securityGroups = append(securityGroups, out.SecurityGroups...)
		if out.NextToken == nil {
			break
		}
		nextToken = out.NextToken
	}

	if len(securityGroups) != len(names) {
		return nil, errors.New("describing security groups did not match names")
	}

	var securityGroupIDs []string
	for _, sg := range securityGroups {
		if sg.GroupId != nil {
			securityGroupIDs = append(securityGroupIDs, *sg.GroupId)
		}
	}

	return securityGroupIDs, nil
}

// SecurityGroupNamesByIds returns security group Ids for the supplied vpc & security group names
func (e EC2) SecurityGroupNamesByIds(ctx context.Context, vpcName *string, ids []string) ([]string, error) {

	if len(ids) == 0 {
		return nil, nil
	}

	filters := []ec2types.Filter{}

	if vpcName != nil {
		vpcId, err := e.VpcIdByName(ctx, *vpcName)
		if err != nil {
			return nil, errors.Wrapf(err, "error fetching vpc %s", *vpcName)
		}

		filters = append(filters, ec2types.Filter{
			Name:   aws.String("vpc-id"),
			Values: []string{aws.ToString(vpcId)},
		})

	}

	var nextToken *string
	var securityGroups []ec2types.SecurityGroup
	for {
		out, err := e.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{
			NextToken: nextToken,
			GroupIds:  ids,
			Filters:   filters,
		})

		if err != nil {
			return nil, errors.Wrap(err, "failed listing security groups")
		}

		securityGroups = append(securityGroups, out.SecurityGroups...)
		if out.NextToken == nil {
			break
		}
		nextToken = out.NextToken
	}

	if len(securityGroups) != len(ids) {
		return nil, errors.New("describing security groups did not match names")
	}

	var securityGroupNames []string
	for _, sg := range securityGroups {
		if sg.GroupName != nil {
			securityGroupNames = append(securityGroupNames, *sg.GroupName)
		}
	}

	return securityGroupNames, nil
}
