package resource

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testSecurityGroupID    = "sg-0123456789abcdef0"
	testSecurityGroupVPCID = "vpc-0123456789abcdef0"
)

// securityGroupRuleSetsFor flattens the definition's and the AWS group's rules
// the way Compare does. CIDR-only rules never reach AWS, so this stays pure.
func securityGroupRuleSetsFor(t *testing.T, sg SecurityGroup, existing *ec2types.SecurityGroup) securityGroupRuleSets {
	t.Helper()
	var rules securityGroupRuleSets
	var err error
	rules.newIngress, err = flattenSecurityGroupRules(context.Background(), Context{}, sg.InboundRules)
	require.NoError(t, err)
	rules.oldIngress, err = flattenAwsIpPermission(existing.IpPermissions)
	require.NoError(t, err)
	rules.newEgress, err = flattenSecurityGroupRules(context.Background(), Context{}, sg.OutboundRules)
	require.NoError(t, err)
	rules.oldEgress, err = flattenAwsIpPermission(existing.IpPermissionsEgress)
	require.NoError(t, err)
	return rules
}

func cidrPermission(protocol string, port int32, cidr string) ec2types.IpPermission {
	return ec2types.IpPermission{
		IpProtocol: aws.String(protocol),
		FromPort:   aws.Int32(port),
		ToPort:     aws.Int32(port),
		IpRanges:   []ec2types.IpRange{{CidrIp: aws.String(cidr)}},
	}
}

func cidrRule(protocol, portRange, cidr string) SecurityGroupRule {
	return SecurityGroupRule{
		IpProtocol: protocol,
		PortRange:  portRange,
		CidrBlocks: []SecurityGroupRuleSource{{Value: cidr}},
	}
}

func TestSecurityGroupDiffIsDetailed(t *testing.T) {
	var _ DetailedResourceDiff = &SecurityGroupDiff{}
}

func TestCompareSecurityGroup_RecordsChanges(t *testing.T) {
	existing := &ec2types.SecurityGroup{
		GroupId:     aws.String(testSecurityGroupID),
		Description: aws.String("old description"),
		VpcId:       aws.String(testSecurityGroupVPCID),
		IpPermissions: []ec2types.IpPermission{
			cidrPermission("tcp", 443, "10.0.0.0/8"),
			cidrPermission("tcp", 80, "10.0.0.0/8"),
		},
	}
	sg := SecurityGroup{
		Description: "new description",
		VPCName:     "example-vpc",
		InboundRules: []SecurityGroupRule{
			cidrRule("tcp", "443", "10.0.0.0/8"),
			cidrRule("tcp", "5432", "192.0.2.0/24"),
		},
		Tags: map[string]string{"env": "new"},
	}
	awsTags := map[string]string{"env": "old"}

	diffs := compareSecurityGroup(context.Background(), sg, existing, testSecurityGroupVPCID, securityGroupRuleSetsFor(t, sg, existing), awsTags)

	assert.Equal(t, []Change{
		{Field: "description", From: "old description", To: "new description"},
		{Field: "inboundRules", To: "tcp 5432-5432 cidr:192.0.2.0/24"},
		{Field: "inboundRules", From: "tcp 80-80 cidr:10.0.0.0/8"},
		{Field: "tags.env", From: "old", To: "new"},
	}, diffs.Changes())

	assert.Equal(t, []string{
		"security group description is not the same",
		"ingress rules are not the same",
		"security group rule to be added port range:5432-5432, protocol:tcp, src:192.0.2.0/24, type:cidr",
		"security group rule to be removed port range:80-80, protocol:tcp, src:10.0.0.0/8, type:cidr",
		"tag env is different: old -> new",
	}, diffs.Differences())

	assert.True(t, diffs.ingressDiff)
	assert.False(t, diffs.egressDiff)
	assert.True(t, diffs.tagsDiff)
	assert.Same(t, existing, diffs.Resource)
}

func TestCompareSecurityGroup_VpcAndEgress(t *testing.T) {
	existing := &ec2types.SecurityGroup{
		GroupId:     aws.String(testSecurityGroupID),
		Description: aws.String("example description"),
		VpcId:       aws.String("vpc-0fedcba9876543210"),
		IpPermissionsEgress: []ec2types.IpPermission{
			cidrPermission("tcp", 443, "10.0.0.0/8"),
		},
	}
	sg := SecurityGroup{
		Description: "example description",
		VPCName:     "example-vpc",
		OutboundRules: []SecurityGroupRule{
			cidrRule("tcp", "443", "10.0.0.0/8"),
			cidrRule("udp", "53", "192.0.2.0/24"),
		},
	}

	diffs := compareSecurityGroup(context.Background(), sg, existing, testSecurityGroupVPCID, securityGroupRuleSetsFor(t, sg, existing), nil)

	assert.Equal(t, []Change{
		{Field: "vpcName", From: "vpc-0fedcba9876543210", To: testSecurityGroupVPCID},
		{Field: "outboundRules", To: "udp 53-53 cidr:192.0.2.0/24"},
	}, diffs.Changes())

	assert.Equal(t, []string{
		"security group vpcid is different",
		"egress rules are not the same",
	}, diffs.Differences())

	assert.False(t, diffs.ingressDiff)
	assert.True(t, diffs.egressDiff)
	assert.False(t, diffs.tagsDiff)
}

func TestCompareSecurityGroup_Identical(t *testing.T) {
	existing := &ec2types.SecurityGroup{
		GroupId:       aws.String(testSecurityGroupID),
		Description:   aws.String("example description"),
		VpcId:         aws.String(testSecurityGroupVPCID),
		IpPermissions: []ec2types.IpPermission{cidrPermission("tcp", 443, "10.0.0.0/8")},
		IpPermissionsEgress: []ec2types.IpPermission{{
			IpProtocol: aws.String(protocolAll),
			IpRanges:   []ec2types.IpRange{{CidrIp: aws.String("0.0.0.0/0")}},
		}},
	}
	sg := SecurityGroup{
		Description:   "example description",
		VPCName:       "example-vpc",
		InboundRules:  []SecurityGroupRule{cidrRule("tcp", "443", "10.0.0.0/8")},
		OutboundRules: []SecurityGroupRule{cidrRule("", "", "0.0.0.0/0")},
		Tags:          map[string]string{"env": "example"},
	}
	awsTags := map[string]string{"env": "example"}

	diffs := compareSecurityGroup(context.Background(), sg, existing, testSecurityGroupVPCID, securityGroupRuleSetsFor(t, sg, existing), awsTags)

	assert.Empty(t, diffs.Differences())
	assert.Nil(t, diffs.Changes())
	assert.False(t, diffs.ingressDiff)
	assert.False(t, diffs.egressDiff)
	assert.False(t, diffs.tagsDiff)
}
