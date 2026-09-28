package awsw

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func zone(id, name string, private bool) route53types.HostedZone {
	return route53types.HostedZone{
		Id:     aws.String("/hostedzone/" + id),
		Name:   aws.String(name),
		Config: &route53types.HostedZoneConfig{PrivateZone: private},
	}
}

// pickHostedZoneForRecord decides which hosted zone receives an ACM validation CNAME when
// dnsValidationDomainName is omitted (DEVOPS-8968). Wrong zone = record nobody resolves and
// a certificate stuck in PENDING_VALIDATION, so the policy is pinned here.
func TestPickHostedZoneForRecord(t *testing.T) {
	apex := zone("ZAPEX", "example.com.", false)
	nested := zone("ZNESTED", "service.example.com.", false)
	other := zone("ZOTHER", "example.io.", false)
	privNested := zone("ZPRIV", "service.example.com.", true)

	tests := []struct {
		name    string
		zones   []route53types.HostedZone
		record  string
		wantID  string
		wantErr string
	}{
		{
			name:   "single zone owns the record",
			zones:  []route53types.HostedZone{apex},
			record: "_abc.api.example.com.",
			wantID: "/hostedzone/ZAPEX",
		},
		{
			name:   "nested zones: the longest suffix wins regardless of listing order",
			zones:  []route53types.HostedZone{apex, nested},
			record: "_abc.uuid.service.example.com.",
			wantID: "/hostedzone/ZNESTED",
		},
		{
			name:   "nested zones: a record directly under the apex ignores the nested zone",
			zones:  []route53types.HostedZone{nested, apex},
			record: "_abc.api.example.com.",
			wantID: "/hostedzone/ZAPEX",
		},
		{
			name:   "record for the zone apex itself matches the zone",
			zones:  []route53types.HostedZone{apex},
			record: "example.com.",
			wantID: "/hostedzone/ZAPEX",
		},
		{
			name:   "suffix match is on a label boundary",
			zones:  []route53types.HostedZone{zone("ZNOT", "notexample.com.", false), apex},
			record: "_abc.example.com.",
			wantID: "/hostedzone/ZAPEX",
		},
		{
			name:   "SAN in another zone resolves to that zone",
			zones:  []route53types.HostedZone{apex, other},
			record: "_abc.api.example.io.",
			wantID: "/hostedzone/ZOTHER",
		},
		{
			name:   "private zone with a longer suffix is skipped in favour of the public apex",
			zones:  []route53types.HostedZone{privNested, apex},
			record: "_abc.uuid.service.example.com.",
			wantID: "/hostedzone/ZAPEX",
		},
		{
			name:   "case and trailing dot are normalized",
			zones:  []route53types.HostedZone{zone("ZAPEX", "Example.COM", false)},
			record: "_abc.API.example.com",
			wantID: "/hostedzone/ZAPEX",
		},
		{
			name:   "a zone with no Config block is treated as public",
			zones:  []route53types.HostedZone{{Id: aws.String("/hostedzone/ZNOCFG"), Name: aws.String("example.com.")}},
			record: "_abc.example.com.",
			wantID: "/hostedzone/ZNOCFG",
		},
		{
			name:    "only private zones match",
			zones:   []route53types.HostedZone{privNested, zone("ZPRIVAPEX", "example.com.", true)},
			record:  "_abc.uuid.service.example.com.",
			wantErr: "only private hosted zone(s) [example.com. service.example.com.] match validation record \"_abc.uuid.service.example.com.\"",
		},
		{
			name:    "no zone matches",
			zones:   []route53types.HostedZone{apex},
			record:  "_abc.api.example.io.",
			wantErr: "no hosted zone in this account owns validation record \"_abc.api.example.io.\"",
		},
		{
			name:    "duplicate public zone names are ambiguous",
			zones:   []route53types.HostedZone{apex, zone("ZAPEX2", "example.com.", false)},
			record:  "_abc.api.example.com.",
			wantErr: "2 hosted zones named \"example.com.\" could own validation record \"_abc.api.example.com.\" ([ZAPEX ZAPEX2])",
		},
		{
			name:   "duplicate names on a shorter suffix do not block a unique longer match",
			zones:  []route53types.HostedZone{apex, zone("ZAPEX2", "example.com.", false), nested},
			record: "_abc.uuid.service.example.com.",
			wantID: "/hostedzone/ZNESTED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := pickHostedZoneForRecord(tt.zones, tt.record)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, tt.wantID, aws.ToString(got.Id))
		})
	}
}

// The per-process listing cache is what keeps discovery to one ListHostedZones per provider per
// run. A cache hit must be served without touching the client at all — here the wrapper carries a
// nil client, so any slow-path call would panic — and the result must be a copy, not a pointer
// into the cached slice.
func TestRoute53_FindHostedZoneForRecord_ServedFromCache(t *testing.T) {
	resetHostedZoneCache()
	t.Cleanup(resetHostedZoneCache)

	hostedZoneCache.Lock()
	hostedZoneCache.zones["example-provider"] = []route53types.HostedZone{
		zone("ZAPEX", "example.com.", false),
		zone("ZNESTED", "service.example.com.", false),
	}
	hostedZoneCache.Unlock()

	r := Route53{providerName: "example-provider"}
	hz, err := r.FindHostedZoneForRecord(context.Background(), "_abc.uuid.service.example.com.")
	require.NoError(t, err)
	assert.Equal(t, "/hostedzone/ZNESTED", aws.ToString(hz.Id))

	hz.Name = aws.String("mutated.")
	hostedZoneCache.Lock()
	assert.Equal(t, "service.example.com.", aws.ToString(hostedZoneCache.zones["example-provider"][1].Name))
	hostedZoneCache.Unlock()
}
