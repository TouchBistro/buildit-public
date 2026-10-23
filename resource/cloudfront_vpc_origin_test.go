package resource

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	cftypes "github.com/aws/aws-sdk-go-v2/service/cloudfront/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

const testLBArn = "arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/internal-alb/50dc6c495c0c9188"

// The DAG collects waitables via a runtime type assertion (config/dag.go), so a
// signature drift would silently drop the deploy wait — pin the contract here.
// CloudfrontVpcOrigin is deliberately NOT waitable: its deploy wait must stay inline
// in Apply because AWS rejects associating a Deploying VPC origin with a distribution.
var _ WaitableResource = CloudfrontDistribution{}

// testVpcOrigin returns a normalized baseline VPC origin. The target is a full load
// balancer ARN so generateEndpointConfig performs no AWS lookups.
func testVpcOrigin() *CloudfrontVpcOrigin {
	r := &CloudfrontVpcOrigin{
		Name:   "test-vpc-origin",
		Target: testLBArn,
	}
	r.Normalize(context.Background())
	return r
}

func TestCloudfrontVpcOrigin_Normalize(t *testing.T) {
	r := testVpcOrigin()

	assert.Equal(t, int32(80), aws.ToInt32(r.HTTPPort))
	assert.Equal(t, int32(443), aws.ToInt32(r.HTTPSPort))
	assert.Equal(t, CloudfrontOriginProtocolHTTPSOnly, aws.ToString(r.OriginProtocolPolicy))
	assert.Equal(t, "TLSv1.2", aws.ToString(r.OriginSSLProtocol))
}

func TestCloudfrontVpcOrigin_Normalize_SslProtocolCaseInsensitive(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"tlsv1.2", "TLSv1.2"},
		{"TLSV1.1", "TLSv1.1"},
		{"sslv3", "SSLv3"},
		{"TlSv1", "TLSv1"},
		{"TLSv1.3", "TLSv1.3"}, // invalid: left verbatim for Validate to report
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			r := &CloudfrontVpcOrigin{
				Name:              "test-vpc-origin",
				Target:            testLBArn,
				OriginSSLProtocol: aws.String(tt.in),
			}
			r.Normalize(context.Background())
			assert.Equal(t, tt.want, aws.ToString(r.OriginSSLProtocol))
		})
	}
}

func TestCloudfrontVpcOrigin_Normalize_MergesTags(t *testing.T) {
	r := &CloudfrontVpcOrigin{
		Name:       "test-vpc-origin",
		Target:     testLBArn,
		Tags:       map[string]string{"Environment": "production"},
		GlobalTags: map[string]string{"Owner": "team-a", "Environment": "should-not-override"},
	}
	r.Normalize(context.Background())
	assert.Equal(t, "production", r.Tags["Environment"])
	assert.Equal(t, "team-a", r.Tags["Owner"])
}

func TestCloudfrontVpcOrigin_Normalize_GlobalTagsOnly(t *testing.T) {
	r := &CloudfrontVpcOrigin{
		Name:       "test-vpc-origin",
		Target:     testLBArn,
		GlobalTags: map[string]string{"Owner": "team-a"},
	}
	r.Normalize(context.Background())
	assert.Equal(t, map[string]string{"Owner": "team-a"}, r.Tags)
}

func TestCloudfrontVpcOrigin_Validate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*CloudfrontVpcOrigin)
		wantErr string
	}{
		{
			name:   "valid",
			mutate: func(r *CloudfrontVpcOrigin) {},
		},
		{
			name:    "missing target",
			mutate:  func(r *CloudfrontVpcOrigin) { r.Target = "" },
			wantErr: "target (load balancer ARN or name) is required",
		},
		{
			name:    "invalid origin protocol policy",
			mutate:  func(r *CloudfrontVpcOrigin) { r.OriginProtocolPolicy = aws.String("both") },
			wantErr: `invalid originProtocolPolicy "both"`,
		},
		{
			name:    "invalid ssl protocol",
			mutate:  func(r *CloudfrontVpcOrigin) { r.OriginSSLProtocol = aws.String("TLSv1.3") },
			wantErr: `invalid originSslProtocol "TLSv1.3"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := testVpcOrigin()
			tt.mutate(r)
			err := r.Validate(context.Background())
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestCloudfrontVpcOrigin_GenerateEndpointConfig(t *testing.T) {
	r := testVpcOrigin()

	cfg, err := r.generateEndpointConfig(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "test-vpc-origin", aws.ToString(cfg.Name))
	assert.Equal(t, testLBArn, aws.ToString(cfg.Arn))
	assert.Equal(t, int32(80), aws.ToInt32(cfg.HTTPPort))
	assert.Equal(t, int32(443), aws.ToInt32(cfg.HTTPSPort))
	assert.Equal(t, cftypes.OriginProtocolPolicyHttpsOnly, cfg.OriginProtocolPolicy)
	require.NotNil(t, cfg.OriginSslProtocols)
	assert.Equal(t, int32(1), aws.ToInt32(cfg.OriginSslProtocols.Quantity))
	assert.Equal(t, []cftypes.SslProtocol{cftypes.SslProtocolTLSv12}, cfg.OriginSslProtocols.Items)
}

func TestCompareVpcOriginEndpointConfig_NoChanges(t *testing.T) {
	r := testVpcOrigin()
	desired, err := r.generateEndpointConfig(context.Background())
	require.NoError(t, err)

	msgs, changes := compareVpcOriginEndpointConfig(*desired, *desired)
	assert.Empty(t, msgs)
	assert.Empty(t, changes)
}

func TestCompareVpcOriginEndpointConfig_SslProtocolOrderInsensitive(t *testing.T) {
	existing := cftypes.VpcOriginEndpointConfig{
		OriginSslProtocols: &cftypes.OriginSslProtocols{
			Quantity: aws.Int32(2),
			Items:    []cftypes.SslProtocol{cftypes.SslProtocolTLSv12, cftypes.SslProtocolTLSv11},
		},
	}
	desired := cftypes.VpcOriginEndpointConfig{
		OriginSslProtocols: &cftypes.OriginSslProtocols{
			Quantity: aws.Int32(2),
			Items:    []cftypes.SslProtocol{cftypes.SslProtocolTLSv11, cftypes.SslProtocolTLSv12},
		},
	}
	msgs, changes := compareVpcOriginEndpointConfig(existing, desired)
	assert.Empty(t, msgs)
	assert.Empty(t, changes)
}

func TestCompareVpcOriginEndpointConfig_Differences(t *testing.T) {
	r := testVpcOrigin()
	desired, err := r.generateEndpointConfig(context.Background())
	require.NoError(t, err)

	existing := *desired
	existing.Arn = aws.String("arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/old-alb/abc123")
	existing.HTTPPort = aws.Int32(8080)
	existing.HTTPSPort = aws.Int32(8443)
	existing.OriginProtocolPolicy = cftypes.OriginProtocolPolicyMatchViewer
	existing.OriginSslProtocols = &cftypes.OriginSslProtocols{
		Quantity: aws.Int32(1),
		Items:    []cftypes.SslProtocol{cftypes.SslProtocolTLSv11},
	}

	msgs, changes := compareVpcOriginEndpointConfig(existing, *desired)
	require.Len(t, msgs, 5)
	assert.Contains(t, msgs[0], "target:")
	assert.Contains(t, msgs[1], "httpPort: 8080 -> 80")
	assert.Contains(t, msgs[2], "httpsPort: 8443 -> 443")
	assert.Contains(t, msgs[3], `originProtocolPolicy: "match-viewer" -> "https-only"`)
	assert.Contains(t, msgs[4], "originSslProtocols: [TLSv1.1] -> [TLSv1.2]")

	// One Change per message, in the same order, keyed by the buildit.yml field.
	assert.Equal(t, []Change{
		{Field: "target", From: "arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/old-alb/abc123", To: testLBArn},
		{Field: "httpPort", From: "8080", To: "80"},
		{Field: "httpsPort", From: "8443", To: "443"},
		{Field: "originProtocolPolicy", From: "match-viewer", To: "https-only"},
		{Field: "originSslProtocol", From: "TLSv1.1", To: "TLSv1.2"},
	}, changes)
}

func TestCompareVpcOriginEndpointConfig_SingleChange(t *testing.T) {
	r := testVpcOrigin()
	desired, err := r.generateEndpointConfig(context.Background())
	require.NoError(t, err)

	existing := *desired
	existing.HTTPSPort = aws.Int32(8443)

	msgs, changes := compareVpcOriginEndpointConfig(existing, *desired)
	assert.Equal(t, []string{"httpsPort: 8443 -> 443"}, msgs)
	assert.Equal(t, []Change{{Field: "httpsPort", From: "8443", To: "443"}}, changes)
}

func TestCompareVpcOriginEndpointConfig_SslProtocolListChange(t *testing.T) {
	existing := cftypes.VpcOriginEndpointConfig{
		OriginSslProtocols: &cftypes.OriginSslProtocols{
			Quantity: aws.Int32(2),
			Items:    []cftypes.SslProtocol{cftypes.SslProtocolTLSv12, cftypes.SslProtocolTLSv11},
		},
	}
	desired := cftypes.VpcOriginEndpointConfig{
		OriginSslProtocols: &cftypes.OriginSslProtocols{
			Quantity: aws.Int32(1),
			Items:    []cftypes.SslProtocol{cftypes.SslProtocolTLSv12},
		},
	}

	msgs, changes := compareVpcOriginEndpointConfig(existing, desired)
	assert.Equal(t, []string{"originSslProtocols: [TLSv1.1 TLSv1.2] -> [TLSv1.2]"}, msgs)
	assert.Equal(t, []Change{{Field: "originSslProtocol", From: "TLSv1.1,TLSv1.2", To: "TLSv1.2"}}, changes)
}

func TestCloudfrontVpcOriginDiff_TagChanges(t *testing.T) {
	awsTags := map[string]string{"env": "old-env", "team": "example-team"}
	desiredTags := map[string]string{"env": "new-env", "owner": "example-owner"}

	tagDiff := TagDiffForContext(context.Background(), awsTags, desiredTags)
	require.True(t, tagDiff.HasChanges())

	diffs := &CloudfrontVpcOriginDiff{}
	diffs.Messages = append(diffs.Messages, TagDiffSummary(awsTags, tagDiff)...)
	diffs.AddChanges(TagChanges(awsTags, tagDiff)...)

	var detailed DetailedResourceDiff = diffs
	assert.Equal(t, []Change{
		{Field: "tags.env", From: "old-env", To: "new-env"},
		{Field: "tags.team", From: "example-team", To: ""},
		{Field: "tags.owner", From: "", To: "example-owner"},
	}, detailed.Changes())
	assert.Equal(t, diffs.Messages, detailed.Differences())
}

func TestCloudfrontVpcOriginDiff_NoChangesWhenIdentical(t *testing.T) {
	r := testVpcOrigin()
	desired, err := r.generateEndpointConfig(context.Background())
	require.NoError(t, err)

	diffs := &CloudfrontVpcOriginDiff{}
	msgs, changes := compareVpcOriginEndpointConfig(*desired, *desired)
	diffs.Messages = append(diffs.Messages, msgs...)
	diffs.AddChanges(changes...)

	tags := map[string]string{"env": "example-env"}
	tagDiff := TagDiffForContext(context.Background(), tags, tags)
	require.False(t, tagDiff.HasChanges())

	assert.Empty(t, diffs.Differences())
	assert.Empty(t, diffs.Changes())
}

func TestCloudfrontVpcOrigin_YAMLUnmarshal(t *testing.T) {
	in := `
target: internal-alb
httpPort: 8080
httpsPort: 8443
originProtocolPolicy: match-viewer
originSslProtocol: TLSv1.1
tags:
  Environment: production
`
	var r CloudfrontVpcOrigin
	require.NoError(t, yaml.Unmarshal([]byte(in), &r))

	assert.Equal(t, "internal-alb", r.Target)
	assert.Equal(t, int32(8080), aws.ToInt32(r.HTTPPort))
	assert.Equal(t, int32(8443), aws.ToInt32(r.HTTPSPort))
	assert.Equal(t, CloudfrontOriginProtocolMatchViewer, aws.ToString(r.OriginProtocolPolicy))
	assert.Equal(t, "TLSv1.1", aws.ToString(r.OriginSSLProtocol))
	assert.Equal(t, "production", r.Tags["Environment"])
}
