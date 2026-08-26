package resource

import (
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	cftypes "github.com/aws/aws-sdk-go-v2/service/cloudfront/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

// testDist returns a normalized baseline distribution with a single VPC origin,
// logging, and a default cache behavior. Certificate is left nil and the origin's
// resolved fields are pre-populated so generateConfig performs no AWS lookups.
func testDist() *CloudfrontDistribution {
	r := &CloudfrontDistribution{
		Name: "test-dist",
		Origins: []CloudfrontOrigin{{
			Name:                "primary",
			Type:                CloudfrontOriginTypeVpc,
			Target:              "vo_123",
			resolvedDomainName:  "alb.internal.example.com",
			resolvedVpcOriginId: "vo_123",
		}},
		DefaultCacheBehavior: CloudfrontCacheBehavior{TargetOriginId: "primary"},
		Logging:              &CloudfrontLogging{Bucket: "logs.s3.amazonaws.com", Prefix: aws.String("cf/test")},
	}
	r.Normalize(context.Background())
	return r
}

func TestCloudfrontDistribution_Normalize(t *testing.T) {
	r := testDist()

	assert.Equal(t, true, aws.ToBool(r.Enabled))
	assert.Equal(t, CloudfrontHTTPVersion2and3, aws.ToString(r.HTTPVersion))
	assert.Equal(t, false, aws.ToBool(r.IsIPV6Enabled))
	assert.Equal(t, CloudfrontPriceClass100, aws.ToString(r.PriceClass))
	assert.Equal(t, CloudfrontMinProtocolTLSv122021, aws.ToString(r.MinProtocolVersion))
	assert.Equal(t, CloudfrontSSLSupportSNIOnly, aws.ToString(r.SSLSupportMethod))

	// default cache behavior defaults
	b := r.DefaultCacheBehavior
	assert.Equal(t, CloudfrontViewerProtocolHTTPSOnly, aws.ToString(b.ViewerProtocolPolicy))
	assert.ElementsMatch(t, cloudfrontDefaultAllowedMethods, b.AllowedMethods)
	assert.Equal(t, CloudfrontCachePolicyCachingDisabled, aws.ToString(b.CachePolicyId))
	assert.Equal(t, CloudfrontOriginRequestPolicyAllViewerAndCF, aws.ToString(b.OriginRequestPolicyId))
	assert.Equal(t, false, aws.ToBool(b.Compress))

	// origin defaults
	o := r.Origins[0]
	assert.Equal(t, int32(3), aws.ToInt32(o.ConnectionAttempts))
	assert.Equal(t, int32(10), aws.ToInt32(o.ConnectionTimeout))
	assert.Equal(t, int32(5), aws.ToInt32(o.OriginKeepAliveTimeout))
	assert.Equal(t, int32(30), aws.ToInt32(o.OriginReadTimeout))
	// responseCompletionTimeout defaults to nil (disabled)
	assert.Nil(t, o.ResponseCompletionTimeout)

	// logging defaults
	assert.Equal(t, true, aws.ToBool(r.Logging.Enabled))
	assert.Equal(t, false, aws.ToBool(r.Logging.IncludeCookies))
}

func TestCloudfrontDistribution_Normalize_MergesTags(t *testing.T) {
	r := &CloudfrontDistribution{
		Name:       "test-dist",
		Tags:       map[string]string{"Environment": "production"},
		GlobalTags: map[string]string{"Owner": "team-a", "Environment": "should-not-override"},
	}
	r.Normalize(context.Background())
	assert.Equal(t, "production", r.Tags["Environment"])
	assert.Equal(t, "team-a", r.Tags["Owner"])
}

func TestCloudfrontDistribution_Normalize_CustomOriginDefaults(t *testing.T) {
	r := &CloudfrontDistribution{
		Name: "test-dist",
		Origins: []CloudfrontOrigin{{
			Name:   "web",
			Type:   CloudfrontOriginTypeCustom,
			Target: "example.com",
		}},
		DefaultCacheBehavior: CloudfrontCacheBehavior{TargetOriginId: "web"},
	}
	r.Normalize(context.Background())

	o := r.Origins[0]
	assert.Equal(t, int32(80), aws.ToInt32(o.HTTPPort))
	assert.Equal(t, int32(443), aws.ToInt32(o.HTTPSPort))
	assert.Equal(t, CloudfrontOriginProtocolHTTPSOnly, aws.ToString(o.OriginProtocolPolicy))
	assert.Equal(t, []string{"TLSv1.2"}, o.OriginSSLProtocols)
	assert.Equal(t, int32(5), aws.ToInt32(o.OriginKeepAliveTimeout))
	assert.Equal(t, int32(30), aws.ToInt32(o.OriginReadTimeout))
}

func TestCloudfrontDistribution_Normalize_S3TargetOriginRequestPolicy(t *testing.T) {
	r := &CloudfrontDistribution{
		Name: "test-dist",
		Origins: []CloudfrontOrigin{{
			Name:   "assets",
			Type:   CloudfrontOriginTypeS3,
			Target: "my-bucket",
		}},
		DefaultCacheBehavior: CloudfrontCacheBehavior{TargetOriginId: "assets"},
	}
	r.Normalize(context.Background())

	// Host-header-forwarding policies are rejected by AWS for S3 targets, so the
	// default must switch to the managed CORS-S3Origin policy.
	assert.Equal(t, CloudfrontOriginRequestPolicyCORSS3, aws.ToString(r.DefaultCacheBehavior.OriginRequestPolicyId))
}

func TestCloudfrontDistribution_Normalize_S3OriginNoTimeoutDefaults(t *testing.T) {
	r := &CloudfrontDistribution{
		Name: "test-dist",
		Origins: []CloudfrontOrigin{{
			Name:   "assets",
			Type:   CloudfrontOriginTypeS3,
			Target: "my-bucket",
		}},
		DefaultCacheBehavior: CloudfrontCacheBehavior{TargetOriginId: "assets"},
	}
	r.Normalize(context.Background())

	// S3 origins carry no read/keep-alive timeouts, so they must stay nil.
	o := r.Origins[0]
	assert.Nil(t, o.OriginReadTimeout)
	assert.Nil(t, o.OriginKeepAliveTimeout)
	assert.Nil(t, o.HTTPPort)
}

func TestCloudfrontDistribution_Normalize_CaseInsensitiveEnums(t *testing.T) {
	r := &CloudfrontDistribution{
		Name:               "test-dist",
		HTTPVersion:        aws.String("HTTP2AND3"),
		PriceClass:         aws.String("priceclass_100"),
		SSLSupportMethod:   aws.String("SNI-Only"),
		MinProtocolVersion: aws.String("tlsv1.2_2021"),
		Origins: []CloudfrontOrigin{
			{
				Name:                "primary",
				Type:                "VPC",
				Target:              "vo_123",
				resolvedDomainName:  "alb.internal.example.com",
				resolvedVpcOriginId: "vo_123",
			},
			{
				Name:                 "web",
				Type:                 "Custom",
				Target:               "example.com",
				OriginProtocolPolicy: aws.String("HTTPS-Only"),
				OriginSSLProtocols:   []string{"tlsv1.2"},
			},
		},
		DefaultCacheBehavior: CloudfrontCacheBehavior{
			TargetOriginId:       "primary",
			ViewerProtocolPolicy: aws.String("Redirect-To-HTTPS"),
			AllowedMethods:       []string{"get", "Head", "options"},
			CachedMethods:        []string{"get", "head"},
			FunctionAssociations: []CloudfrontFunctionAssociation{
				{EventType: "Viewer-Request", FunctionARN: "arn:aws:cloudfront::123:function/f"},
			},
		},
	}
	r.Normalize(context.Background())

	assert.Equal(t, CloudfrontHTTPVersion2and3, aws.ToString(r.HTTPVersion))
	assert.Equal(t, CloudfrontPriceClass100, aws.ToString(r.PriceClass))
	assert.Equal(t, CloudfrontSSLSupportSNIOnly, aws.ToString(r.SSLSupportMethod))
	assert.Equal(t, CloudfrontMinProtocolTLSv122021, aws.ToString(r.MinProtocolVersion))

	assert.Equal(t, CloudfrontOriginTypeVpc, r.Origins[0].Type)
	assert.Equal(t, CloudfrontOriginTypeCustom, r.Origins[1].Type)
	assert.Equal(t, CloudfrontOriginProtocolHTTPSOnly, aws.ToString(r.Origins[1].OriginProtocolPolicy))
	assert.Equal(t, []string{"TLSv1.2"}, r.Origins[1].OriginSSLProtocols)

	b := r.DefaultCacheBehavior
	assert.Equal(t, CloudfrontViewerProtocolRedirectToHTTPS, aws.ToString(b.ViewerProtocolPolicy))
	assert.Equal(t, []string{"GET", "HEAD", "OPTIONS"}, b.AllowedMethods)
	assert.Equal(t, []string{"GET", "HEAD"}, b.CachedMethods)
	assert.Equal(t, CloudfrontEventViewerRequest, b.FunctionAssociations[0].EventType)

	// normalized values must validate cleanly
	assert.Nil(t, r.Validate(context.Background()))
}

func TestCloudfrontDistribution_Normalize_ManagedPolicyNames(t *testing.T) {
	tests := []struct {
		name                  string
		cachePolicy           string
		originRequestPolicy   string
		responseHeadersPolicy string
		wantCache             string
		wantOriginRequest     string
		wantResponseHeaders   string
	}{
		{
			name:                  "documented names",
			cachePolicy:           "CachingOptimized",
			originRequestPolicy:   "AllViewer",
			responseHeadersPolicy: "SecurityHeadersPolicy",
			wantCache:             "658327ea-f89d-4fab-a63d-7e88639e58f6",
			wantOriginRequest:     cloudfrontOriginRequestPolicyAllViewer,
			wantResponseHeaders:   "67f7725c-6f97-4210-82d7-5512b31e9d03",
		},
		{
			name:                  "case-insensitive names",
			cachePolicy:           "cachingoptimized",
			originRequestPolicy:   "ALLVIEWER",
			responseHeadersPolicy: "securityheaderspolicy",
			wantCache:             "658327ea-f89d-4fab-a63d-7e88639e58f6",
			wantOriginRequest:     cloudfrontOriginRequestPolicyAllViewer,
			wantResponseHeaders:   "67f7725c-6f97-4210-82d7-5512b31e9d03",
		},
		{
			name:                  "Managed- prefix accepted",
			cachePolicy:           "Managed-CachingDisabled",
			originRequestPolicy:   "managed-CORS-S3Origin",
			responseHeadersPolicy: "Managed-SimpleCORS",
			wantCache:             CloudfrontCachePolicyCachingDisabled,
			wantOriginRequest:     CloudfrontOriginRequestPolicyCORSS3,
			wantResponseHeaders:   "60669652-455b-4ae9-85a4-c4c02393f86c",
		},
		{
			name:                  "ids pass through untouched",
			cachePolicy:           CloudfrontCachePolicyCachingDisabled,
			originRequestPolicy:   CloudfrontOriginRequestPolicyAllViewerAndCF,
			responseHeadersPolicy: "67f7725c-6f97-4210-82d7-5512b31e9d03",
			wantCache:             CloudfrontCachePolicyCachingDisabled,
			wantOriginRequest:     CloudfrontOriginRequestPolicyAllViewerAndCF,
			wantResponseHeaders:   "67f7725c-6f97-4210-82d7-5512b31e9d03",
		},
		{
			name:                  "custom policy names left for API resolution",
			cachePolicy:           "my-custom-cache-policy",
			originRequestPolicy:   "my-custom-origin-policy",
			responseHeadersPolicy: "my-custom-headers-policy",
			wantCache:             "my-custom-cache-policy",
			wantOriginRequest:     "my-custom-origin-policy",
			wantResponseHeaders:   "my-custom-headers-policy",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := testDist()
			r.DefaultCacheBehavior.CachePolicyId = aws.String(tt.cachePolicy)
			r.DefaultCacheBehavior.OriginRequestPolicyId = aws.String(tt.originRequestPolicy)
			r.DefaultCacheBehavior.ResponseHeadersPolicyId = aws.String(tt.responseHeadersPolicy)
			r.Normalize(context.Background())

			b := r.DefaultCacheBehavior
			assert.Equal(t, tt.wantCache, aws.ToString(b.CachePolicyId))
			assert.Equal(t, tt.wantOriginRequest, aws.ToString(b.OriginRequestPolicyId))
			assert.Equal(t, tt.wantResponseHeaders, aws.ToString(b.ResponseHeadersPolicyId))
		})
	}
}

func TestCloudfrontDistribution_Validate_Methods(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name    string
		allowed []string
		cached  []string
		wantErr string // empty means valid
	}{
		{name: "defaulted methods", wantErr: ""},
		{name: "read-only combo", allowed: []string{"GET", "HEAD"}, cached: []string{"GET", "HEAD"}},
		{name: "options combo any order", allowed: []string{"OPTIONS", "GET", "HEAD"}, cached: []string{"OPTIONS", "HEAD", "GET"}},
		{name: "full combo", allowed: []string{"GET", "HEAD", "OPTIONS", "PUT", "POST", "PATCH", "DELETE"}},
		{name: "lowercase input", allowed: []string{"get", "head"}, cached: []string{"head", "get"}},
		{name: "partial combo", allowed: []string{"GET", "POST"}, wantErr: "invalid allowedMethods"},
		{name: "unknown method", allowed: []string{"GET", "HEAD", "TRACE"}, wantErr: "invalid allowedMethods"},
		{name: "duplicate allowed method", allowed: []string{"GET", "GET", "HEAD"}, wantErr: "invalid allowedMethods"},
		{name: "invalid cached combo", cached: []string{"GET"}, wantErr: "invalid cachedMethods"},
		{name: "cached not subset of allowed", allowed: []string{"GET", "HEAD"}, cached: []string{"GET", "HEAD", "OPTIONS"}, wantErr: "must also be in allowedMethods"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &CloudfrontDistribution{
				Name: "x",
				Origins: []CloudfrontOrigin{{
					Name:                "primary",
					Type:                CloudfrontOriginTypeVpc,
					Target:              "vo_123",
					resolvedDomainName:  "alb.internal.example.com",
					resolvedVpcOriginId: "vo_123",
				}},
				DefaultCacheBehavior: CloudfrontCacheBehavior{
					TargetOriginId: "primary",
					AllowedMethods: tt.allowed,
					CachedMethods:  tt.cached,
				},
			}
			r.Normalize(ctx)
			err := r.Validate(ctx)
			if tt.wantErr == "" {
				assert.Nil(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

// TestCloudfrontDistribution_Validate_S3IncompatiblePolicyByName ensures the
// Host-header-forwarding check also catches managed policies referenced by name
// (normalized to ids before validation).
func TestCloudfrontDistribution_Validate_S3IncompatiblePolicyByName(t *testing.T) {
	ctx := context.Background()
	for _, policy := range []string{"AllViewer", "allviewerandcloudfrontheaders-2022-06", "Managed-HostHeaderOnly"} {
		t.Run(policy, func(t *testing.T) {
			r := &CloudfrontDistribution{
				Name:    "x",
				Origins: []CloudfrontOrigin{{Name: "assets", Type: CloudfrontOriginTypeS3, Target: "my-bucket"}},
				DefaultCacheBehavior: CloudfrontCacheBehavior{
					TargetOriginId:        "assets",
					OriginRequestPolicyId: aws.String(policy),
				},
			}
			r.Normalize(ctx)
			err := r.Validate(ctx)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "cannot be used with s3 target origin")
		})
	}
}

// TestCloudfrontDistribution_GenerateConfig_ManagedNamesNoAWSCalls ensures a config using
// managed policy names generates ids without any AWS lookup (would panic without a client).
func TestCloudfrontDistribution_GenerateConfig_ManagedNamesNoAWSCalls(t *testing.T) {
	ctx := context.Background()
	r := testDist()
	r.DefaultCacheBehavior.CachePolicyId = aws.String("CachingOptimized")
	r.DefaultCacheBehavior.OriginRequestPolicyId = aws.String("AllViewer")
	r.DefaultCacheBehavior.ResponseHeadersPolicyId = aws.String("SecurityHeadersPolicy")
	r.Normalize(ctx)

	cfg, err := r.generateConfig(ctx)
	require.NoError(t, err)
	assert.Equal(t, "658327ea-f89d-4fab-a63d-7e88639e58f6", aws.ToString(cfg.DefaultCacheBehavior.CachePolicyId))
	assert.Equal(t, cloudfrontOriginRequestPolicyAllViewer, aws.ToString(cfg.DefaultCacheBehavior.OriginRequestPolicyId))
	assert.Equal(t, "67f7725c-6f97-4210-82d7-5512b31e9d03", aws.ToString(cfg.DefaultCacheBehavior.ResponseHeadersPolicyId))
}

func TestCloudfrontDistribution_Validate(t *testing.T) {
	ctx := context.Background()

	t.Run("valid configuration", func(t *testing.T) {
		r := testDist()
		assert.Nil(t, r.Validate(ctx))
	})

	t.Run("missing origins", func(t *testing.T) {
		r := &CloudfrontDistribution{Name: "x", DefaultCacheBehavior: CloudfrontCacheBehavior{TargetOriginId: "primary"}}
		r.Normalize(ctx)
		err := r.Validate(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "at least one origin")
	})

	t.Run("origin with no type", func(t *testing.T) {
		r := &CloudfrontDistribution{
			Name:                 "x",
			Origins:              []CloudfrontOrigin{{Name: "o1", Target: "t"}},
			DefaultCacheBehavior: CloudfrontCacheBehavior{TargetOriginId: "o1"},
		}
		r.Normalize(ctx)
		err := r.Validate(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "type is required")
	})

	t.Run("origin with invalid type", func(t *testing.T) {
		r := &CloudfrontDistribution{
			Name:                 "x",
			Origins:              []CloudfrontOrigin{{Name: "o1", Type: "alb", Target: "t"}},
			DefaultCacheBehavior: CloudfrontCacheBehavior{TargetOriginId: "o1"},
		}
		r.Normalize(ctx)
		err := r.Validate(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid type")
	})

	t.Run("origin with no target", func(t *testing.T) {
		r := &CloudfrontDistribution{
			Name:                 "x",
			Origins:              []CloudfrontOrigin{{Name: "o1", Type: CloudfrontOriginTypeVpc}},
			DefaultCacheBehavior: CloudfrontCacheBehavior{TargetOriginId: "o1"},
		}
		r.Normalize(ctx)
		err := r.Validate(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "target is required")
	})

	t.Run("custom-only fields on vpc origin", func(t *testing.T) {
		r := &CloudfrontDistribution{
			Name: "x",
			Origins: []CloudfrontOrigin{{
				Name:     "o1",
				Type:     CloudfrontOriginTypeVpc,
				Target:   "vo_1",
				HTTPPort: aws.Int32(8080),
			}},
			DefaultCacheBehavior: CloudfrontCacheBehavior{TargetOriginId: "o1"},
		}
		r.Normalize(ctx)
		err := r.Validate(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "only apply to custom origins")
	})

	t.Run("read timeout on s3 origin", func(t *testing.T) {
		r := &CloudfrontDistribution{
			Name: "x",
			Origins: []CloudfrontOrigin{{
				Name:              "o1",
				Type:              CloudfrontOriginTypeS3,
				Target:            "my-bucket",
				OriginReadTimeout: aws.Int32(10),
			}},
			DefaultCacheBehavior: CloudfrontCacheBehavior{TargetOriginId: "o1"},
		}
		r.Normalize(ctx)
		err := r.Validate(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "do not apply to s3 origins")
	})

	t.Run("responseCompletionTimeout below originReadTimeout", func(t *testing.T) {
		r := testDist()
		r.Origins[0].ResponseCompletionTimeout = aws.Int32(10) // originReadTimeout defaults to 30
		err := r.Validate(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be >= originReadTimeout")
	})

	t.Run("duplicate origin names", func(t *testing.T) {
		r := &CloudfrontDistribution{
			Name: "x",
			Origins: []CloudfrontOrigin{
				{Name: "o1", Type: CloudfrontOriginTypeVpc, Target: "vo_1"},
				{Name: "o1", Type: CloudfrontOriginTypeVpc, Target: "vo_2"},
			},
			DefaultCacheBehavior: CloudfrontCacheBehavior{TargetOriginId: "o1"},
		}
		r.Normalize(ctx)
		err := r.Validate(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate origin name")
	})

	t.Run("s3-incompatible origin request policy", func(t *testing.T) {
		r := &CloudfrontDistribution{
			Name:    "x",
			Origins: []CloudfrontOrigin{{Name: "assets", Type: CloudfrontOriginTypeS3, Target: "my-bucket"}},
			DefaultCacheBehavior: CloudfrontCacheBehavior{
				TargetOriginId:        "assets",
				OriginRequestPolicyId: aws.String(CloudfrontOriginRequestPolicyAllViewerAndCF),
			},
		}
		r.Normalize(ctx)
		err := r.Validate(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be used with s3 target origin")
	})

	t.Run("dangling target origin id", func(t *testing.T) {
		r := &CloudfrontDistribution{
			Name:                 "x",
			Origins:              []CloudfrontOrigin{{Name: "o1", Type: CloudfrontOriginTypeVpc, Target: "vo_1"}},
			DefaultCacheBehavior: CloudfrontCacheBehavior{TargetOriginId: "missing"},
		}
		r.Normalize(ctx)
		err := r.Validate(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not match any origin id")
	})

	t.Run("invalid enum", func(t *testing.T) {
		r := testDist()
		r.PriceClass = aws.String("PriceClass_Nope")
		err := r.Validate(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid priceClass")
	})

	t.Run("aliases without certificate", func(t *testing.T) {
		r := testDist()
		r.Aliases = []string{"cdn.example.com"}
		err := r.Validate(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "aliases require a certificate")
	})

	t.Run("logging without bucket", func(t *testing.T) {
		r := testDist()
		r.Logging = &CloudfrontLogging{Prefix: aws.String("cf/x")}
		err := r.Validate(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "logging.bucket is required")
	})

	t.Run("name too long", func(t *testing.T) {
		r := testDist()
		r.Name = strings.Repeat("a", 129)
		err := r.Validate(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be longer than 128 characters")
	})
}

// TestCloudfrontDistribution_Compare_RoundTripNoDiff guards against perpetual no-op
// updates: a config read back from AWS (with AWS-populated default-cert fields and the
// default CachedMethods) must compare equal to the config buildit generates from the same
// resource. Covers review must-fix #1 (CachedMethods) and #2 (default-cert drift).
func TestCloudfrontDistribution_Compare_RoundTripNoDiff(t *testing.T) {
	ctx := context.Background()
	r := testDist() // no Certificate => default CloudFront cert; VPC origin; logging

	desired, err := r.generateConfig(ctx)
	require.NoError(t, err)

	// CachedMethods must be emitted (fix #1) so it matches what AWS returns.
	require.NotNil(t, desired.DefaultCacheBehavior.AllowedMethods.CachedMethods)
	// Aliases / WebACLId are always populated for explicit-clear semantics (fix #5).
	require.NotNil(t, desired.Aliases)
	require.NotNil(t, desired.WebACLId)

	// Simulate what AWS returns on read-back for a default-cert distribution.
	awsLike, err := r.generateConfig(ctx)
	require.NoError(t, err)
	awsLike.ViewerCertificate.MinimumProtocolVersion = cftypes.MinimumProtocolVersionTLSv1
	awsLike.ViewerCertificate.SSLSupportMethod = cftypes.SSLSupportMethodVip
	// AWS represents "no items" as non-nil containers with Quantity 0 where the
	// generated config leaves the field nil; this must not produce a phantom diff.
	awsLike.Origins.Items[0].CustomHeaders = &cftypes.CustomHeaders{Quantity: aws.Int32(0)}
	awsLike.CustomErrorResponses = &cftypes.CustomErrorResponses{Quantity: aws.Int32(0)}

	assert.Empty(t, compareDistributionConfig(*awsLike, *desired, nil),
		"a config read back from AWS must not differ from the regenerated desired config")
}

// TestCloudfrontDistribution_Compare_IgnoresUnmanagedFields verifies that ordered cache
// behaviors and origin groups on the existing distribution (which buildit does not
// manage) do not produce a diff.
func TestCloudfrontDistribution_Compare_IgnoresUnmanagedFields(t *testing.T) {
	ctx := context.Background()

	existingCfg, err := testDist().generateConfig(ctx)
	require.NoError(t, err)
	existingCfg.CacheBehaviors = &cftypes.CacheBehaviors{
		Quantity: aws.Int32(1),
		Items: []cftypes.CacheBehavior{{
			PathPattern:    aws.String("/api/*"),
			TargetOriginId: aws.String("primary"),
		}},
	}

	desiredCfg, err := testDist().generateConfig(ctx)
	require.NoError(t, err)

	assert.Empty(t, compareDistributionConfig(*existingCfg, *desiredCfg, nil),
		"unmanaged ordered cache behaviors must not produce a diff")
}

// TestCloudfrontDistribution_Compare_ClearsWebACLAndAliases verifies that removing
// webAclName / aliases from the config is detected as a change (and thus sent to AWS as
// an explicit clear). Covers review should-fix #5.
func TestCloudfrontDistribution_Compare_ClearsWebACLAndAliases(t *testing.T) {
	ctx := context.Background()

	existing := testDist()
	// An "arn:" value skips the name lookup (the memoized form), so no AWS call is made.
	existing.WebACLName = aws.String("arn:aws:wafv2:us-east-1:1:global/webacl/x/y")
	existing.Aliases = []string{"cdn.example.com"}
	existingCfg, err := existing.generateConfig(ctx)
	require.NoError(t, err)

	desiredCfg, err := testDist().generateConfig(ctx) // no webACL, no aliases
	require.NoError(t, err)

	// unset webAclName must emit an explicit empty WebACLId so AWS disassociates it
	require.NotNil(t, desiredCfg.WebACLId)
	assert.Equal(t, "", aws.ToString(desiredCfg.WebACLId))

	msgs := compareDistributionConfig(*existingCfg, *desiredCfg, nil)
	joined := strings.Join(msgs, "\n")
	assert.Contains(t, joined, "webAclName")
	assert.Contains(t, joined, "aliases")
}

// TestCloudfrontDistribution_Compare_DiffShowsUserSuppliedValues verifies diff messages
// render resolved identifiers as the values the user supplied (names) instead of the
// raw ARNs/ids, on both sides of the change — including a web ACL on the existing
// distribution that buildit never resolved (its name is parsed from the wafv2 ARN).
func TestCloudfrontDistribution_Compare_DiffShowsUserSuppliedValues(t *testing.T) {
	ctx := context.Background()

	existing := testDist() // default cache policy: managed CachingDisabled
	// out-of-band web ACL: raw ARN read back from AWS, never in the display map
	existing.WebACLName = aws.String("arn:aws:wafv2:us-east-1:123456789012:global/webacl/old-web-acl/1f562b14-7729-46d7-aff8-5a85f9b5eaa3")
	existingCfg, err := existing.generateConfig(ctx)
	require.NoError(t, err)

	desired := testDist()
	desired.DefaultCacheBehavior.CachePolicyId = aws.String("CachingOptimized")
	desired.WebACLName = aws.String("arn:aws:wafv2:us-east-1:1:global/webacl/new-web-acl/abc") // memoized form; skips the lookup
	desired.Normalize(ctx)                                                                     // canonicalize the managed policy name to its id
	// simulate what web ACL resolution records (the lookup itself needs AWS)
	desired.displayValues["arn:aws:wafv2:us-east-1:1:global/webacl/new-web-acl/abc"] = "new-web-acl"
	desiredCfg, err := desired.generateConfig(ctx)
	require.NoError(t, err)

	msgs := compareDistributionConfig(*existingCfg, *desiredCfg, desired.displayValues)
	joined := strings.Join(msgs, "\n")

	// policy ids render as their documented managed names on both sides
	assert.Contains(t, joined, "cachePolicyId: CachingDisabled -> CachingOptimized")
	assert.NotContains(t, joined, "658327ea") // CachingOptimized's id
	// both web ACLs render by name: the existing one parsed from its ARN, the desired
	// one from the display map
	assert.Contains(t, joined, `webAclName: "old-web-acl" -> "new-web-acl"`)
	assert.NotContains(t, joined, "wafv2")
}

// TestCloudfrontDistribution_Compare_WebACLSameNameDifferentACL verifies that a web ACL
// replaced by another with the same name (delete + recreate) still diffs — comparison is
// by ARN — and the message falls back to the raw ARNs since names alone can't tell the
// two apart.
func TestCloudfrontDistribution_Compare_WebACLSameNameDifferentACL(t *testing.T) {
	ctx := context.Background()

	existing := testDist()
	existing.WebACLName = aws.String("arn:aws:wafv2:us-east-1:1:global/webacl/test-web-acl/old-id")
	existingCfg, err := existing.generateConfig(ctx)
	require.NoError(t, err)

	desired := testDist()
	desired.WebACLName = aws.String("arn:aws:wafv2:us-east-1:1:global/webacl/test-web-acl/new-id")
	desiredCfg, err := desired.generateConfig(ctx)
	require.NoError(t, err)

	msgs := compareDistributionConfig(*existingCfg, *desiredCfg, nil)
	joined := strings.Join(msgs, "\n")
	assert.Contains(t, joined, "webAclName")
	assert.Contains(t, joined, "old-id")
	assert.Contains(t, joined, "new-id")
}

// TestCloudfrontDistribution_GenerateConfig_DoesNotMutateUserInput guards the
// Normalize/Validate/generate separation: generateConfig must never write resolved AWS
// values back into the resource's user-supplied (YAML-facing) fields.
func TestCloudfrontDistribution_GenerateConfig_DoesNotMutateUserInput(t *testing.T) {
	ctx := context.Background()

	r := testDist()
	webACL := aws.String("arn:aws:wafv2:us-east-1:1:global/webacl/my-acl/abc")
	cachePolicy := aws.String("00000000-0000-0000-0000-000000000001")
	r.WebACLName = webACL
	r.DefaultCacheBehavior.CachePolicyId = cachePolicy

	first, err := r.generateConfig(ctx)
	require.NoError(t, err)
	second, err := r.generateConfig(ctx)
	require.NoError(t, err)

	// the fields still hold the original pointers and values
	assert.Same(t, webACL, r.WebACLName)
	assert.Equal(t, "arn:aws:wafv2:us-east-1:1:global/webacl/my-acl/abc", *r.WebACLName)
	assert.Same(t, cachePolicy, r.DefaultCacheBehavior.CachePolicyId)
	assert.Equal(t, "00000000-0000-0000-0000-000000000001", *r.DefaultCacheBehavior.CachePolicyId)

	// repeated generation is deterministic
	assert.Empty(t, compareDistributionConfig(*first, *second, nil))
}

// TestCloudfrontDistribution_ResolveCacheBehaviorPolicies_ReturnsCopy verifies the
// resolver returns an equivalent behavior without touching the caller's struct when all
// policy identifiers are already ids (the only case that needs no AWS client).
func TestCloudfrontDistribution_ResolveCacheBehaviorPolicies_ReturnsCopy(t *testing.T) {
	ctx := context.Background()
	r := testDist()

	in := r.DefaultCacheBehavior
	cachePtr, originPtr := in.CachePolicyId, in.OriginRequestPolicyId
	out, err := r.resolveCacheBehaviorPolicies(ctx, in)
	require.NoError(t, err)

	assert.Equal(t, in, out)
	assert.Same(t, cachePtr, in.CachePolicyId)
	assert.Same(t, originPtr, in.OriginRequestPolicyId)
}

// TestCloudfrontDistribution_Compare_ClearsLogging verifies that removing the logging
// block is detected and sent to AWS as an explicit disable (not a perpetual no-op diff).
func TestCloudfrontDistribution_Compare_ClearsLogging(t *testing.T) {
	ctx := context.Background()

	existingCfg, err := testDist().generateConfig(ctx) // testDist has logging configured
	require.NoError(t, err)

	desired := testDist()
	desired.Logging = nil
	desiredCfg, err := desired.generateConfig(ctx)
	require.NoError(t, err)

	// desired must emit an explicit disabled LoggingConfig (not nil) so AWS clears it.
	require.NotNil(t, desiredCfg.Logging)
	assert.False(t, aws.ToBool(desiredCfg.Logging.Enabled))

	msgs := compareDistributionConfig(*existingCfg, *desiredCfg, nil)
	assert.Contains(t, strings.Join(msgs, "\n"), "logging")

	// And a second apply with logging still absent must report no logging diff.
	desiredCfg2, err := desired.generateConfig(ctx)
	require.NoError(t, err)
	assert.Empty(t, compareDistributionConfig(*desiredCfg, *desiredCfg2, nil))
}

func TestCloudfrontDistribution_Compare_NoChange(t *testing.T) {
	ctx := context.Background()
	r := testDist()

	a, err := r.generateConfig(ctx)
	require.NoError(t, err)
	b, err := r.generateConfig(ctx)
	require.NoError(t, err)

	assert.Empty(t, compareDistributionConfig(*a, *b, nil))
}

func TestCloudfrontDistribution_Compare_DetectsChanges(t *testing.T) {
	ctx := context.Background()

	cases := []struct {
		name   string
		mutate func(r *CloudfrontDistribution)
		want   string
	}{
		{"comment", func(r *CloudfrontDistribution) { r.Comment = aws.String("changed") }, "comment"},
		{"enabled", func(r *CloudfrontDistribution) { r.Enabled = aws.Bool(false) }, "enabled"},
		{"defaultRootObject", func(r *CloudfrontDistribution) { r.DefaultRootObject = aws.String("index.html") }, "defaultRootObject"},
		{"httpVersion", func(r *CloudfrontDistribution) { r.HTTPVersion = aws.String(CloudfrontHTTPVersion2) }, "httpVersion"},
		{"isIPV6Enabled", func(r *CloudfrontDistribution) { r.IsIPV6Enabled = aws.Bool(true) }, "isIPV6Enabled"},
		{"priceClass", func(r *CloudfrontDistribution) { r.PriceClass = aws.String(CloudfrontPriceClassAll) }, "priceClass"},
		{"webAclName", func(r *CloudfrontDistribution) {
			r.WebACLName = aws.String("arn:aws:wafv2:us-east-1:1:global/webacl/x/y")
		}, "webAclName"},
		{"aliases", func(r *CloudfrontDistribution) { r.Aliases = []string{"cdn.example.com"} }, "aliases"},
		{"viewerProtocolPolicy", func(r *CloudfrontDistribution) {
			r.DefaultCacheBehavior.ViewerProtocolPolicy = aws.String(CloudfrontViewerProtocolRedirectToHTTPS)
		}, "defaultCacheBehavior"},
		{"cachePolicyId", func(r *CloudfrontDistribution) {
			r.DefaultCacheBehavior.CachePolicyId = aws.String("00000000-0000-0000-0000-000000000000")
		}, "defaultCacheBehavior"},
		{"functionAssociations", func(r *CloudfrontDistribution) {
			r.DefaultCacheBehavior.FunctionAssociations = []CloudfrontFunctionAssociation{
				{EventType: CloudfrontEventViewerRequest, FunctionARN: "arn:aws:cloudfront::1:function/x"},
			}
		}, "defaultCacheBehavior"},
		{"origin domainName", func(r *CloudfrontDistribution) { r.Origins[0].resolvedDomainName = "other.example.com" },
			`origin "primary": domainName: alb.internal.example.com -> other.example.com`},
		{"origin connectionAttempts", func(r *CloudfrontDistribution) { r.Origins[0].ConnectionAttempts = aws.Int32(1) },
			`origin "primary": connectionAttempts: 3 -> 1`},
		{"origin responseCompletionTimeout", func(r *CloudfrontDistribution) {
			r.Origins[0].ResponseCompletionTimeout = aws.Int32(60)
		}, `origin "primary": responseCompletionTimeout: 0 -> 60`},
		{"vpc origin id", func(r *CloudfrontDistribution) { r.Origins[0].resolvedVpcOriginId = "vo_999" },
			`origin "primary": vpc: vpcOriginId: vo_123 -> vo_999`},
		{"origin removed", func(r *CloudfrontDistribution) { r.Origins = nil }, `origin "primary": removed`},
		{"origin added", func(r *CloudfrontDistribution) {
			r.Origins = append(r.Origins, CloudfrontOrigin{
				Name: "extra", Type: CloudfrontOriginTypeCustom, Target: "api.example.com",
				resolvedDomainName: "api.example.com",
			})
			r.Normalize(context.Background())
		}, `origin "extra": added (type custom, domain api.example.com)`},
		{"origin type change", func(r *CloudfrontDistribution) {
			r.Origins[0].Type = CloudfrontOriginTypeCustom
			r.Origins[0].HTTPPort = aws.Int32(80)
			r.Origins[0].HTTPSPort = aws.Int32(443)
			r.Origins[0].OriginProtocolPolicy = aws.String(CloudfrontOriginProtocolHTTPSOnly)
			r.Origins[0].OriginSSLProtocols = []string{"TLSv1.2"}
		}, `origin "primary": type: vpc -> custom`},
		{"logging prefix", func(r *CloudfrontDistribution) { r.Logging.Prefix = aws.String("cf/other") }, "logging: prefix: cf/test -> cf/other"},
		{"customErrorResponses", func(r *CloudfrontDistribution) {
			r.CustomErrorResponses = []CloudfrontCustomErrorResponse{{ErrorCode: 404, ResponseCode: aws.String("200"), ResponsePagePath: aws.String("/index.html")}}
		}, "customErrorResponses"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base := testDist()
			variant := testDist()
			tc.mutate(variant)

			a, err := base.generateConfig(ctx)
			require.NoError(t, err)
			b, err := variant.generateConfig(ctx)
			require.NoError(t, err)

			msgs := compareDistributionConfig(*a, *b, nil)
			require.NotEmpty(t, msgs, "expected a detected change")
			assert.Contains(t, strings.Join(msgs, "\n"), tc.want)
		})
	}
}

// TestCloudfrontDistribution_ResolveOrigins_NoAWSCalls covers the resolution paths that
// require no AWS lookups: a literal domain for a custom origin and a full S3 endpoint.
func TestCloudfrontDistribution_ResolveOrigins_NoAWSCalls(t *testing.T) {
	ctx := context.Background()

	r := &CloudfrontDistribution{
		Name: "test-dist",
		Origins: []CloudfrontOrigin{
			{Name: "web", Type: CloudfrontOriginTypeCustom, Target: "api.example.com"},
			{Name: "assets", Type: CloudfrontOriginTypeS3, Target: "my-assets.s3.us-east-1.amazonaws.com"},
		},
		DefaultCacheBehavior: CloudfrontCacheBehavior{TargetOriginId: "web"},
	}
	r.Normalize(ctx)

	require.NoError(t, r.resolveOrigins(ctx))
	assert.Equal(t, "api.example.com", r.Origins[0].resolvedDomainName)
	assert.Equal(t, "my-assets.s3.us-east-1.amazonaws.com", r.Origins[1].resolvedDomainName)
}

// TestCloudfrontDistribution_MergeManagedConfig verifies that building the update
// config preserves unmanaged fields from the existing distribution (UpdateDistribution
// requires the complete config and buildit must not wipe what it does not manage) while
// overwriting the managed ones.
func TestCloudfrontDistribution_MergeManagedConfig(t *testing.T) {
	ctx := context.Background()

	r := testDist()
	r.Comment = aws.String("updated comment")
	generated, err := r.generateConfig(ctx)
	require.NoError(t, err)

	// Simulate an existing config carrying unmanaged state.
	existing, err := testDist().generateConfig(ctx)
	require.NoError(t, err)
	existing.Restrictions = &cftypes.Restrictions{
		GeoRestriction: &cftypes.GeoRestriction{RestrictionType: cftypes.GeoRestrictionTypeNone, Quantity: aws.Int32(0)},
	}
	existing.CacheBehaviors = &cftypes.CacheBehaviors{Quantity: aws.Int32(1), Items: []cftypes.CacheBehavior{{PathPattern: aws.String("/api/*")}}}
	existing.ContinuousDeploymentPolicyId = aws.String("cdp-123")
	existing.DefaultCacheBehavior.SmoothStreaming = aws.Bool(true)
	existing.DefaultCacheBehavior.FieldLevelEncryptionId = aws.String("fle-1")
	existing.Origins.Items[0].OriginShield = &cftypes.OriginShield{Enabled: aws.Bool(true), OriginShieldRegion: aws.String("us-east-1")}

	desired := mergeManagedConfig(*existing, generated)

	// managed fields overwritten
	assert.Equal(t, "updated comment", aws.ToString(desired.Comment))
	// unmanaged fields preserved
	assert.NotNil(t, desired.Restrictions)
	assert.Equal(t, aws.ToString(existing.ContinuousDeploymentPolicyId), aws.ToString(desired.ContinuousDeploymentPolicyId))
	require.NotNil(t, desired.CacheBehaviors)
	assert.Equal(t, int32(1), aws.ToInt32(desired.CacheBehaviors.Quantity))
	assert.Equal(t, true, aws.ToBool(desired.DefaultCacheBehavior.SmoothStreaming))
	assert.Equal(t, "fle-1", aws.ToString(desired.DefaultCacheBehavior.FieldLevelEncryptionId))
	assert.Equal(t, true, aws.ToBool(desired.Origins.Items[0].OriginShield.Enabled))
}

// TestCloudfrontDistribution_YAMLUnmarshal verifies the YAML tags map a full config
// (as it would appear under resources.cloudfront-distribution) onto the struct, and that
// Normalize + Validate accept it. This exercises the same path config parsing uses.
func TestCloudfrontDistribution_YAMLUnmarshal(t *testing.T) {
	const doc = `
my-cdn:
  comment: "CDN for my-service"
  aliases:
    - cdn.example.com
  certificate: arn:aws:acm:us-east-1:111122223333:certificate/abc-123
  priceClass: PriceClass_100
  httpVersion: http2and3
  webAclName: my-acl
  defaultRootObject: index.html
  logging:
    bucket: my-logs.s3.amazonaws.com
    prefix: cloudfront/my-cdn
  origins:
    - name: primary-cell
      type: vpc
      target: my-service-cell1-vo
      customHeaders:
        x-example-stack: my-service
      responseCompletionTimeout: 40
    - name: static-assets
      type: custom
      target: uswest::my-public-alb
      originProtocolPolicy: https-only
  defaultCacheBehavior:
    targetOriginId: primary-cell
    viewerProtocolPolicy: https-only
    functionAssociations:
      - eventType: viewer-request
        functionARN: arn:aws:cloudfront::111122223333:function/my-vreq
  customErrorResponses:
    - errorCode: 404
      responseCode: "200"
      responsePagePath: /index.html
      errorCachingMinTTL: 10
  tags:
    Name: my-cdn
`

	var parsed map[string]CloudfrontDistribution
	require.NoError(t, yaml.Unmarshal([]byte(doc), &parsed))

	r, ok := parsed["my-cdn"]
	require.True(t, ok)

	// scalar fields
	assert.Equal(t, "CDN for my-service", aws.ToString(r.Comment))
	assert.Equal(t, []string{"cdn.example.com"}, r.Aliases)
	assert.Equal(t, "arn:aws:acm:us-east-1:111122223333:certificate/abc-123", aws.ToString(r.Certificate))
	assert.Equal(t, "index.html", aws.ToString(r.DefaultRootObject))
	assert.Equal(t, "my-acl", aws.ToString(r.WebACLName))

	// origins
	require.Len(t, r.Origins, 2)
	assert.Equal(t, "primary-cell", r.Origins[0].Name)
	assert.Equal(t, CloudfrontOriginTypeVpc, r.Origins[0].Type)
	assert.Equal(t, "my-service-cell1-vo", r.Origins[0].Target)
	assert.Equal(t, "my-service", r.Origins[0].CustomHeaders["x-example-stack"])
	assert.Equal(t, int32(40), aws.ToInt32(r.Origins[0].ResponseCompletionTimeout))
	assert.Equal(t, CloudfrontOriginTypeCustom, r.Origins[1].Type)
	assert.Equal(t, "uswest::my-public-alb", r.Origins[1].Target)

	// behaviors
	assert.Equal(t, "primary-cell", r.DefaultCacheBehavior.TargetOriginId)
	require.Len(t, r.DefaultCacheBehavior.FunctionAssociations, 1)
	assert.Equal(t, "viewer-request", r.DefaultCacheBehavior.FunctionAssociations[0].EventType)

	// custom error responses
	require.Len(t, r.CustomErrorResponses, 1)
	assert.Equal(t, int32(404), r.CustomErrorResponses[0].ErrorCode)

	// Normalize + Validate must accept it
	r.Name = "my-cdn"
	r.Normalize(context.Background())
	assert.NoError(t, r.Validate(context.Background()))
}
