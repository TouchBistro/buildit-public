package resource

import (
	"context"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"sort"
	"strings"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/TouchBistro/buildit/util"
	"github.com/TouchBistro/goutils/color"
	"github.com/aws/aws-sdk-go-v2/aws"
	cftypes "github.com/aws/aws-sdk-go-v2/service/cloudfront/types"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

const (
	// Price classes
	CloudfrontPriceClass100 = string(cftypes.PriceClassPriceClass100)
	CloudfrontPriceClass200 = string(cftypes.PriceClassPriceClass200)
	CloudfrontPriceClassAll = string(cftypes.PriceClassPriceClassAll)

	// HTTP versions
	CloudfrontHTTPVersion11    = string(cftypes.HttpVersionHttp11)
	CloudfrontHTTPVersion2     = string(cftypes.HttpVersionHttp2)
	CloudfrontHTTPVersion3     = string(cftypes.HttpVersionHttp3)
	CloudfrontHTTPVersion2and3 = string(cftypes.HttpVersionHttp2and3)

	// Viewer protocol policies
	CloudfrontViewerProtocolAllowAll        = string(cftypes.ViewerProtocolPolicyAllowAll)
	CloudfrontViewerProtocolHTTPSOnly       = string(cftypes.ViewerProtocolPolicyHttpsOnly)
	CloudfrontViewerProtocolRedirectToHTTPS = string(cftypes.ViewerProtocolPolicyRedirectToHttps)

	// SSL support methods
	CloudfrontSSLSupportSNIOnly  = string(cftypes.SSLSupportMethodSniOnly)
	CloudfrontSSLSupportVIP      = string(cftypes.SSLSupportMethodVip)
	CloudfrontSSLSupportStaticIP = string(cftypes.SSLSupportMethodStaticIp)

	// Minimum TLS protocol version (default)
	CloudfrontMinProtocolTLSv122021 = string(cftypes.MinimumProtocolVersionTLSv122021)

	// Function association event types
	CloudfrontEventViewerRequest  = string(cftypes.EventTypeViewerRequest)
	CloudfrontEventViewerResponse = string(cftypes.EventTypeViewerResponse)

	// Custom origin protocol policies
	CloudfrontOriginProtocolHTTPOnly    = string(cftypes.OriginProtocolPolicyHttpOnly)
	CloudfrontOriginProtocolHTTPSOnly   = string(cftypes.OriginProtocolPolicyHttpsOnly)
	CloudfrontOriginProtocolMatchViewer = string(cftypes.OriginProtocolPolicyMatchViewer)

	// AWS-managed policy IDs used as defaults (mirror wafer's opinionated defaults)
	CloudfrontCachePolicyCachingDisabled        = "4135ea2d-6df8-44a3-9df3-4b5a84be39ad" // Managed-CachingDisabled
	CloudfrontOriginRequestPolicyAllViewerAndCF = "33f36d7e-f396-46d9-90e0-52428a34d9dc" // Managed-AllViewerAndCloudFrontHeaders-2022-06
	CloudfrontOriginRequestPolicyCORSS3         = "88a5eaf4-2fd4-4709-b370-b4c650ea3fcf" // Managed-CORS-S3Origin

	// Managed origin request policies that forward the viewer Host header (see
	// cloudfrontS3IncompatibleOriginRequestPolicies).
	cloudfrontOriginRequestPolicyAllViewer      = "216adef6-5c7f-47e4-b989-5492eafa07d3" // Managed-AllViewer
	cloudfrontOriginRequestPolicyHostHeaderOnly = "bf0718e1-ba1e-49d1-88b1-f726733018ae" // Managed-HostHeaderOnly
)

// The managed policy maps below let the AWS-managed policies be referenced in YAML by
// their documented names (case-insensitive, "Managed-" prefix optional) instead of by
// UUID. Names and ids are published in the CloudFront Developer Guide under
// "Use managed cache policies" / "... origin request policies" / "... response headers
// policies"; these ids are global constants across all AWS accounts.

var cloudfrontManagedCachePolicies = map[string]string{
	"Amplify":                                   "2e54312d-136d-493c-8eb9-b001f22f67d2",
	"CachingDisabled":                           CloudfrontCachePolicyCachingDisabled,
	"CachingOptimized":                          "658327ea-f89d-4fab-a63d-7e88639e58f6",
	"CachingOptimizedForUncompressedObjects":    "b2884449-e4de-46a7-ac36-70bc7f1ddd6d",
	"Elemental-MediaPackage":                    "08627262-05a9-4f76-9ded-b50ca2e3a84f",
	"UseOriginCacheControlHeaders":              "83da9c7e-98b4-4e11-a168-04f0df8e2c65",
	"UseOriginCacheControlHeaders-QueryStrings": "4cc15a8a-d715-48a4-82b8-cc0b614638fe",
}

var cloudfrontManagedOriginRequestPolicies = map[string]string{
	"AllViewer":                                   cloudfrontOriginRequestPolicyAllViewer,
	"AllViewerAndCloudFrontHeaders-2022-06":       CloudfrontOriginRequestPolicyAllViewerAndCF,
	"AllViewerExceptHostHeader":                   "b689b0a8-53d0-40ab-baf2-68738e2966ac",
	"CORS-CustomOrigin":                           "59781a5b-3903-41f3-afcb-af62929ccde1",
	"CORS-S3Origin":                               CloudfrontOriginRequestPolicyCORSS3,
	"Elemental-MediaTailor-PersonalizedManifests": "775133bc-15f2-49f9-abea-afb2e0bf67d2",
	"HostHeaderOnly":                              cloudfrontOriginRequestPolicyHostHeaderOnly,
	"UserAgentRefererHeaders":                     "acba4595-bd28-49b8-b9fe-13317c0390fa",
}

var cloudfrontManagedResponseHeadersPolicies = map[string]string{
	"CORS-and-SecurityHeadersPolicy":                "e61eb60c-9c35-4d20-a928-2b84e02af89c",
	"CORS-With-Preflight":                           "5cc3b908-e619-4b99-88e5-2cf7f45965bd",
	"CORS-with-preflight-and-SecurityHeadersPolicy": "eaab4381-ed33-4a86-88ca-d9558dc6cd63",
	"SecurityHeadersPolicy":                         "67f7725c-6f97-4210-82d7-5512b31e9d03",
	"SimpleCORS":                                    "60669652-455b-4ae9-85a4-c4c02393f86c",
}

// cloudfrontS3IncompatibleOriginRequestPolicies are rejected by the CloudFront API when
// the behavior's target origin is an S3 bucket endpoint (they forward the viewer Host
// header, which breaks S3 request routing).
var cloudfrontS3IncompatibleOriginRequestPolicies = map[string]string{
	cloudfrontOriginRequestPolicyAllViewer:      "AllViewer",
	CloudfrontOriginRequestPolicyAllViewerAndCF: "AllViewerAndCloudFrontHeaders-2022-06",
	cloudfrontOriginRequestPolicyHostHeaderOnly: "HostHeaderOnly",
}

// cloudfrontDefaultAllowedMethods mirrors wafer's default allowed methods (full API support).
var cloudfrontDefaultAllowedMethods = []string{"GET", "HEAD", "OPTIONS", "PUT", "POST", "PATCH", "DELETE"}

// cloudfrontDefaultCachedMethods is the CloudFront default set of cached methods.
var cloudfrontDefaultCachedMethods = []string{"GET", "HEAD"}

// CloudFront accepts only these exact method sets for a behavior's allowed and cached
// methods; each set is stored sorted so a sorted input can be compared with slices.Equal.
var (
	cloudfrontValidAllowedMethodSets = [][]string{
		{"GET", "HEAD"},
		{"GET", "HEAD", "OPTIONS"},
		{"DELETE", "GET", "HEAD", "OPTIONS", "PATCH", "POST", "PUT"},
	}
	cloudfrontValidCachedMethodSets = [][]string{
		{"GET", "HEAD"},
		{"GET", "HEAD", "OPTIONS"},
	}
)

// Canonical enum forms derived from the SDK so new values are picked up on upgrades.
var (
	cloudfrontMinProtocolVersions = enumStrings(cftypes.MinimumProtocolVersion("").Values())
	cloudfrontSSLProtocols        = enumStrings(cftypes.SslProtocol("").Values())
)

func enumStrings[T ~string](vals []T) []string {
	out := make([]string, 0, len(vals))
	for _, v := range vals {
		out = append(out, string(v))
	}
	return out
}

// CloudfrontDistribution is a declarative AWS CloudFront distribution resource.
// It exposes the properties wafer manages as user-settable YAML, applying wafer's
// opinionated values as defaults in Normalize.
type CloudfrontDistribution struct {
	BaseResource         `yaml:",inline"`
	Name                 string                          `yaml:"-"` // from resource key; used as CallerReference + comparison identifier
	Comment              *string                         `yaml:"comment"`
	Enabled              *bool                           `yaml:"enabled"`
	DefaultRootObject    *string                         `yaml:"defaultRootObject"`
	Aliases              []string                        `yaml:"aliases"`
	HTTPVersion          *string                         `yaml:"httpVersion"`
	IsIPV6Enabled        *bool                           `yaml:"isIPV6Enabled"`
	PriceClass           *string                         `yaml:"priceClass"`
	WebACLName           *string                         `yaml:"webAclName"`  // global (CloudFront-scope) WAFv2 web ACL Name; nil/empty => no web ACL
	Certificate          *string                         `yaml:"certificate"` // ACM ARN or domain identifier; nil => CloudFront default cert
	MinProtocolVersion   *string                         `yaml:"minimumProtocolVersion"`
	SSLSupportMethod     *string                         `yaml:"sslSupportMethod"`
	Logging              *CloudfrontLogging              `yaml:"logging"`
	Origins              []CloudfrontOrigin              `yaml:"origins"`
	DefaultCacheBehavior CloudfrontCacheBehavior         `yaml:"defaultCacheBehavior"`
	CustomErrorResponses []CloudfrontCustomErrorResponse `yaml:"customErrorResponses"`
	Tags                 map[string]string               `yaml:"tags"`
	GlobalTags           map[string]string               `yaml:"-"`
	DependsOn            []Key                           `yaml:"dependsOn"`

	// displayValues maps resolved AWS values (ARNs, ids, domain names) back to the form
	// the user supplied (names, targets) so diff messages read like the YAML config.
	// Created by Normalize and shared across receiver copies; used only for rendering,
	// never for comparison.
	displayValues map[string]string `yaml:"-"`
}

// CloudfrontLogging configures access logging for the distribution.
type CloudfrontLogging struct {
	Bucket         string  `yaml:"bucket"` // S3 bucket domain, e.g. my-logs.s3.amazonaws.com
	Prefix         *string `yaml:"prefix"`
	IncludeCookies *bool   `yaml:"includeCookies"`
	Enabled        *bool   `yaml:"enabled"`
}

// Origin types.
const (
	CloudfrontOriginTypeVpc    = "vpc"
	CloudfrontOriginTypeS3     = "s3"
	CloudfrontOriginTypeCustom = "custom"
)

// CloudfrontOrigin describes a single distribution origin as a generic (type, target)
// pair. The origin domain name is never supplied in YAML; it is inferred from the target
// based on the type:
//
//   - vpc:    target is a VPC origin id (vo_...) or Name. The domain is the DNS name of
//     the load balancer the VPC origin points at.
//   - s3:     target is a bucket name; the domain is the bucket's regional S3 endpoint.
//   - custom: target is an (internet-facing) load balancer name resolved to its DNS
//     name, or a literal domain name (anything containing a dot) used as-is.
//
// The target may carry a `provider::` prefix to resolve the underlying load balancer or
// bucket via a different provider (e.g. a VPC origin whose ALB lives in another region).
type CloudfrontOrigin struct {
	Name                      string            `yaml:"name"` // CloudFront origin id; must be unique within the distribution
	Type                      string            `yaml:"type"` // vpc | s3 | custom
	Target                    string            `yaml:"target"`
	Path                      *string           `yaml:"path"`
	CustomHeaders             map[string]string `yaml:"customHeaders"`
	ConnectionAttempts        *int32            `yaml:"connectionAttempts"`
	ConnectionTimeout         *int32            `yaml:"connectionTimeout"`
	OriginReadTimeout         *int32            `yaml:"originReadTimeout"`         // vpc | custom only
	OriginKeepAliveTimeout    *int32            `yaml:"originKeepAliveTimeout"`    // vpc | custom only
	ResponseCompletionTimeout *int32            `yaml:"responseCompletionTimeout"` // all types; nil = disabled (no maximum enforced)
	OriginAccessControlId     *string           `yaml:"originAccessControlId"`
	// custom origins only
	HTTPPort             *int32   `yaml:"httpPort"`
	HTTPSPort            *int32   `yaml:"httpsPort"`
	OriginProtocolPolicy *string  `yaml:"originProtocolPolicy"`
	OriginSSLProtocols   []string `yaml:"originSslProtocols"`

	// resolved values, populated by resolveOrigins (pre-populated in tests to avoid AWS calls)
	resolvedDomainName  string `yaml:"-"`
	resolvedVpcOriginId string `yaml:"-"`
}

// CloudfrontCacheBehavior configures the default cache behavior. Ordered (path-pattern)
// cache behaviors are not managed by buildit; any that exist on the distribution are
// preserved untouched on update.
type CloudfrontCacheBehavior struct {
	TargetOriginId          string                          `yaml:"targetOriginId"`
	ViewerProtocolPolicy    *string                         `yaml:"viewerProtocolPolicy"`
	AllowedMethods          []string                        `yaml:"allowedMethods"`
	CachedMethods           []string                        `yaml:"cachedMethods"`
	CachePolicyId           *string                         `yaml:"cachePolicyId"`
	OriginRequestPolicyId   *string                         `yaml:"originRequestPolicyId"`
	ResponseHeadersPolicyId *string                         `yaml:"responseHeadersPolicyId"`
	Compress                *bool                           `yaml:"compress"`
	FunctionAssociations    []CloudfrontFunctionAssociation `yaml:"functionAssociations"`
}

// CloudfrontFunctionAssociation associates a CloudFront Function with an event type.
type CloudfrontFunctionAssociation struct {
	EventType   string `yaml:"eventType"` // viewer-request | viewer-response
	FunctionARN string `yaml:"functionARN"`
}

// CloudfrontCustomErrorResponse customizes the response for an origin error code.
type CloudfrontCustomErrorResponse struct {
	ErrorCode          int32   `yaml:"errorCode"`
	ResponseCode       *string `yaml:"responseCode"`
	ResponsePagePath   *string `yaml:"responsePagePath"`
	ErrorCachingMinTTL *int64  `yaml:"errorCachingMinTTL"`
}

// Key returns the unique key for the resource for this buildit context.
func (c CloudfrontDistribution) Key() Key {
	return NewKey(c.Context.ProviderName, c.Identifier())
}

// Identifier returns the distribution name (also used as the CallerReference).
func (c CloudfrontDistribution) Identifier() string {
	return c.Name
}

// Normalize sets default values mirroring wafer's opinionated configuration.
func (c *CloudfrontDistribution) Normalize(ctx context.Context) {
	// Seed the display map with the managed policy names so diff messages render the
	// documented policy names instead of their UUIDs, whichever form the user supplied.
	if c.displayValues == nil {
		c.displayValues = make(map[string]string)
	}
	for _, m := range []map[string]string{cloudfrontManagedCachePolicies, cloudfrontManagedOriginRequestPolicies, cloudfrontManagedResponseHeadersPolicies} {
		for name, id := range m {
			c.displayValues[id] = name
		}
	}

	// Enum-valued fields are case-insensitive in YAML; canonicalize them first so
	// defaulting, validation, and AWS comparison always see the exact CloudFront
	// constant. Origin types must be canonical before normalizeCacheBehavior and the
	// per-origin defaulting below, both of which switch on the type.
	normalizeCloudfrontEnum(c.HTTPVersion, CloudfrontHTTPVersion11, CloudfrontHTTPVersion2, CloudfrontHTTPVersion3, CloudfrontHTTPVersion2and3)
	normalizeCloudfrontEnum(c.PriceClass, CloudfrontPriceClass100, CloudfrontPriceClass200, CloudfrontPriceClassAll)
	normalizeCloudfrontEnum(c.SSLSupportMethod, CloudfrontSSLSupportSNIOnly, CloudfrontSSLSupportVIP, CloudfrontSSLSupportStaticIP)
	normalizeCloudfrontEnum(c.MinProtocolVersion, cloudfrontMinProtocolVersions...)
	for i := range c.Origins {
		o := &c.Origins[i]
		o.Type = canonicalCloudfrontEnum(o.Type, CloudfrontOriginTypeVpc, CloudfrontOriginTypeS3, CloudfrontOriginTypeCustom)
		normalizeCloudfrontEnum(o.OriginProtocolPolicy, CloudfrontOriginProtocolHTTPOnly, CloudfrontOriginProtocolHTTPSOnly, CloudfrontOriginProtocolMatchViewer)
		for j := range o.OriginSSLProtocols {
			o.OriginSSLProtocols[j] = canonicalCloudfrontEnum(o.OriginSSLProtocols[j], cloudfrontSSLProtocols...)
		}
	}

	if c.Enabled == nil {
		c.Enabled = aws.Bool(true)
	}
	if c.HTTPVersion == nil {
		c.HTTPVersion = aws.String(CloudfrontHTTPVersion2and3)
	}
	if c.IsIPV6Enabled == nil {
		c.IsIPV6Enabled = aws.Bool(false) // matches the CloudFront CreateDistribution API default
	}
	if c.PriceClass == nil {
		c.PriceClass = aws.String(CloudfrontPriceClass100)
	}
	if c.MinProtocolVersion == nil {
		c.MinProtocolVersion = aws.String(CloudfrontMinProtocolTLSv122021)
	}
	if c.SSLSupportMethod == nil {
		c.SSLSupportMethod = aws.String(CloudfrontSSLSupportSNIOnly)
	}

	c.normalizeCacheBehavior(&c.DefaultCacheBehavior)

	for i := range c.Origins {
		o := &c.Origins[i]
		if o.ConnectionAttempts == nil {
			o.ConnectionAttempts = aws.Int32(3)
		}
		if o.ConnectionTimeout == nil {
			o.ConnectionTimeout = aws.Int32(10)
		}
		if o.Path == nil {
			o.Path = aws.String("")
		}
		// Read/keep-alive timeouts do not exist for S3 origins; only default them for
		// the types that carry them so Validate can reject explicit values on s3.
		if o.Type == CloudfrontOriginTypeVpc || o.Type == CloudfrontOriginTypeCustom {
			if o.OriginKeepAliveTimeout == nil {
				o.OriginKeepAliveTimeout = aws.Int32(5)
			}
			if o.OriginReadTimeout == nil {
				o.OriginReadTimeout = aws.Int32(30)
			}
		}
		if o.Type == CloudfrontOriginTypeCustom {
			if o.HTTPPort == nil {
				o.HTTPPort = aws.Int32(80)
			}
			if o.HTTPSPort == nil {
				o.HTTPSPort = aws.Int32(443)
			}
			if o.OriginProtocolPolicy == nil {
				o.OriginProtocolPolicy = aws.String(CloudfrontOriginProtocolHTTPSOnly)
			}
			if len(o.OriginSSLProtocols) == 0 {
				o.OriginSSLProtocols = []string{string(cftypes.SslProtocolTLSv12)}
			}
		}
	}

	if c.Logging != nil {
		if c.Logging.Enabled == nil {
			c.Logging.Enabled = aws.Bool(true)
		}
		if c.Logging.IncludeCookies == nil {
			c.Logging.IncludeCookies = aws.Bool(false)
		}
	}

	if c.Tags == nil {
		c.Tags = make(map[string]string)
	}
	ResourceTags(c.Tags).Merge(c.GlobalTags)
}

// normalizeCacheBehavior canonicalizes case-insensitive values and applies wafer's
// default cache-behavior values.
func (c *CloudfrontDistribution) normalizeCacheBehavior(b *CloudfrontCacheBehavior) {
	normalizeCloudfrontEnum(b.ViewerProtocolPolicy, CloudfrontViewerProtocolAllowAll, CloudfrontViewerProtocolHTTPSOnly, CloudfrontViewerProtocolRedirectToHTTPS)
	for i := range b.AllowedMethods {
		b.AllowedMethods[i] = strings.ToUpper(b.AllowedMethods[i])
	}
	for i := range b.CachedMethods {
		b.CachedMethods[i] = strings.ToUpper(b.CachedMethods[i])
	}
	for i := range b.FunctionAssociations {
		fa := &b.FunctionAssociations[i]
		fa.EventType = canonicalCloudfrontEnum(fa.EventType, CloudfrontEventViewerRequest, CloudfrontEventViewerResponse)
	}
	// The policy fields accept AWS-managed policy names; canonicalize those to ids here
	// so validation and comparison work on ids. Values that are neither ids nor managed
	// names are treated as custom policy names and resolved via the CloudFront API when
	// the config is generated.
	normalizeManagedPolicyId(b.CachePolicyId, cloudfrontManagedCachePolicies)
	normalizeManagedPolicyId(b.OriginRequestPolicyId, cloudfrontManagedOriginRequestPolicies)
	normalizeManagedPolicyId(b.ResponseHeadersPolicyId, cloudfrontManagedResponseHeadersPolicies)

	if b.ViewerProtocolPolicy == nil {
		b.ViewerProtocolPolicy = aws.String(CloudfrontViewerProtocolHTTPSOnly)
	}
	if len(b.AllowedMethods) == 0 {
		b.AllowedMethods = append([]string(nil), cloudfrontDefaultAllowedMethods...)
	}
	// CloudFront populates CachedMethods to GET,HEAD on read-back; default it here so the
	// desired config matches the AWS representation and does not produce a perpetual diff.
	if len(b.CachedMethods) == 0 {
		b.CachedMethods = append([]string(nil), cloudfrontDefaultCachedMethods...)
	}
	if b.CachePolicyId == nil {
		b.CachePolicyId = aws.String(CloudfrontCachePolicyCachingDisabled)
	}
	if b.OriginRequestPolicyId == nil {
		// CloudFront rejects Host-header-forwarding policies (including wafer's default
		// AllViewerAndCloudFrontHeaders) on behaviors targeting an S3 origin, so default
		// those to the S3-safe managed CORS-S3Origin policy instead.
		if c.originType(b.TargetOriginId) == CloudfrontOriginTypeS3 {
			b.OriginRequestPolicyId = aws.String(CloudfrontOriginRequestPolicyCORSS3)
		} else {
			b.OriginRequestPolicyId = aws.String(CloudfrontOriginRequestPolicyAllViewerAndCF)
		}
	}
	if b.Compress == nil {
		b.Compress = aws.Bool(false)
	}
}

// recordDisplayValue remembers the user-supplied form of a resolved AWS value so diff
// messages show what the user wrote (a web ACL name, an origin target, a policy name)
// instead of the resolved ARN/id. Entries go into the displayValues map created by
// Normalize; maps are reference types, so entries persist across the value-receiver
// copies used throughout the resource lifecycle.
func (c CloudfrontDistribution) recordDisplayValue(resolved, supplied string) {
	if c.displayValues == nil || resolved == "" || resolved == supplied {
		return
	}
	c.displayValues[resolved] = supplied
}

// originType returns the declared type of the origin with the supplied name, or ""
// when no origin matches.
func (c CloudfrontDistribution) originType(name string) string {
	for _, o := range c.Origins {
		if o.Name == name {
			return o.Type
		}
	}
	return ""
}

// Validate checks the distribution configuration. It performs structural validation
// only; AWS lookups (certificate resolution) happen at apply time.
func (c CloudfrontDistribution) Validate(ctx context.Context) error {
	var msgs []string

	if len(c.Origins) == 0 {
		msgs = append(msgs, "at least one origin must be specified")
	}

	originIds := make(map[string]struct{})
	for i, o := range c.Origins {
		if o.Name == "" {
			msgs = append(msgs, fmt.Sprintf("origin[%d]: name is required", i))
		} else {
			if _, dup := originIds[o.Name]; dup {
				msgs = append(msgs, fmt.Sprintf("duplicate origin name %q", o.Name))
			}
			originIds[o.Name] = struct{}{}
		}
		switch o.Type {
		case CloudfrontOriginTypeVpc, CloudfrontOriginTypeS3, CloudfrontOriginTypeCustom:
		case "":
			msgs = append(msgs, fmt.Sprintf("origin %q: type is required (vpc, s3, or custom)", o.Name))
		default:
			msgs = append(msgs, fmt.Sprintf("origin %q: invalid type %q (must be vpc, s3, or custom)", o.Name, o.Type))
		}
		if o.Target == "" {
			msgs = append(msgs, fmt.Sprintf("origin %q: target is required", o.Name))
		}
		if o.Type != CloudfrontOriginTypeCustom {
			// Normalize only defaults these for custom origins, so any non-nil value
			// here was supplied by the user.
			if o.HTTPPort != nil || o.HTTPSPort != nil || o.OriginProtocolPolicy != nil || len(o.OriginSSLProtocols) > 0 {
				msgs = append(msgs, fmt.Sprintf("origin %q: httpPort, httpsPort, originProtocolPolicy, and originSslProtocols only apply to custom origins", o.Name))
			}
		}
		if o.Type == CloudfrontOriginTypeS3 && (o.OriginReadTimeout != nil || o.OriginKeepAliveTimeout != nil) {
			msgs = append(msgs, fmt.Sprintf("origin %q: originReadTimeout and originKeepAliveTimeout do not apply to s3 origins", o.Name))
		}
		// AWS requires ResponseCompletionTimeout >= OriginReadTimeout when both are set.
		if o.ResponseCompletionTimeout != nil && o.OriginReadTimeout != nil &&
			*o.ResponseCompletionTimeout < *o.OriginReadTimeout {
			msgs = append(msgs, fmt.Sprintf("origin %q: responseCompletionTimeout (%d) must be >= originReadTimeout (%d)", o.Name, *o.ResponseCompletionTimeout, *o.OriginReadTimeout))
		}
		if o.Type == CloudfrontOriginTypeCustom && o.OriginProtocolPolicy != nil {
			switch *o.OriginProtocolPolicy {
			case CloudfrontOriginProtocolHTTPOnly, CloudfrontOriginProtocolHTTPSOnly, CloudfrontOriginProtocolMatchViewer:
			default:
				msgs = append(msgs, fmt.Sprintf("origin %q: invalid originProtocolPolicy %q", o.Name, *o.OriginProtocolPolicy))
			}
		}
	}

	// enum validation
	if !validCloudfrontEnum(c.HTTPVersion, CloudfrontHTTPVersion11, CloudfrontHTTPVersion2, CloudfrontHTTPVersion3, CloudfrontHTTPVersion2and3) {
		msgs = append(msgs, fmt.Sprintf("invalid httpVersion %q", aws.ToString(c.HTTPVersion)))
	}
	if !validCloudfrontEnum(c.PriceClass, CloudfrontPriceClass100, CloudfrontPriceClass200, CloudfrontPriceClassAll) {
		msgs = append(msgs, fmt.Sprintf("invalid priceClass %q", aws.ToString(c.PriceClass)))
	}
	if !validCloudfrontEnum(c.SSLSupportMethod, CloudfrontSSLSupportSNIOnly, CloudfrontSSLSupportVIP, CloudfrontSSLSupportStaticIP) {
		msgs = append(msgs, fmt.Sprintf("invalid sslSupportMethod %q", aws.ToString(c.SSLSupportMethod)))
	}

	// default cache behavior (ordered cache behaviors are not managed by buildit)
	msgs = append(msgs, c.validateCacheBehavior(c.DefaultCacheBehavior, originIds, "defaultCacheBehavior")...)

	// aliases require a certificate
	if len(c.Aliases) > 0 && c.Certificate == nil {
		msgs = append(msgs, "aliases require a certificate (ACM ARN or domain) to be specified")
	}

	// logging requires a bucket
	if c.Logging != nil && c.Logging.Bucket == "" {
		msgs = append(msgs, "logging.bucket is required when logging is configured")
	}

	// the resource name is used as the CallerReference, which is limited to 128 characters
	if len(c.Name) > 128 {
		msgs = append(msgs, fmt.Sprintf("name cannot be longer than 128 characters, current length: %d", len(c.Name)))
	}

	if len(msgs) == 0 {
		return nil
	}
	return &ValidationError{
		ResourceIdentifier: c.Identifier(),
		ResourceType:       "CloudFront Distribution",
		Messages:           msgs,
	}
}

// validateCacheBehavior validates a single cache behavior; ctxLabel is used in messages.
func (c CloudfrontDistribution) validateCacheBehavior(b CloudfrontCacheBehavior, originIds map[string]struct{}, ctxLabel string) []string {
	var msgs []string
	if b.TargetOriginId == "" {
		msgs = append(msgs, fmt.Sprintf("%s: targetOriginId is required", ctxLabel))
	} else if _, ok := originIds[b.TargetOriginId]; !ok {
		msgs = append(msgs, fmt.Sprintf("%s: targetOriginId %q does not match any origin id", ctxLabel, b.TargetOriginId))
	}
	if !validCloudfrontEnum(b.ViewerProtocolPolicy, CloudfrontViewerProtocolAllowAll, CloudfrontViewerProtocolHTTPSOnly, CloudfrontViewerProtocolRedirectToHTTPS) {
		msgs = append(msgs, fmt.Sprintf("%s: invalid viewerProtocolPolicy %q", ctxLabel, aws.ToString(b.ViewerProtocolPolicy)))
	}
	if b.OriginRequestPolicyId != nil && c.originType(b.TargetOriginId) == CloudfrontOriginTypeS3 {
		if policyName, bad := cloudfrontS3IncompatibleOriginRequestPolicies[*b.OriginRequestPolicyId]; bad {
			msgs = append(msgs, fmt.Sprintf("%s: origin request policy %s (%s) cannot be used with s3 target origin %q", ctxLabel, policyName, *b.OriginRequestPolicyId, b.TargetOriginId))
		}
	}
	msgs = append(msgs, validateCloudfrontMethods(ctxLabel, b.AllowedMethods, b.CachedMethods)...)
	for _, fa := range b.FunctionAssociations {
		switch fa.EventType {
		case CloudfrontEventViewerRequest, CloudfrontEventViewerResponse:
		default:
			msgs = append(msgs, fmt.Sprintf("%s: invalid function association eventType %q", ctxLabel, fa.EventType))
		}
		if fa.FunctionARN == "" {
			msgs = append(msgs, fmt.Sprintf("%s: function association functionARN is required", ctxLabel))
		}
	}
	return msgs
}

// validCloudfrontEnum returns true if val is nil-safe-equal to one of the allowed values.
func validCloudfrontEnum(val *string, allowed ...string) bool {
	return slices.Contains(allowed, aws.ToString(val))
}

// validateCloudfrontMethods enforces CloudFront's rules for a behavior's methods at plan
// time: allowedMethods and cachedMethods must each be exactly one of the method sets
// CloudFront accepts (duplicates rejected), and every cached method must also be allowed.
func validateCloudfrontMethods(ctxLabel string, allowed, cached []string) []string {
	var msgs []string
	if !slices.ContainsFunc(cloudfrontValidAllowedMethodSets, func(s []string) bool { return equalMethodSet(allowed, s) }) {
		msgs = append(msgs, fmt.Sprintf("%s: invalid allowedMethods %v; must be one of [GET HEAD], [GET HEAD OPTIONS], or [GET HEAD OPTIONS PUT POST PATCH DELETE]", ctxLabel, allowed))
	}
	if !slices.ContainsFunc(cloudfrontValidCachedMethodSets, func(s []string) bool { return equalMethodSet(cached, s) }) {
		msgs = append(msgs, fmt.Sprintf("%s: invalid cachedMethods %v; must be [GET HEAD] or [GET HEAD OPTIONS]", ctxLabel, cached))
	}
	for _, m := range cached {
		if !slices.Contains(allowed, m) {
			msgs = append(msgs, fmt.Sprintf("%s: cached method %q must also be in allowedMethods", ctxLabel, m))
		}
	}
	return msgs
}

// equalMethodSet reports whether methods equals want ignoring order; want must be sorted.
func equalMethodSet(methods, want []string) bool {
	m := append([]string(nil), methods...)
	sort.Strings(m)
	return slices.Equal(m, want)
}

// canonicalCloudfrontEnum returns the canonical form of val when it matches one of the
// canonical values case-insensitively, or val unchanged so Validate can report it.
func canonicalCloudfrontEnum(val string, canonical ...string) string {
	for _, c := range canonical {
		if strings.EqualFold(val, c) {
			return c
		}
	}
	return val
}

// normalizeCloudfrontEnum rewrites *val in place to its canonical form (nil-safe).
func normalizeCloudfrontEnum(val *string, canonical ...string) {
	if val != nil {
		*val = canonicalCloudfrontEnum(*val, canonical...)
	}
}

// normalizeManagedPolicyId rewrites *val in place to the managed policy id when the
// value names one of the AWS-managed policies (case-insensitive, "Managed-" prefix
// optional). Ids and unknown names (custom policies) are left untouched.
func normalizeManagedPolicyId(val *string, managed map[string]string) {
	if val == nil || awsw.IsCloudfrontPolicyId(*val) {
		return
	}
	for name, id := range managed {
		if strings.EqualFold(name, *val) || strings.EqualFold("Managed-"+name, *val) {
			*val = id
			return
		}
	}
}

// Apply creates or updates the distribution.
func (c CloudfrontDistribution) Apply(ctx context.Context) error {
	log.Debugf("applying cloudfront distribution %v", c.Identifier())

	diffs, err := c.Compare(ctx)
	if err != nil {
		return err
	}

	if diffs == nil {
		log.WithFields(log.Fields{"Name": c.Identifier()}).Info("no updates required")
		return nil
	}

	if diffs.AWSResource() != nil {
		// Surface the change set at Info so operators can see what is about to change on a
		// high-blast-radius edge resource without enabling debug logging.
		for _, d := range diffs.Differences() {
			log.WithField("Name", c.Identifier()).Info(d)
		}
		return c.applyDiffs(ctx, diffs)
	}

	return c.apply(ctx)
}

// apply creates a new distribution.
func (c CloudfrontDistribution) apply(ctx context.Context) error {
	cfg, err := c.generateConfig(ctx)
	if err != nil {
		return err
	}

	cfClient := awsw.NewCloudfront(ctx, c.Context.ProviderName)
	out, err := cfClient.CreateDistributionWithTags(ctx, cfg, c.Tags)
	if err != nil {
		return err
	}

	id := aws.ToString(out.Distribution.Id)
	log.WithFields(log.Fields{"Name": c.Identifier(), "Id": id}).Info(color.Green("cloudfront distribution created (deploying)"))
	return nil
}

// applyDiffs updates an existing distribution and reconciles its tags.
func (c CloudfrontDistribution) applyDiffs(ctx context.Context, diffs ResourceDiff) error {
	cfDiff, ok := diffs.(*CloudfrontDistributionDiff)
	if !ok {
		return errors.New("invalid diff type supplied")
	}

	cfClient := awsw.NewCloudfront(ctx, c.Context.ProviderName)

	if cfDiff.configChanged {
		if _, err := cfClient.UpdateDistributionConfig(ctx, cfDiff.id, cfDiff.etag, cfDiff.desired); err != nil {
			return err
		}
	}

	if cfDiff.tagsDiff {
		if upserts := cfDiff.tagDiff.Upserts(); len(upserts) > 0 {
			if err := cfClient.AddDistributionTags(ctx, cfDiff.arn, upserts); err != nil {
				return err
			}
		}
		if keys := cfDiff.tagDiff.DeletedKeys(); len(keys) > 0 {
			if err := cfClient.DeleteDistributionTags(ctx, cfDiff.arn, keys); err != nil {
				return err
			}
		}
	}

	log.WithFields(log.Fields{"Name": c.Identifier(), "Id": cfDiff.id}).Info(color.Yellow("cloudfront distribution updated"))
	return nil
}

// Wait waits for any in-progress distribution deploy to complete. It implements
// WaitableResource, so the DAG runs all waits concurrently after every Apply has
// finished (see config/dag.go). Nothing in buildit depends on a distribution being
// Deployed (a route53 alias only needs its domain name, available at create time), so
// deferring the wait is safe. Destroy does NOT rely on this: it performs its own inline
// wait, since AWS rejects deleting a distribution whose status is InProgress.
func (c CloudfrontDistribution) Wait(ctx context.Context) error {
	cfClient := awsw.NewCloudfront(ctx, c.Context.ProviderName)
	existing, err := cfClient.FindDistributionByCallerReference(ctx, c.Identifier())
	if err != nil {
		return err
	}
	if existing == nil {
		// nothing to wait on (e.g. a targeted apply that skipped this resource)
		return nil
	}

	log.WithFields(log.Fields{"Name": c.Identifier(), "Id": existing.Id}).Info("waiting for cloudfront distribution to deploy")
	if err := cfClient.WaitForDeployed(ctx, existing.Id); err != nil {
		return errors.Wrapf(err, "cloudfront distribution %v failed to deploy", c.Identifier())
	}
	log.WithFields(log.Fields{"Name": c.Identifier(), "Id": existing.Id}).Info(color.Green("cloudfront distribution deployed"))
	return nil
}

// Destroy disables (if needed) and deletes the distribution. No-op if it does not exist.
func (c CloudfrontDistribution) Destroy(ctx context.Context) error {
	log.Debugf("destroying cloudfront distribution %v", c.Identifier())

	cfClient := awsw.NewCloudfront(ctx, c.Context.ProviderName)
	existing, err := cfClient.FindDistributionByCallerReference(ctx, c.Identifier())
	if err != nil {
		return err
	}
	if existing == nil {
		log.WithFields(log.Fields{"Name": c.Identifier()}).Info("cloudfront distribution does not exist, nothing to destroy")
		return nil
	}

	etag := existing.ETag

	// CloudFront requires a distribution to be disabled before it can be deleted.
	if aws.ToBool(existing.Config.Enabled) {
		disabled := existing.Config
		disabled.Enabled = aws.Bool(false)
		log.WithFields(log.Fields{"Name": c.Identifier(), "Id": existing.Id}).Info("disabling cloudfront distribution before delete")
		newETag, err := cfClient.UpdateDistributionConfig(ctx, existing.Id, etag, &disabled)
		if err != nil {
			return err
		}
		etag = newETag
	}

	// CloudFront refuses to delete a distribution whose status is InProgress, so wait for
	// it to be deployed regardless of whether we just disabled it (it may have been left
	// disabled-but-in-progress by a prior interrupted destroy).
	if err := cfClient.WaitForDeployed(ctx, existing.Id); err != nil {
		return errors.Wrapf(err, "cloudfront distribution %v failed to reach deployed state before delete", c.Identifier())
	}

	if err := cfClient.DeleteDistribution(ctx, existing.Id, etag); err != nil {
		return err
	}

	log.WithFields(log.Fields{"Name": c.Identifier(), "Id": existing.Id}).Info(color.Red("cloudfront distribution destroyed"))
	return nil
}

// CloudfrontDistributionDiff captures the diff between desired and existing state.
type CloudfrontDistributionDiff struct {
	BaseResourceDiff

	id            string
	arn           string
	etag          string
	desired       *cftypes.DistributionConfig
	configChanged bool
	tagsDiff      bool
	tagDiff       util.TagDiffResult
}

// Compare fetches the existing distribution and diffs it against the desired config.
func (c CloudfrontDistribution) Compare(ctx context.Context) (ResourceDiff, error) {
	cfClient := awsw.NewCloudfront(ctx, c.Context.ProviderName)
	existing, err := cfClient.FindDistributionByCallerReference(ctx, c.Identifier())
	if err != nil {
		return nil, err
	}

	diffs := &CloudfrontDistributionDiff{}
	if existing == nil {
		diffs.Messages = append(diffs.Messages, "cloudfront distribution does not exist")
		return diffs, nil
	}

	generated, err := c.generateConfig(ctx)
	if err != nil {
		return nil, err
	}

	desired := mergeManagedConfig(existing.Config, generated)

	diffs.Resource = existing
	diffs.id = existing.Id
	diffs.arn = existing.ARN
	diffs.etag = existing.ETag
	diffs.desired = &desired

	configMsgs := compareDistributionConfig(existing.Config, desired, c.displayValues)
	if len(configMsgs) > 0 {
		diffs.configChanged = true
		diffs.Messages = append(diffs.Messages, configMsgs...)
	}

	if tagDiff := TagDiffForContext(ctx, existing.Tags, c.Tags); tagDiff.HasChanges() {
		diffs.tagsDiff = true
		diffs.tagDiff = tagDiff
		diffs.Messages = append(diffs.Messages, TagDiffSummary(existing.Tags, tagDiff)...)
	}

	if !diffs.configChanged && !diffs.tagsDiff {
		return nil, nil
	}
	return diffs, nil
}

// mergeManagedConfig builds the desired config for UpdateDistribution on top of the
// existing one, overwriting only buildit-managed fields (recursively: managed leaves of
// the default cache behavior and of each origin are overwritten, everything else in
// those structs is preserved). UpdateDistribution replaces the complete distribution
// config and rejects incomplete structs (e.g. "The parameter SmoothStreaming flag is
// missing"), so unmanaged fields — Restrictions, OriginGroups, ordered CacheBehaviors,
// ContinuousDeploymentPolicyId, SmoothStreaming, TrustedSigners/KeyGroups, Lambda@Edge
// associations, GrpcConfig, OriginShield, IpAddressType, ... — must survive untouched.
// CallerReference stays as-is (immutable; it matched the lookup).
func mergeManagedConfig(existing cftypes.DistributionConfig, generated *cftypes.DistributionConfig) cftypes.DistributionConfig {
	desired := existing
	desired.Comment = generated.Comment
	desired.Enabled = generated.Enabled
	desired.DefaultRootObject = generated.DefaultRootObject
	desired.HttpVersion = generated.HttpVersion
	desired.IsIPV6Enabled = generated.IsIPV6Enabled
	desired.PriceClass = generated.PriceClass
	desired.WebACLId = generated.WebACLId
	desired.Aliases = generated.Aliases
	desired.ViewerCertificate = generated.ViewerCertificate
	desired.Logging = generated.Logging
	desired.CustomErrorResponses = generated.CustomErrorResponses

	// default cache behavior: managed fields only
	if existing.DefaultCacheBehavior != nil && generated.DefaultCacheBehavior != nil {
		db := *existing.DefaultCacheBehavior
		g := generated.DefaultCacheBehavior
		db.TargetOriginId = g.TargetOriginId
		db.ViewerProtocolPolicy = g.ViewerProtocolPolicy
		db.AllowedMethods = g.AllowedMethods
		db.CachePolicyId = g.CachePolicyId
		db.OriginRequestPolicyId = g.OriginRequestPolicyId
		db.ResponseHeadersPolicyId = g.ResponseHeadersPolicyId
		db.Compress = g.Compress
		db.FunctionAssociations = g.FunctionAssociations
		desired.DefaultCacheBehavior = &db
	} else {
		desired.DefaultCacheBehavior = generated.DefaultCacheBehavior
	}

	// origins: merge per origin by id (generated order); origins the config no longer
	// declares are dropped, new ones are taken from the generated config as-is
	existingOrigins := make(map[string]cftypes.Origin)
	if existing.Origins != nil {
		for _, o := range existing.Origins.Items {
			existingOrigins[aws.ToString(o.Id)] = o
		}
	}
	items := make([]cftypes.Origin, 0, len(generated.Origins.Items))
	for _, g := range generated.Origins.Items {
		e, ok := existingOrigins[aws.ToString(g.Id)]
		if !ok {
			items = append(items, g)
			continue
		}
		e.DomainName = g.DomainName
		e.OriginPath = g.OriginPath
		e.ConnectionAttempts = g.ConnectionAttempts
		e.ConnectionTimeout = g.ConnectionTimeout
		e.ResponseCompletionTimeout = g.ResponseCompletionTimeout
		e.OriginAccessControlId = g.OriginAccessControlId
		e.CustomHeaders = g.CustomHeaders
		e.VpcOriginConfig = mergeVpcOriginConfig(e.VpcOriginConfig, g.VpcOriginConfig)
		e.CustomOriginConfig = mergeCustomOriginConfig(e.CustomOriginConfig, g.CustomOriginConfig)
		e.S3OriginConfig = mergeS3OriginConfig(e.S3OriginConfig, g.S3OriginConfig)
		items = append(items, e)
	}
	desired.Origins = &cftypes.Origins{
		Quantity: aws.Int32(int32(len(items))),
		Items:    items,
	}

	return desired
}

func mergeVpcOriginConfig(e, g *cftypes.VpcOriginConfig) *cftypes.VpcOriginConfig {
	if e == nil || g == nil {
		return g
	}
	m := *e
	m.VpcOriginId = g.VpcOriginId
	m.OriginKeepaliveTimeout = g.OriginKeepaliveTimeout
	m.OriginReadTimeout = g.OriginReadTimeout
	return &m
}

func mergeCustomOriginConfig(e, g *cftypes.CustomOriginConfig) *cftypes.CustomOriginConfig {
	if e == nil || g == nil {
		return g
	}
	m := *e // preserves IpAddressType and future unmanaged fields
	m.HTTPPort = g.HTTPPort
	m.HTTPSPort = g.HTTPSPort
	m.OriginProtocolPolicy = g.OriginProtocolPolicy
	m.OriginSslProtocols = g.OriginSslProtocols
	m.OriginKeepaliveTimeout = g.OriginKeepaliveTimeout
	m.OriginReadTimeout = g.OriginReadTimeout
	return &m
}

func mergeS3OriginConfig(e, g *cftypes.S3OriginConfig) *cftypes.S3OriginConfig {
	if e == nil || g == nil {
		return g
	}
	m := *e // preserves OriginReadTimeout (S3 read-backs carry one) and future fields
	m.OriginAccessIdentity = g.OriginAccessIdentity
	return &m
}

// generateConfig builds a complete DistributionConfig from the resource definition.
func (c CloudfrontDistribution) generateConfig(ctx context.Context) (*cftypes.DistributionConfig, error) {
	// Aliases and WebACLId are always populated (empty slice / empty string when unset) so
	// that removing them from the buildit config sends an explicit "clear" to AWS rather
	// than omitting the field (which AWS treats as "no change") and avoids a perpetual diff.
	aliasItems := c.Aliases
	if aliasItems == nil {
		aliasItems = []string{}
	}

	origins, err := c.buildOrigins(ctx)
	if err != nil {
		return nil, err
	}

	defaultBehavior, err := c.buildDefaultCacheBehavior(ctx)
	if err != nil {
		return nil, err
	}

	// The YAML supplies a global (CloudFront-scope) web ACL Name; the API takes the ARN.
	// The wrapper accepts ARNs directly (Tier-0); the prefix check here only avoids
	// constructing the WAFv2 client when no lookup is needed, mirroring how
	// buildFunctionAssociations and certificate resolution defer client construction.
	// Names resolve on every generateConfig call (no memoization).
	webACLArn := ""
	if name := aws.ToString(c.WebACLName); name != "" {
		if strings.HasPrefix(name, "arn:") {
			webACLArn = name
		} else {
			arn, err := awsw.NewWAFV2(ctx, c.Context.ProviderName).WebACLArnForIdentifier(ctx, name)
			if err != nil {
				return nil, errors.Wrapf(err, "error resolving web acl %q for cloudfront distribution %v", name, c.Identifier())
			}
			webACLArn = aws.ToString(arn)
			c.recordDisplayValue(webACLArn, name)
		}
	}

	cfg := &cftypes.DistributionConfig{
		CallerReference:      aws.String(c.Identifier()),
		Comment:              aws.String(aws.ToString(c.Comment)), // API requires a non-nil string; default to ""
		Enabled:              c.Enabled,
		HttpVersion:          cftypes.HttpVersion(aws.ToString(c.HTTPVersion)),
		IsIPV6Enabled:        c.IsIPV6Enabled,
		PriceClass:           cftypes.PriceClass(aws.ToString(c.PriceClass)),
		DefaultRootObject:    aws.String(aws.ToString(c.DefaultRootObject)),
		DefaultCacheBehavior: defaultBehavior,
		Origins:              origins,
		ViewerCertificate:    nil,
		WebACLId:             aws.String(webACLArn),
		Aliases: &cftypes.Aliases{
			Quantity: aws.Int32(int32(len(aliasItems))),
			Items:    aliasItems,
		},
	}

	// viewer certificate
	if c.Certificate != nil {
		// Resolve the certificate identifier: a full ARN is used as-is (skipping client
		// construction, like the web ACL block above); a certificate id (UUID) or domain
		// name is looked up in the distribution's provider account via ACM pinned to
		// us-east-1 — the only region CloudFront reads viewer certificates from.
		var certArn string
		if strings.HasPrefix(*c.Certificate, "arn:") {
			certArn = *c.Certificate
		} else {
			arn, err := awsw.NewACMGlobal(ctx, c.Context.ProviderName).CertificateArnForIdentifier(ctx, *c.Certificate)
			if err != nil {
				return nil, errors.Wrapf(err, "error resolving certificate %q for cloudfront distribution %v", *c.Certificate, c.Identifier())
			}
			certArn = aws.ToString(arn)
		}
		c.recordDisplayValue(certArn, *c.Certificate)
		cfg.ViewerCertificate = &cftypes.ViewerCertificate{
			CloudFrontDefaultCertificate: aws.Bool(false),
			ACMCertificateArn:            aws.String(certArn),
			MinimumProtocolVersion:       cftypes.MinimumProtocolVersion(aws.ToString(c.MinProtocolVersion)),
			SSLSupportMethod:             cftypes.SSLSupportMethod(aws.ToString(c.SSLSupportMethod)),
		}
	} else {
		// MinimumProtocolVersion/SSLSupportMethod mirror what AWS stores for default-cert
		// distributions: UpdateDistribution rejects the config without them
		// (InvalidViewerCertificate: You must specify MinimumProtocolVersion) and the
		// comparison snapshot ignores both fields when the default certificate is used.
		cfg.ViewerCertificate = &cftypes.ViewerCertificate{
			CloudFrontDefaultCertificate: aws.Bool(true),
			MinimumProtocolVersion:       cftypes.MinimumProtocolVersionTLSv1,
			SSLSupportMethod:             cftypes.SSLSupportMethodVip,
		}
	}

	// custom error responses — always populated (Quantity 0 when empty) so the config
	// is complete for UpdateDistribution, and so removing them sends an explicit clear.
	errItems := make([]cftypes.CustomErrorResponse, 0, len(c.CustomErrorResponses))
	for _, e := range c.CustomErrorResponses {
		errItems = append(errItems, cftypes.CustomErrorResponse{
			ErrorCode:          aws.Int32(e.ErrorCode),
			ResponseCode:       e.ResponseCode,
			ResponsePagePath:   e.ResponsePagePath,
			ErrorCachingMinTTL: e.ErrorCachingMinTTL,
		})
	}
	cfg.CustomErrorResponses = &cftypes.CustomErrorResponses{
		Quantity: aws.Int32(int32(len(errItems))),
		Items:    errItems,
	}

	// logging — always populated so removing the logging block sends an explicit "disable"
	// to AWS (matching the Aliases/WebACLId clear semantics) rather than a perpetual diff.
	if c.Logging != nil {
		cfg.Logging = &cftypes.LoggingConfig{
			Enabled:        c.Logging.Enabled,
			Bucket:         aws.String(c.Logging.Bucket),
			Prefix:         aws.String(aws.ToString(c.Logging.Prefix)),
			IncludeCookies: c.Logging.IncludeCookies,
		}
	} else {
		cfg.Logging = &cftypes.LoggingConfig{
			Enabled:        aws.Bool(false),
			Bucket:         aws.String(""),
			Prefix:         aws.String(""),
			IncludeCookies: aws.Bool(false),
		}
	}

	return cfg, nil
}

// buildDefaultCacheBehavior converts the default behavior to its SDK type.
func (c CloudfrontDistribution) buildDefaultCacheBehavior(ctx context.Context) (*cftypes.DefaultCacheBehavior, error) {
	b, err := c.resolveCacheBehaviorPolicies(ctx, c.DefaultCacheBehavior)
	if err != nil {
		return nil, err
	}
	fa, err := c.buildFunctionAssociations(ctx, b.FunctionAssociations)
	if err != nil {
		return nil, err
	}
	return &cftypes.DefaultCacheBehavior{
		TargetOriginId:          aws.String(b.TargetOriginId),
		ViewerProtocolPolicy:    cftypes.ViewerProtocolPolicy(aws.ToString(b.ViewerProtocolPolicy)),
		AllowedMethods:          buildAllowedMethods(b.AllowedMethods, b.CachedMethods),
		CachePolicyId:           b.CachePolicyId,
		OriginRequestPolicyId:   b.OriginRequestPolicyId,
		ResponseHeadersPolicyId: b.ResponseHeadersPolicyId,
		Compress:                b.Compress,
		FunctionAssociations:    fa,
	}, nil
}

// resolveCacheBehaviorPolicies returns a copy of b with any policy identifiers that are
// still names resolved to ids via the CloudFront API. Managed names were canonicalized
// to ids in Normalize (the sanctioned mutation phase), so only custom policy names reach
// the API (and the client is only constructed when one does). The caller's behavior and
// the user-supplied strings are never modified; resolution repeats on each
// generateConfig call by design, mirroring certificate and function-ARN resolution.
func (c CloudfrontDistribution) resolveCacheBehaviorPolicies(ctx context.Context, b CloudfrontCacheBehavior) (CloudfrontCacheBehavior, error) {
	if b.CachePolicyId != nil && !awsw.IsCloudfrontPolicyId(*b.CachePolicyId) {
		id, err := awsw.NewCloudfront(ctx, c.Context.ProviderName).CachePolicyIdForIdentifier(ctx, *b.CachePolicyId)
		if err != nil {
			return b, errors.Wrapf(err, "error resolving cache policy %q for cloudfront distribution %v", *b.CachePolicyId, c.Identifier())
		}
		c.recordDisplayValue(id, *b.CachePolicyId)
		b.CachePolicyId = aws.String(id)
	}
	if b.OriginRequestPolicyId != nil && !awsw.IsCloudfrontPolicyId(*b.OriginRequestPolicyId) {
		id, err := awsw.NewCloudfront(ctx, c.Context.ProviderName).OriginRequestPolicyIdForIdentifier(ctx, *b.OriginRequestPolicyId)
		if err != nil {
			return b, errors.Wrapf(err, "error resolving origin request policy %q for cloudfront distribution %v", *b.OriginRequestPolicyId, c.Identifier())
		}
		c.recordDisplayValue(id, *b.OriginRequestPolicyId)
		b.OriginRequestPolicyId = aws.String(id)
	}
	if b.ResponseHeadersPolicyId != nil && !awsw.IsCloudfrontPolicyId(*b.ResponseHeadersPolicyId) {
		id, err := awsw.NewCloudfront(ctx, c.Context.ProviderName).ResponseHeadersPolicyIdForIdentifier(ctx, *b.ResponseHeadersPolicyId)
		if err != nil {
			return b, errors.Wrapf(err, "error resolving response headers policy %q for cloudfront distribution %v", *b.ResponseHeadersPolicyId, c.Identifier())
		}
		c.recordDisplayValue(id, *b.ResponseHeadersPolicyId)
		b.ResponseHeadersPolicyId = aws.String(id)
	}
	return b, nil
}

// resolveOrigins infers each origin's domain name (and VPC origin id) from its
// (type, target) pair, making AWS calls as needed. Results are memoized on the origin
// so repeated generateConfig calls (Compare then Apply) resolve once.
//
// The target may carry a `provider::` prefix; it selects the provider whose regional
// clients resolve the underlying load balancer or bucket. The VPC origin itself is
// always looked up via the distribution's provider (CloudFront is global).
func (c CloudfrontDistribution) resolveOrigins(ctx context.Context) error {
	for i := range c.Origins {
		o := &c.Origins[i]
		if o.resolvedDomainName != "" {
			continue
		}

		target, targetProvider := awsw.ParseIdentifier(o.Target)
		if targetProvider == "" {
			targetProvider = c.Context.ProviderName
		}

		switch o.Type {
		case CloudfrontOriginTypeVpc:
			vo, err := awsw.NewCloudfront(ctx, c.Context.ProviderName).VpcOriginByIdentifier(ctx, target)
			if err != nil {
				return errors.Wrapf(err, "error resolving vpc origin %q for origin %q", o.Target, o.Name)
			}
			if vo.VpcOriginEndpointConfig == nil || aws.ToString(vo.VpcOriginEndpointConfig.Arn) == "" {
				return errors.Errorf("vpc origin %q has no endpoint configuration", o.Target)
			}
			endpointArn := aws.ToString(vo.VpcOriginEndpointConfig.Arn)
			// VPC origins can also point at EC2 instances; only load balancer endpoints
			// are supported because the domain name is taken from the LB's DNS name.
			if !strings.Contains(endpointArn, ":elasticloadbalancing:") {
				return errors.Errorf("vpc origin %q endpoint %q is not a load balancer; only ALB/NLB VPC origins are supported", o.Target, endpointArn)
			}
			lb, err := awsw.NewELB(ctx, targetProvider).FindLoadBalancerByArn(ctx, endpointArn)
			if err != nil {
				return errors.Wrapf(err, "error resolving load balancer for vpc origin %q (origin %q)", o.Target, o.Name)
			}
			o.resolvedDomainName = aws.ToString(lb.DNSName)
			o.resolvedVpcOriginId = aws.ToString(vo.Id)
			c.recordDisplayValue(o.resolvedDomainName, o.Target)
			c.recordDisplayValue(o.resolvedVpcOriginId, o.Target)

		case CloudfrontOriginTypeS3:
			// A full S3 endpoint is used as-is; otherwise the target is a bucket name and
			// the domain is its regional endpoint.
			if strings.HasSuffix(target, ".amazonaws.com") {
				o.resolvedDomainName = target
				continue
			}
			region, err := awsw.NewS3(ctx, targetProvider).BucketRegion(ctx, target)
			if err != nil {
				return errors.Wrapf(err, "error resolving s3 bucket %q for origin %q", o.Target, o.Name)
			}
			o.resolvedDomainName = fmt.Sprintf("%s.s3.%s.amazonaws.com", target, region)
			c.recordDisplayValue(o.resolvedDomainName, o.Target)

		case CloudfrontOriginTypeCustom:
			// A literal domain name (anything containing a dot) is used as-is; otherwise
			// the target is a load balancer name resolved to its DNS name. LB names cannot
			// contain dots, so the discriminator is unambiguous.
			if strings.Contains(target, ".") {
				o.resolvedDomainName = target
				continue
			}
			lb, err := awsw.NewELB(ctx, targetProvider).FindLoadBalancerForName(ctx, target)
			if err != nil {
				return errors.Wrapf(err, "error resolving load balancer %q for origin %q", o.Target, o.Name)
			}
			o.resolvedDomainName = aws.ToString(lb.DNSName)
			c.recordDisplayValue(o.resolvedDomainName, o.Target)

		default:
			return errors.Errorf("origin %q: unknown type %q", o.Name, o.Type)
		}
	}
	return nil
}

// buildOrigins converts the origins to their SDK type. Origin domain names (and VPC
// origin ids) are inferred from each origin's (type, target) pair via resolveOrigins,
// so AWS calls may be made.
func (c CloudfrontDistribution) buildOrigins(ctx context.Context) (*cftypes.Origins, error) {
	if err := c.resolveOrigins(ctx); err != nil {
		return nil, err
	}

	items := make([]cftypes.Origin, 0, len(c.Origins))
	for _, o := range c.Origins {
		origin := cftypes.Origin{
			Id:                        aws.String(o.Name),
			DomainName:                aws.String(o.resolvedDomainName),
			OriginPath:                aws.String(aws.ToString(o.Path)),
			ConnectionAttempts:        o.ConnectionAttempts,
			ConnectionTimeout:         o.ConnectionTimeout,
			ResponseCompletionTimeout: o.ResponseCompletionTimeout,
			OriginAccessControlId:     o.OriginAccessControlId,
			// explicitly disabled so the struct is complete for UpdateDistribution when
			// a new origin is added to an existing distribution (buildit does not manage
			// Origin Shield; existing origins keep their read-back value via the merge)
			OriginShield: &cftypes.OriginShield{Enabled: aws.Bool(false)},
		}
		// CustomHeaders is always populated (Quantity 0 when empty): UpdateDistribution
		// rejects the config with "The 'OriginCustomHeaders' field is missing" otherwise.
		hdrItems := make([]cftypes.OriginCustomHeader, 0, len(o.CustomHeaders))
		for _, k := range slices.Sorted(maps.Keys(o.CustomHeaders)) {
			hdrItems = append(hdrItems, cftypes.OriginCustomHeader{
				HeaderName:  aws.String(k),
				HeaderValue: aws.String(o.CustomHeaders[k]),
			})
		}
		origin.CustomHeaders = &cftypes.CustomHeaders{
			Quantity: aws.Int32(int32(len(hdrItems))),
			Items:    hdrItems,
		}
		switch o.Type {
		case CloudfrontOriginTypeVpc:
			origin.VpcOriginConfig = &cftypes.VpcOriginConfig{
				VpcOriginId:            aws.String(o.resolvedVpcOriginId),
				OriginKeepaliveTimeout: o.OriginKeepAliveTimeout,
				OriginReadTimeout:      o.OriginReadTimeout,
			}
		case CloudfrontOriginTypeCustom:
			ssl := make([]cftypes.SslProtocol, 0, len(o.OriginSSLProtocols))
			for _, p := range o.OriginSSLProtocols {
				ssl = append(ssl, cftypes.SslProtocol(p))
			}
			origin.CustomOriginConfig = &cftypes.CustomOriginConfig{
				HTTPPort:               o.HTTPPort,
				HTTPSPort:              o.HTTPSPort,
				OriginProtocolPolicy:   cftypes.OriginProtocolPolicy(aws.ToString(o.OriginProtocolPolicy)),
				OriginKeepaliveTimeout: o.OriginKeepAliveTimeout,
				OriginReadTimeout:      o.OriginReadTimeout,
				OriginSslProtocols: &cftypes.OriginSslProtocols{
					Quantity: aws.Int32(int32(len(ssl))),
					Items:    ssl,
				},
			}
		case CloudfrontOriginTypeS3:
			// Access is expected to be granted via an origin access control (OAC); the
			// legacy origin access identity is always sent empty.
			origin.S3OriginConfig = &cftypes.S3OriginConfig{
				OriginAccessIdentity: aws.String(""),
			}
		}
		items = append(items, origin)
	}
	return &cftypes.Origins{
		Quantity: aws.Int32(int32(len(items))),
		Items:    items,
	}, nil
}

// buildAllowedMethods builds the AllowedMethods SDK type; CachedMethods is omitted when empty.
func buildAllowedMethods(methods, cached []string) *cftypes.AllowedMethods {
	am := &cftypes.AllowedMethods{
		Quantity: aws.Int32(int32(len(methods))),
		Items:    toMethods(methods),
	}
	if len(cached) > 0 {
		am.CachedMethods = &cftypes.CachedMethods{
			Quantity: aws.Int32(int32(len(cached))),
			Items:    toMethods(cached),
		}
	}
	return am
}

// buildFunctionAssociations builds the FunctionAssociations SDK type (always non-nil).
// Each functionARN is resolved like the certificate/VPC-origin identifiers: an ARN is used
// as-is, otherwise it is treated as a CloudFront Function Name and resolved to its ARN. The
// AWS client is only constructed when a name actually needs resolving.
func (c CloudfrontDistribution) buildFunctionAssociations(ctx context.Context, fas []CloudfrontFunctionAssociation) (*cftypes.FunctionAssociations, error) {
	items := make([]cftypes.FunctionAssociation, 0, len(fas))
	for _, fa := range fas {
		fnArn := fa.FunctionARN
		if !strings.HasPrefix(fnArn, "arn:") {
			resolved, err := awsw.NewCloudfront(ctx, c.Context.ProviderName).FunctionArnForIdentifier(ctx, fnArn)
			if err != nil {
				return nil, errors.Wrapf(err, "error resolving cloudfront function %q", fnArn)
			}
			c.recordDisplayValue(resolved, fa.FunctionARN)
			fnArn = resolved
		}
		items = append(items, cftypes.FunctionAssociation{
			EventType:   cftypes.EventType(fa.EventType),
			FunctionARN: aws.String(fnArn),
		})
	}
	return &cftypes.FunctionAssociations{
		Quantity: aws.Int32(int32(len(items))),
		Items:    items,
	}, nil
}

func toMethods(in []string) []cftypes.Method {
	out := make([]cftypes.Method, 0, len(in))
	for _, m := range in {
		out = append(out, cftypes.Method(m))
	}
	return out
}

// compareDistributionConfig returns human-readable messages describing the managed-field
// differences between an existing distribution config and the desired one. An empty result
// means no managed change is required. Only fields buildit manages are compared, so drift
// in unmanaged fields does not trigger spurious updates. The display map (resolved value
// -> user-supplied form, nil-safe) makes messages read like the YAML config; both sides
// pass through the same map, so it cannot mask or invent a difference.
func compareDistributionConfig(existing, desired cftypes.DistributionConfig, display map[string]string) []string {
	var msgs []string

	e := newDistSnapshot(existing, display)
	d := newDistSnapshot(desired, display)

	if e.Comment != d.Comment {
		msgs = append(msgs, fmt.Sprintf("comment: %q -> %q", e.Comment, d.Comment))
	}
	if e.Enabled != d.Enabled {
		msgs = append(msgs, fmt.Sprintf("enabled: %v -> %v", e.Enabled, d.Enabled))
	}
	if e.DefaultRootObject != d.DefaultRootObject {
		msgs = append(msgs, fmt.Sprintf("defaultRootObject: %q -> %q", e.DefaultRootObject, d.DefaultRootObject))
	}
	if e.HTTPVersion != d.HTTPVersion {
		msgs = append(msgs, fmt.Sprintf("httpVersion: %q -> %q", e.HTTPVersion, d.HTTPVersion))
	}
	if e.IsIPV6Enabled != d.IsIPV6Enabled {
		msgs = append(msgs, fmt.Sprintf("isIPV6Enabled: %v -> %v", e.IsIPV6Enabled, d.IsIPV6Enabled))
	}
	if e.PriceClass != d.PriceClass {
		msgs = append(msgs, fmt.Sprintf("priceClass: %q -> %q", e.PriceClass, d.PriceClass))
	}
	if e.WebACLId != d.WebACLId {
		eDisp, dDisp := webACLDisplay(display, e.WebACLId), webACLDisplay(display, d.WebACLId)
		if eDisp == dDisp {
			// same name but different ACLs (e.g. deleted and recreated); only the raw
			// ARNs disambiguate the change
			eDisp, dDisp = e.WebACLId, d.WebACLId
		}
		msgs = append(msgs, fmt.Sprintf("webAclName: %q -> %q", eDisp, dDisp))
	}
	if !reflect.DeepEqual(e.Aliases, d.Aliases) {
		msgs = append(msgs, fmt.Sprintf("aliases: %v -> %v", e.Aliases, d.Aliases))
	}

	msgs = append(msgs, diffFields("viewerCertificate", e.Cert, d.Cert)...)
	msgs = append(msgs, diffFields("logging", e.Logging, d.Logging)...)
	msgs = append(msgs, diffFields("defaultCacheBehavior", e.DefaultBehavior, d.DefaultBehavior)...)
	msgs = append(msgs, diffOrigins(e.Origins, d.Origins)...)

	if !reflect.DeepEqual(e.Errors, d.Errors) {
		msgs = append(msgs, fmt.Sprintf("customErrorResponses: %+v -> %+v", e.Errors, d.Errors))
	}

	return msgs
}

// webACLDisplay renders a web ACL value for diff messages: the user-supplied name when
// the display map knows it, else the Name segment parsed from the wafv2 web ACL ARN
// (arn:aws:wafv2:us-east-1:{account}:global/webacl/{Name}/{Id}) so ACLs buildit never
// resolved (e.g. attached out-of-band) still render by name, else the raw value.
func webACLDisplay(display map[string]string, v string) string {
	if d, ok := display[v]; ok {
		return d
	}
	if strings.HasPrefix(v, "arn:") {
		if _, rest, found := strings.Cut(v, ":global/webacl/"); found {
			if parts := strings.Split(rest, "/"); len(parts) == 2 && parts[0] != "" {
				return parts[0]
			}
		}
	}
	return v
}

// diffOrigins returns per-origin, per-field difference messages between the existing and
// desired origin snapshots (both sorted by id).
func diffOrigins(existing, desired []originSnapshot) []string {
	var msgs []string

	em := make(map[string]originSnapshot, len(existing))
	for _, o := range existing {
		em[o.Id] = o
	}
	dm := make(map[string]originSnapshot, len(desired))
	for _, o := range desired {
		dm[o.Id] = o
	}

	idSet := make(map[string]struct{}, len(em)+len(dm))
	for id := range em {
		idSet[id] = struct{}{}
	}
	for id := range dm {
		idSet[id] = struct{}{}
	}
	ids := slices.Sorted(maps.Keys(idSet))

	for _, id := range ids {
		e, eok := em[id]
		d, dok := dm[id]
		label := fmt.Sprintf("origin %q", id)
		switch {
		case !dok:
			msgs = append(msgs, label+": removed")
		case !eok:
			msgs = append(msgs, fmt.Sprintf("%s: added (type %s, domain %s)", label, originSnapshotType(d), d.DomainName))
		default:
			if et, dt := originSnapshotType(e), originSnapshotType(d); et != dt {
				msgs = append(msgs, fmt.Sprintf("%s: type: %s -> %s", label, et, dt))
				continue
			}
			msgs = append(msgs, diffFields(label, e, d)...)
		}
	}
	return msgs
}

// originSnapshotType names the origin kind of a snapshot.
func originSnapshotType(o originSnapshot) string {
	switch {
	case o.Vpc != nil:
		return CloudfrontOriginTypeVpc
	case o.Custom != nil:
		return CloudfrontOriginTypeCustom
	case o.S3 != nil:
		return CloudfrontOriginTypeS3
	}
	return "unknown"
}

// diffFields compares two snapshot structs of the same type field by field (following
// pointers into nested snapshot structs) and returns a "<label>: <field>: <a> -> <b>"
// message per differing leaf field. Field names are rendered lower-camel-cased to match
// the YAML config keys.
func diffFields[T any](label string, e, d T) []string {
	return diffValues(label, reflect.ValueOf(e), reflect.ValueOf(d))
}

func diffValues(label string, e, d reflect.Value) []string {
	var msgs []string
	for i := 0; i < e.NumField(); i++ {
		f := e.Type().Field(i)
		fieldLabel := fmt.Sprintf("%s: %s", label, lowerCamel(f.Name))
		ev, dv := e.Field(i), d.Field(i)

		if f.Type.Kind() == reflect.Pointer {
			switch {
			case ev.IsNil() && dv.IsNil():
			case ev.IsNil():
				msgs = append(msgs, fmt.Sprintf("%s: added (%+v)", fieldLabel, dv.Elem().Interface()))
			case dv.IsNil():
				msgs = append(msgs, fieldLabel+": removed")
			default:
				msgs = append(msgs, diffValues(fieldLabel, ev.Elem(), dv.Elem())...)
			}
			continue
		}

		if !reflect.DeepEqual(ev.Interface(), dv.Interface()) {
			msgs = append(msgs, fmt.Sprintf("%s: %v -> %v", fieldLabel, ev.Interface(), dv.Interface()))
		}
	}
	return msgs
}

// lowerCamel converts an exported Go field name to its YAML-ish lower-camel form
// (Id -> id, DomainName -> domainName, HTTPPort -> httpPort, ACMArn -> acmArn).
func lowerCamel(s string) string {
	if s == "" {
		return s
	}
	// lowercase the leading run of capitals, keeping the last capital of the run
	// uppercase when it starts a new word (e.g. HTTPPort -> httpPort)
	i := 0
	for i < len(s) && s[i] >= 'A' && s[i] <= 'Z' {
		i++
	}
	if i == 0 {
		return s
	}
	if i == len(s) {
		return strings.ToLower(s)
	}
	if i > 1 {
		i--
	}
	return strings.ToLower(s[:i]) + s[i:]
}

// distSnapshot is a normalized, comparable view of the managed fields of a
// DistributionConfig. Slices are sorted (where order is not significant) so that
// reflect.DeepEqual compares by value rather than ordering.
type distSnapshot struct {
	Comment           string
	Enabled           bool
	DefaultRootObject string
	HTTPVersion       string
	IsIPV6Enabled     bool
	PriceClass        string
	WebACLId          string
	Aliases           []string
	Cert              certSnapshot
	Logging           loggingSnapshot
	DefaultBehavior   behaviorSnapshot
	Origins           []originSnapshot // sorted by id
	Errors            []errorSnapshot  // sorted by code
}

type certSnapshot struct {
	DefaultCert bool
	ACMArn      string
	MinProtocol string
	SSLMethod   string
}

type loggingSnapshot struct {
	Enabled        bool
	Bucket         string
	Prefix         string
	IncludeCookies bool
}

type behaviorSnapshot struct {
	TargetOriginId          string
	ViewerProtocolPolicy    string
	AllowedMethods          []string // sorted
	CachedMethods           []string // sorted
	CachePolicyId           string
	OriginRequestPolicyId   string
	ResponseHeadersPolicyId string
	Compress                bool
	Functions               []string // "eventType=arn", sorted
}

type originSnapshot struct {
	Id                        string
	DomainName                string
	OriginPath                string
	ConnectionAttempts        int32
	ConnectionTimeout         int32
	ResponseCompletionTimeout int32
	OriginAccessControlId     string
	CustomHeaders             []string // "name=value", sorted
	Vpc                       *vpcSnapshot
	Custom                    *customOriginSnapshot
	S3                        *s3OriginSnapshot
}

type vpcSnapshot struct {
	VpcOriginId      string
	KeepaliveTimeout int32
	ReadTimeout      int32
}

type customOriginSnapshot struct {
	HTTPPort         int32
	HTTPSPort        int32
	ProtocolPolicy   string
	SSLProtocols     []string // sorted
	KeepaliveTimeout int32
	ReadTimeout      int32
}

type s3OriginSnapshot struct {
	OriginAccessIdentity string
}

type errorSnapshot struct {
	ErrorCode          int32
	ResponseCode       string
	ResponsePagePath   string
	ErrorCachingMinTTL int64
}

// newDistSnapshot builds a comparable view of cfg. Resolved identifiers are rendered
// through the display map (resolved value -> user-supplied form; nil-safe) so diff
// messages show what the user wrote. Both compared snapshots use the same map and each
// user value resolves to a single AWS value per run, so the mapping preserves equality.
func newDistSnapshot(cfg cftypes.DistributionConfig, display map[string]string) distSnapshot {
	dv := func(v string) string {
		if d, ok := display[v]; ok {
			return d
		}
		return v
	}

	s := distSnapshot{
		Comment:           aws.ToString(cfg.Comment),
		Enabled:           aws.ToBool(cfg.Enabled),
		DefaultRootObject: aws.ToString(cfg.DefaultRootObject),
		HTTPVersion:       string(cfg.HttpVersion),
		IsIPV6Enabled:     aws.ToBool(cfg.IsIPV6Enabled),
		PriceClass:        string(cfg.PriceClass),
		// kept raw: web ACLs are compared by ARN and only rendered as names when the
		// diff message is formatted (see compareDistributionConfig), because two ACLs
		// can share a name (delete + recreate) and must still compare as different
		WebACLId: aws.ToString(cfg.WebACLId),
	}

	if cfg.Aliases != nil {
		s.Aliases = sortedCopy(cfg.Aliases.Items)
	}

	if vc := cfg.ViewerCertificate; vc != nil {
		s.Cert = certSnapshot{
			DefaultCert: aws.ToBool(vc.CloudFrontDefaultCertificate),
			ACMArn:      dv(aws.ToString(vc.ACMCertificateArn)),
			MinProtocol: string(vc.MinimumProtocolVersion),
			SSLMethod:   string(vc.SSLSupportMethod),
		}
		// When using the default CloudFront certificate, AWS populates MinimumProtocolVersion
		// and SSLSupportMethod (e.g. TLSv1/vip) on read-back even though buildit does not set
		// them. Ignore those fields in that case so they don't produce a perpetual diff.
		if s.Cert.DefaultCert {
			s.Cert.ACMArn = ""
			s.Cert.MinProtocol = ""
			s.Cert.SSLMethod = ""
		}
	}

	if lg := cfg.Logging; lg != nil {
		s.Logging = loggingSnapshot{
			Enabled:        aws.ToBool(lg.Enabled),
			Bucket:         aws.ToString(lg.Bucket),
			Prefix:         aws.ToString(lg.Prefix),
			IncludeCookies: aws.ToBool(lg.IncludeCookies),
		}
	}

	if db := cfg.DefaultCacheBehavior; db != nil {
		s.DefaultBehavior = behaviorSnapshot{
			TargetOriginId:          aws.ToString(db.TargetOriginId),
			ViewerProtocolPolicy:    string(db.ViewerProtocolPolicy),
			AllowedMethods:          methodsSnapshot(db.AllowedMethods),
			CachedMethods:           cachedMethodsSnapshot(db.AllowedMethods),
			CachePolicyId:           dv(aws.ToString(db.CachePolicyId)),
			OriginRequestPolicyId:   dv(aws.ToString(db.OriginRequestPolicyId)),
			ResponseHeadersPolicyId: dv(aws.ToString(db.ResponseHeadersPolicyId)),
			Compress:                aws.ToBool(db.Compress),
			Functions:               functionsSnapshot(db.FunctionAssociations, dv),
		}
	}

	if cfg.Origins != nil {
		for _, o := range cfg.Origins.Items {
			os := originSnapshot{
				Id:                        aws.ToString(o.Id),
				DomainName:                dv(aws.ToString(o.DomainName)),
				OriginPath:                aws.ToString(o.OriginPath),
				ConnectionAttempts:        aws.ToInt32(o.ConnectionAttempts),
				ConnectionTimeout:         aws.ToInt32(o.ConnectionTimeout),
				ResponseCompletionTimeout: aws.ToInt32(o.ResponseCompletionTimeout),
				OriginAccessControlId:     aws.ToString(o.OriginAccessControlId),
				CustomHeaders:             customHeadersSnapshot(o.CustomHeaders),
			}
			if o.VpcOriginConfig != nil {
				os.Vpc = &vpcSnapshot{
					VpcOriginId:      dv(aws.ToString(o.VpcOriginConfig.VpcOriginId)),
					KeepaliveTimeout: aws.ToInt32(o.VpcOriginConfig.OriginKeepaliveTimeout),
					ReadTimeout:      aws.ToInt32(o.VpcOriginConfig.OriginReadTimeout),
				}
			}
			if o.CustomOriginConfig != nil {
				co := o.CustomOriginConfig
				os.Custom = &customOriginSnapshot{
					HTTPPort:         aws.ToInt32(co.HTTPPort),
					HTTPSPort:        aws.ToInt32(co.HTTPSPort),
					ProtocolPolicy:   string(co.OriginProtocolPolicy),
					SSLProtocols:     sslProtocolsSnapshot(co.OriginSslProtocols),
					KeepaliveTimeout: aws.ToInt32(co.OriginKeepaliveTimeout),
					ReadTimeout:      aws.ToInt32(co.OriginReadTimeout),
				}
			}
			if o.S3OriginConfig != nil {
				os.S3 = &s3OriginSnapshot{
					OriginAccessIdentity: aws.ToString(o.S3OriginConfig.OriginAccessIdentity),
				}
			}
			s.Origins = append(s.Origins, os)
		}
		sort.Slice(s.Origins, func(i, j int) bool { return s.Origins[i].Id < s.Origins[j].Id })
	}

	if cfg.CustomErrorResponses != nil {
		for _, e := range cfg.CustomErrorResponses.Items {
			s.Errors = append(s.Errors, errorSnapshot{
				ErrorCode:          aws.ToInt32(e.ErrorCode),
				ResponseCode:       aws.ToString(e.ResponseCode),
				ResponsePagePath:   aws.ToString(e.ResponsePagePath),
				ErrorCachingMinTTL: aws.ToInt64(e.ErrorCachingMinTTL),
			})
		}
		sort.Slice(s.Errors, func(i, j int) bool { return s.Errors[i].ErrorCode < s.Errors[j].ErrorCode })
	}

	return s
}

// The snapshot slice helpers below normalize empty to nil: AWS read-backs represent
// "no items" as a non-nil container with Quantity 0 (e.g. CustomHeaders{Quantity: 0})
// while buildit's generated config leaves the field nil, and reflect.DeepEqual treats
// nil and empty slices as different — which would cause phantom diffs.

func methodsSnapshot(am *cftypes.AllowedMethods) []string {
	if am == nil {
		return nil
	}
	out := make([]string, 0, len(am.Items))
	for _, m := range am.Items {
		out = append(out, string(m))
	}
	sort.Strings(out)
	return nilIfEmpty(out)
}

func cachedMethodsSnapshot(am *cftypes.AllowedMethods) []string {
	if am == nil || am.CachedMethods == nil {
		return nil
	}
	out := make([]string, 0, len(am.CachedMethods.Items))
	for _, m := range am.CachedMethods.Items {
		out = append(out, string(m))
	}
	sort.Strings(out)
	return nilIfEmpty(out)
}

func functionsSnapshot(fa *cftypes.FunctionAssociations, dv func(string) string) []string {
	if fa == nil {
		return nil
	}
	out := make([]string, 0, len(fa.Items))
	for _, f := range fa.Items {
		out = append(out, fmt.Sprintf("%s=%s", string(f.EventType), dv(aws.ToString(f.FunctionARN))))
	}
	sort.Strings(out)
	return nilIfEmpty(out)
}

func customHeadersSnapshot(ch *cftypes.CustomHeaders) []string {
	if ch == nil {
		return nil
	}
	out := make([]string, 0, len(ch.Items))
	for _, h := range ch.Items {
		out = append(out, fmt.Sprintf("%s=%s", aws.ToString(h.HeaderName), aws.ToString(h.HeaderValue)))
	}
	sort.Strings(out)
	return nilIfEmpty(out)
}

func sslProtocolsSnapshot(sp *cftypes.OriginSslProtocols) []string {
	if sp == nil {
		return nil
	}
	out := make([]string, 0, len(sp.Items))
	for _, p := range sp.Items {
		out = append(out, string(p))
	}
	sort.Strings(out)
	return nilIfEmpty(out)
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

func nilIfEmpty(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	return in
}
