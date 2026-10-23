package resource

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testHostedZoneId = "Z0123456789ABCDEFGHIJ"

// testRecord returns a simple A record definition as Normalize would leave it.
func testRecord() Route53Record {
	return Route53Record{
		Name:         "www",
		HostedZone:   "example.com.",
		Type:         aws.String(RecordTypeA),
		TTL:          aws.Int64(60),
		Destinations: []string{"192.0.2.10", "192.0.2.11"},
		dnsName:      "www",
		hostedZoneId: aws.String(testHostedZoneId),
	}
}

// testRecordSet returns the AWS record set that matches testRecord.
func testRecordSet() route53types.ResourceRecordSet {
	return route53types.ResourceRecordSet{
		Name: aws.String("www.example.com."),
		Type: route53types.RRTypeA,
		TTL:  aws.Int64(60),
		ResourceRecords: []route53types.ResourceRecord{
			{Value: aws.String("192.0.2.10")},
			{Value: aws.String("192.0.2.11")},
		},
	}
}

// noAlias fails the test if the comparison tries to resolve an alias target.
func noAlias(t *testing.T) func() (*route53types.AliasTarget, error) {
	return func() (*route53types.AliasTarget, error) {
		t.Fatal("resolveAlias must not be called")
		return nil, nil
	}
}

func TestRoute53RecordCompareIdentical(t *testing.T) {
	diff, err := testRecord().compareRecordSets([]route53types.ResourceRecordSet{testRecordSet()}, noAlias(t))
	require.NoError(t, err)
	assert.Nil(t, diff)
}

func TestRoute53RecordCompareMissing(t *testing.T) {
	diff, err := testRecord().compareRecordSets(nil, noAlias(t))
	require.NoError(t, err)
	require.NotNil(t, diff)
	assert.Equal(t, []string{"route53 record does not exist"}, diff.Differences())
	assert.Nil(t, diff.Changes())
	assert.Nil(t, diff.AWSResource())
}

func TestRoute53RecordCompareChanges(t *testing.T) {
	cases := []struct {
		name     string
		desired  func(r *Route53Record)
		existing func(rr *route53types.ResourceRecordSet)
		messages []string
		changes  []Change
	}{
		{
			name: "type",
			desired: func(r *Route53Record) {
				r.Type = aws.String(RecordTypeCNAME)
				r.Destinations = []string{"origin.example.com"}
			},
			messages: []string{"record type is different A -> CNAME"},
			changes:  []Change{{Field: "recordType", From: "A", To: "CNAME"}},
		},
		{
			name:     "ttl",
			desired:  func(r *Route53Record) { r.TTL = aws.Int64(300) },
			messages: []string{"record alias ttl is different 60 -> 300"},
			changes:  []Change{{Field: "ttl", From: "60", To: "300"}},
		},
		{
			name:     "destination value",
			desired:  func(r *Route53Record) { r.Destinations = []string{"192.0.2.10", "192.0.2.20"} },
			messages: []string{"record destinations are different"},
			changes:  []Change{{Field: "destinations", From: "192.0.2.10,192.0.2.11", To: "192.0.2.10,192.0.2.20"}},
		},
		{
			name:     "destination count",
			desired:  func(r *Route53Record) { r.Destinations = []string{"192.0.2.10"} },
			messages: []string{"record destinations are different"},
			changes:  []Change{{Field: "destinations", From: "192.0.2.10,192.0.2.11", To: "192.0.2.10"}},
		},
		{
			name: "ttl and destinations",
			desired: func(r *Route53Record) {
				r.TTL = aws.Int64(300)
				r.Destinations = []string{"192.0.2.20"}
			},
			messages: []string{
				"record alias ttl is different 60 -> 300",
				"record destinations are different",
			},
			changes: []Change{
				{Field: "ttl", From: "60", To: "300"},
				{Field: "destinations", From: "192.0.2.10,192.0.2.11", To: "192.0.2.20"},
			},
		},
		{
			name: "alias added",
			desired: func(r *Route53Record) {
				r.AliasType = aws.String(AliasTypeLoadBalancer)
				r.Destinations = []string{"example-lb"}
			},
			messages: []string{"record alias target is being added"},
			changes:  []Change{{Field: "aliasType", From: "", To: AliasTypeLoadBalancer}},
		},
		{
			name: "alias removed",
			existing: func(rr *route53types.ResourceRecordSet) {
				rr.TTL = nil
				rr.ResourceRecords = nil
				rr.AliasTarget = &route53types.AliasTarget{
					HostedZoneId: aws.String("Z0123456789ABCDEFGHIJ"),
					DNSName:      aws.String("dualstack.example-lb.us-east-1.elb.example.com."),
				}
			},
			messages: []string{"record alias target is being removed"},
			changes: []Change{
				{Field: "alias.dnsName", From: "dualstack.example-lb.us-east-1.elb.example.com.", To: ""},
				{Field: "alias.hostedZoneId", From: "Z0123456789ABCDEFGHIJ", To: ""},
			},
		},
		{
			name: "weighted to simple",
			existing: func(rr *route53types.ResourceRecordSet) {
				rr.Weight = aws.Int64(10)
				rr.SetIdentifier = aws.String("example-1")
			},
			messages: []string{"existing record has weighted routing, switching to simple"},
			changes:  []Change{{Field: "routingPolicy", From: "weighted", To: "simple"}},
		},
		{
			name: "simple to weighted",
			desired: func(r *Route53Record) {
				r.Policy = &Route53RecordRoutingPolicy{SetIdentifier: "example-1", Weight: 10}
			},
			messages: []string{"existing record has simple routing, switching to weighted"},
			changes:  []Change{{Field: "routingPolicy", From: "simple", To: "weighted"}},
		},
		{
			name: "weight",
			desired: func(r *Route53Record) {
				r.Policy = &Route53RecordRoutingPolicy{SetIdentifier: "example-1", Weight: 20}
			},
			existing: func(rr *route53types.ResourceRecordSet) {
				rr.Weight = aws.Int64(10)
				rr.SetIdentifier = aws.String("example-1")
			},
			messages: []string{"record routing policy weight is different 10 -> 20"},
			changes:  []Change{{Field: "routingPolicy.weight", From: "10", To: "20"}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := testRecord()
			if tc.desired != nil {
				tc.desired(&r)
			}
			rr := testRecordSet()
			if tc.existing != nil {
				tc.existing(&rr)
			}
			existings := []route53types.ResourceRecordSet{rr}

			diff, err := r.compareRecordSets(existings, noAlias(t))
			require.NoError(t, err)
			require.NotNil(t, diff)
			assert.Equal(t, tc.messages, diff.Differences())
			assert.Equal(t, tc.changes, diff.Changes())
			assert.Equal(t, existings, diff.AWSResource())
		})
	}
}

func TestRoute53RecordCompareAliasTarget(t *testing.T) {
	existing := func(dnsName, zone string) route53types.ResourceRecordSet {
		return route53types.ResourceRecordSet{
			Name: aws.String("www.example.com."),
			Type: route53types.RRTypeA,
			AliasTarget: &route53types.AliasTarget{
				HostedZoneId: aws.String(zone),
				DNSName:      aws.String(dnsName),
			},
		}
	}
	desired := testRecord()
	desired.AliasType = aws.String(AliasTypeLoadBalancer)
	desired.Destinations = []string{"example-lb"}
	resolved := &route53types.AliasTarget{
		HostedZoneId: aws.String("Z0123456789ABCDEFGHIJ"),
		DNSName:      aws.String("dualstack.new-lb.us-east-1.elb.example.com."),
	}
	resolve := func() (*route53types.AliasTarget, error) { return resolved, nil }

	t.Run("identical", func(t *testing.T) {
		diff, err := desired.compareRecordSets(
			[]route53types.ResourceRecordSet{existing(*resolved.DNSName, *resolved.HostedZoneId)}, resolve)
		require.NoError(t, err)
		assert.Nil(t, diff)
	})

	t.Run("dns name", func(t *testing.T) {
		diff, err := desired.compareRecordSets(
			[]route53types.ResourceRecordSet{existing("dualstack.old-lb.us-east-1.elb.example.com.", *resolved.HostedZoneId)}, resolve)
		require.NoError(t, err)
		require.NotNil(t, diff)
		assert.Equal(t, []string{
			"record alias target is different Z0123456789ABCDEFGHIJ/dualstack.old-lb.us-east-1.elb.example.com.(false) -> Z0123456789ABCDEFGHIJ/dualstack.new-lb.us-east-1.elb.example.com.(false)",
		}, diff.Differences())
		assert.Equal(t, []Change{
			{Field: "alias.dnsName", From: "dualstack.old-lb.us-east-1.elb.example.com.", To: "dualstack.new-lb.us-east-1.elb.example.com."},
		}, diff.Changes())
	})

	t.Run("dns name and hosted zone", func(t *testing.T) {
		diff, err := desired.compareRecordSets(
			[]route53types.ResourceRecordSet{existing("dualstack.old-lb.us-east-1.elb.example.com.", "Z0123456789ABCDEFGHIK")}, resolve)
		require.NoError(t, err)
		require.NotNil(t, diff)
		assert.Len(t, diff.Differences(), 1)
		assert.Equal(t, []Change{
			{Field: "alias.dnsName", From: "dualstack.old-lb.us-east-1.elb.example.com.", To: "dualstack.new-lb.us-east-1.elb.example.com."},
			{Field: "alias.hostedZoneId", From: "Z0123456789ABCDEFGHIK", To: "Z0123456789ABCDEFGHIJ"},
		}, diff.Changes())
	})

	t.Run("evaluate target health", func(t *testing.T) {
		healthChecked := &route53types.AliasTarget{
			HostedZoneId: resolved.HostedZoneId, DNSName: resolved.DNSName, EvaluateTargetHealth: true,
		}
		diff, err := desired.compareRecordSets(
			[]route53types.ResourceRecordSet{existing(*resolved.DNSName, *resolved.HostedZoneId)},
			func() (*route53types.AliasTarget, error) { return healthChecked, nil })
		require.NoError(t, err)
		require.NotNil(t, diff)
		assert.Len(t, diff.Differences(), 1)
		assert.Equal(t, []Change{
			{Field: "alias.evaluateTargetHealth", From: "false", To: "true"},
		}, diff.Changes())
	})

	t.Run("resolve error", func(t *testing.T) {
		diff, err := desired.compareRecordSets(
			[]route53types.ResourceRecordSet{existing("dualstack.old-lb.us-east-1.elb.example.com.", *resolved.HostedZoneId)},
			func() (*route53types.AliasTarget, error) { return nil, errors.New("not found") })
		require.Error(t, err)
		assert.Contains(t, err.Error(), "error converting referenced resource to alias target")
		assert.Nil(t, diff)
	})
}

func TestRoute53RecordDiffIsDetailed(t *testing.T) {
	var _ DetailedResourceDiff = &Route53RecordDiff{}
}
