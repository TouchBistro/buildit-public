package awsw

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/TouchBistro/buildit/client"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	cftypes "github.com/aws/aws-sdk-go-v2/service/cloudfront/types"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

// Cloudfront is a thin wrapper around the AWS CloudFront SDK client that adds
// convenience helpers used by the cloudfront-distribution resource.
type Cloudfront struct {
	*cloudfront.Client
}

// NewCloudfront creates a new instance of the Cloudfront wrapper.
func NewCloudfront(ctx context.Context, providerName string) Cloudfront {
	return Cloudfront{client.Cloudfront(ctx, providerName)}
}

// ExistingDistribution holds an existing CloudFront distribution's identifying
// info and current configuration, used for comparison and updates.
type ExistingDistribution struct {
	Id     string
	ARN    string
	ETag   string
	Config cftypes.DistributionConfig
	Tags   map[string]string
}

// FindDistributionByCallerReference paginates distributions and returns the one whose
// CallerReference matches ref. CloudFront has no name-based lookup API; CallerReference
// is immutable and unique per distribution, so buildit uses it as the stable identifier.
// Returns (nil, nil) when no distribution matches.
func (c Cloudfront) FindDistributionByCallerReference(ctx context.Context, ref string) (*ExistingDistribution, error) {
	var marker *string
	for {
		out, err := c.ListDistributions(ctx, &cloudfront.ListDistributionsInput{Marker: marker})
		if err != nil {
			return nil, errors.Wrap(err, "error listing cloudfront distributions")
		}

		if out.DistributionList == nil {
			return nil, nil
		}

		for _, d := range out.DistributionList.Items {
			cfgOut, err := c.GetDistributionConfig(ctx, &cloudfront.GetDistributionConfigInput{Id: d.Id})
			if err != nil {
				return nil, errors.Wrapf(err, "error getting cloudfront distribution config %v", aws.ToString(d.Id))
			}

			if aws.ToString(cfgOut.DistributionConfig.CallerReference) != ref {
				continue
			}

			tags, err := c.DistributionTags(ctx, aws.ToString(d.ARN))
			if err != nil {
				return nil, err
			}

			return &ExistingDistribution{
				Id:     aws.ToString(d.Id),
				ARN:    aws.ToString(d.ARN),
				ETag:   aws.ToString(cfgOut.ETag),
				Config: *cfgOut.DistributionConfig,
				Tags:   tags,
			}, nil
		}

		if aws.ToBool(out.DistributionList.IsTruncated) {
			marker = out.DistributionList.NextMarker
		} else {
			return nil, nil
		}
	}
}

// VpcOriginIdForIdentifier resolves a VPC origin id from an identifier that is either the
// id itself (prefix "vo_") or the VPC origin Name. Names are resolved by paginating
// ListVpcOrigins. Returns an error if a name cannot be found.
func (c Cloudfront) VpcOriginIdForIdentifier(ctx context.Context, identifier string) (string, error) {
	if strings.HasPrefix(identifier, "vo_") {
		return identifier, nil
	}

	var marker *string
	for {
		out, err := c.ListVpcOrigins(ctx, &cloudfront.ListVpcOriginsInput{Marker: marker})
		if err != nil {
			return "", errors.Wrap(err, "error listing cloudfront vpc origins")
		}
		if out.VpcOriginList == nil {
			break
		}
		for _, v := range out.VpcOriginList.Items {
			if aws.ToString(v.Name) == identifier {
				return aws.ToString(v.Id), nil
			}
		}
		if aws.ToBool(out.VpcOriginList.IsTruncated) {
			marker = out.VpcOriginList.NextMarker
		} else {
			break
		}
	}

	return "", errors.Errorf("vpc origin not found for name %q", identifier)
}

// VpcOriginByIdentifier resolves a VPC origin from an identifier that is either the id
// itself (prefix "vo_") or the VPC origin Name, and returns the full VpcOrigin including
// its endpoint configuration (the ARN of the underlying load balancer).
func (c Cloudfront) VpcOriginByIdentifier(ctx context.Context, identifier string) (*cftypes.VpcOrigin, error) {
	id, err := c.VpcOriginIdForIdentifier(ctx, identifier)
	if err != nil {
		return nil, err
	}

	out, err := c.GetVpcOrigin(ctx, &cloudfront.GetVpcOriginInput{Id: aws.String(id)})
	if err != nil {
		return nil, errors.Wrapf(err, "error getting cloudfront vpc origin %q", id)
	}
	if out.VpcOrigin == nil {
		return nil, errors.Errorf("cloudfront vpc origin %q not found", id)
	}
	return out.VpcOrigin, nil
}

// FunctionArnForIdentifier resolves a CloudFront Function ARN from an identifier that is
// either the ARN itself ("arn:" prefix) or the function Name (resolved via DescribeFunction).
// The returned ARN is stage-independent. Returns an error if the name cannot be found.
func (c Cloudfront) FunctionArnForIdentifier(ctx context.Context, identifier string) (string, error) {
	if strings.HasPrefix(identifier, "arn:") {
		return identifier, nil
	}

	out, err := c.DescribeFunction(ctx, &cloudfront.DescribeFunctionInput{Name: aws.String(identifier)})
	if err != nil {
		return "", errors.Wrapf(err, "error describing cloudfront function %q", identifier)
	}
	if out.FunctionSummary == nil || out.FunctionSummary.FunctionMetadata == nil {
		return "", errors.Errorf("cloudfront function %q has no ARN", identifier)
	}
	return aws.ToString(out.FunctionSummary.FunctionMetadata.FunctionARN), nil
}

// cloudfrontPolicyIdRegexp matches CloudFront policy ids (UUIDs), distinguishing them
// from human-readable policy names in user-supplied identifiers.
var cloudfrontPolicyIdRegexp = regexp.MustCompile(`^[0-9a-fA-F]{8}(-[0-9a-fA-F]{4}){3}-[0-9a-fA-F]{12}$`)

// IsCloudfrontPolicyId reports whether the identifier is a CloudFront policy id (UUID)
// rather than a policy name.
func IsCloudfrontPolicyId(identifier string) bool {
	return cloudfrontPolicyIdRegexp.MatchString(identifier)
}

// matchesPolicyName reports whether a policy's name matches the identifier
// case-insensitively. AWS-managed policy names carry a "Managed-" prefix in API
// responses that their documentation omits, so the prefix is accepted on either side.
func matchesPolicyName(name, identifier string) bool {
	return strings.EqualFold(name, identifier) ||
		strings.EqualFold(name, "Managed-"+identifier) ||
		strings.EqualFold("Managed-"+name, identifier)
}

// resolvePolicyId returns the single id whose name matched, or an error when the name
// resolved to zero or multiple policies.
func resolvePolicyId(kind, identifier string, matches []string) (string, error) {
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return "", errors.Errorf("cloudfront %s policy not found for name %q", kind, identifier)
	default:
		return "", errors.Errorf("multiple cloudfront %s policies match name %q: %v", kind, identifier, matches)
	}
}

// CachePolicyIdForIdentifier resolves a cache policy id from an identifier that is either
// the id itself (a UUID) or a policy name (managed or custom), resolved by paginating
// ListCachePolicies with a case-insensitive name match.
func (c Cloudfront) CachePolicyIdForIdentifier(ctx context.Context, identifier string) (string, error) {
	if IsCloudfrontPolicyId(identifier) {
		return identifier, nil
	}

	var matches []string
	var marker *string
	for {
		out, err := c.ListCachePolicies(ctx, &cloudfront.ListCachePoliciesInput{Marker: marker})
		if err != nil {
			return "", errors.Wrap(err, "error listing cloudfront cache policies")
		}
		if out.CachePolicyList == nil {
			break
		}
		for _, p := range out.CachePolicyList.Items {
			if p.CachePolicy == nil || p.CachePolicy.CachePolicyConfig == nil {
				continue
			}
			if matchesPolicyName(aws.ToString(p.CachePolicy.CachePolicyConfig.Name), identifier) {
				matches = append(matches, aws.ToString(p.CachePolicy.Id))
			}
		}
		marker = out.CachePolicyList.NextMarker
		if aws.ToString(marker) == "" {
			break
		}
	}
	return resolvePolicyId("cache", identifier, matches)
}

// OriginRequestPolicyIdForIdentifier resolves an origin request policy id from an
// identifier that is either the id itself (a UUID) or a policy name (managed or custom),
// resolved by paginating ListOriginRequestPolicies with a case-insensitive name match.
func (c Cloudfront) OriginRequestPolicyIdForIdentifier(ctx context.Context, identifier string) (string, error) {
	if IsCloudfrontPolicyId(identifier) {
		return identifier, nil
	}

	var matches []string
	var marker *string
	for {
		out, err := c.ListOriginRequestPolicies(ctx, &cloudfront.ListOriginRequestPoliciesInput{Marker: marker})
		if err != nil {
			return "", errors.Wrap(err, "error listing cloudfront origin request policies")
		}
		if out.OriginRequestPolicyList == nil {
			break
		}
		for _, p := range out.OriginRequestPolicyList.Items {
			if p.OriginRequestPolicy == nil || p.OriginRequestPolicy.OriginRequestPolicyConfig == nil {
				continue
			}
			if matchesPolicyName(aws.ToString(p.OriginRequestPolicy.OriginRequestPolicyConfig.Name), identifier) {
				matches = append(matches, aws.ToString(p.OriginRequestPolicy.Id))
			}
		}
		marker = out.OriginRequestPolicyList.NextMarker
		if aws.ToString(marker) == "" {
			break
		}
	}
	return resolvePolicyId("origin request", identifier, matches)
}

// ResponseHeadersPolicyIdForIdentifier resolves a response headers policy id from an
// identifier that is either the id itself (a UUID) or a policy name (managed or custom),
// resolved by paginating ListResponseHeadersPolicies with a case-insensitive name match.
func (c Cloudfront) ResponseHeadersPolicyIdForIdentifier(ctx context.Context, identifier string) (string, error) {
	if IsCloudfrontPolicyId(identifier) {
		return identifier, nil
	}

	var matches []string
	var marker *string
	for {
		out, err := c.ListResponseHeadersPolicies(ctx, &cloudfront.ListResponseHeadersPoliciesInput{Marker: marker})
		if err != nil {
			return "", errors.Wrap(err, "error listing cloudfront response headers policies")
		}
		if out.ResponseHeadersPolicyList == nil {
			break
		}
		for _, p := range out.ResponseHeadersPolicyList.Items {
			if p.ResponseHeadersPolicy == nil || p.ResponseHeadersPolicy.ResponseHeadersPolicyConfig == nil {
				continue
			}
			if matchesPolicyName(aws.ToString(p.ResponseHeadersPolicy.ResponseHeadersPolicyConfig.Name), identifier) {
				matches = append(matches, aws.ToString(p.ResponseHeadersPolicy.Id))
			}
		}
		marker = out.ResponseHeadersPolicyList.NextMarker
		if aws.ToString(marker) == "" {
			break
		}
	}
	return resolvePolicyId("response headers", identifier, matches)
}

// CreateDistributionWithTags creates a new distribution with the supplied config and tags.
func (c Cloudfront) CreateDistributionWithTags(ctx context.Context, config *cftypes.DistributionConfig, tags map[string]string) (*cloudfront.CreateDistributionWithTagsOutput, error) {
	out, err := c.Client.CreateDistributionWithTags(ctx, &cloudfront.CreateDistributionWithTagsInput{
		DistributionConfigWithTags: &cftypes.DistributionConfigWithTags{
			DistributionConfig: config,
			Tags:               toTags(tags),
		},
	})
	if err != nil {
		return nil, errors.Wrap(err, "error creating cloudfront distribution")
	}
	return out, nil
}

// UpdateDistributionConfig updates an existing distribution's config using optimistic
// concurrency via the supplied ETag (IfMatch). Returns the new ETag.
func (c Cloudfront) UpdateDistributionConfig(ctx context.Context, id, etag string, config *cftypes.DistributionConfig) (string, error) {
	out, err := c.UpdateDistribution(ctx, &cloudfront.UpdateDistributionInput{
		Id:                 aws.String(id),
		IfMatch:            aws.String(etag),
		DistributionConfig: config,
	})
	if err != nil {
		return "", errors.Wrapf(err, "error updating cloudfront distribution %v", id)
	}
	return aws.ToString(out.ETag), nil
}

// DeleteDistribution deletes an existing distribution. The distribution must already be
// disabled and deployed; the caller is responsible for disabling it first.
func (c Cloudfront) DeleteDistribution(ctx context.Context, id, etag string) error {
	_, err := c.Client.DeleteDistribution(ctx, &cloudfront.DeleteDistributionInput{
		Id:      aws.String(id),
		IfMatch: aws.String(etag),
	})
	if err != nil {
		return errors.Wrapf(err, "error deleting cloudfront distribution %v", id)
	}
	return nil
}

// DistributionTags returns the tags for the distribution identified by arn.
func (c Cloudfront) DistributionTags(ctx context.Context, arn string) (map[string]string, error) {
	out, err := c.ListTagsForResource(ctx, &cloudfront.ListTagsForResourceInput{
		Resource: aws.String(arn),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "error listing tags for cloudfront distribution %v", arn)
	}

	tags := make(map[string]string)
	if out.Tags != nil {
		for _, t := range out.Tags.Items {
			tags[aws.ToString(t.Key)] = aws.ToString(t.Value)
		}
	}
	return tags, nil
}

// AddDistributionTags adds/updates the supplied tags on the distribution.
func (c Cloudfront) AddDistributionTags(ctx context.Context, arn string, tags map[string]string) error {
	if len(tags) == 0 {
		return nil
	}
	_, err := c.TagResource(ctx, &cloudfront.TagResourceInput{
		Resource: aws.String(arn),
		Tags:     toTags(tags),
	})
	if err != nil {
		return errors.Wrapf(err, "error tagging cloudfront distribution %v", arn)
	}
	return nil
}

// DeleteDistributionTags removes the supplied tag keys from the distribution.
func (c Cloudfront) DeleteDistributionTags(ctx context.Context, arn string, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	_, err := c.UntagResource(ctx, &cloudfront.UntagResourceInput{
		Resource: aws.String(arn),
		TagKeys:  &cftypes.TagKeys{Items: keys},
	})
	if err != nil {
		return errors.Wrapf(err, "error untagging cloudfront distribution %v", arn)
	}
	return nil
}

// WaitForDeployed blocks until the distribution's status is no longer "InProgress",
// or until the timeout elapses. CloudFront propagation routinely takes 15-45 minutes
// (especially on first create), so the timeout is set to 45 minutes. The context is
// honored, so callers can cancel earlier.
func (c Cloudfront) WaitForDeployed(ctx context.Context, id string) error {
	waiter := cloudfront.NewDistributionDeployedWaiter(c.Client, func(o *cloudfront.DistributionDeployedWaiterOptions) {
		o.MinDelay = 5 * time.Second
		o.MaxDelay = 30 * time.Second
	})
	return waiter.Wait(ctx, &cloudfront.GetDistributionInput{Id: aws.String(id)}, 45*time.Minute)
}

// VpcOriginStatusDeployed is the VPC origin status once propagation completes.
// (The API exposes status as a plain string; the SDK defines no enum for it.)
const VpcOriginStatusDeployed = "Deployed"

// ExistingVpcOrigin holds an existing CloudFront VPC origin's identifying info and
// current endpoint configuration, used for comparison and updates.
type ExistingVpcOrigin struct {
	Id     string
	ARN    string
	ETag   string
	Status string
	Config cftypes.VpcOriginEndpointConfig
	Tags   map[string]string
}

// FindVpcOriginByName paginates VPC origins and returns the one whose Name matches,
// including its ETag, endpoint configuration, and tags. Returns (nil, nil) when no
// VPC origin matches (unlike VpcOriginIdForIdentifier, which errors — lifecycle
// callers need "absent" as a non-error state).
func (c Cloudfront) FindVpcOriginByName(ctx context.Context, name string) (*ExistingVpcOrigin, error) {
	var marker *string
	for {
		out, err := c.ListVpcOrigins(ctx, &cloudfront.ListVpcOriginsInput{Marker: marker})
		if err != nil {
			return nil, errors.Wrap(err, "error listing cloudfront vpc origins")
		}
		if out.VpcOriginList == nil {
			return nil, nil
		}

		for _, v := range out.VpcOriginList.Items {
			if aws.ToString(v.Name) != name {
				continue
			}

			id := aws.ToString(v.Id)
			getOut, err := c.GetVpcOrigin(ctx, &cloudfront.GetVpcOriginInput{Id: aws.String(id)})
			if err != nil {
				return nil, errors.Wrapf(err, "error getting cloudfront vpc origin %q", id)
			}
			if getOut.VpcOrigin == nil || getOut.VpcOrigin.VpcOriginEndpointConfig == nil {
				return nil, errors.Errorf("cloudfront vpc origin %q has no endpoint configuration", id)
			}

			arn := aws.ToString(getOut.VpcOrigin.Arn)
			tags, err := c.VpcOriginTags(ctx, arn)
			if err != nil {
				return nil, err
			}

			return &ExistingVpcOrigin{
				Id:     id,
				ARN:    arn,
				ETag:   aws.ToString(getOut.ETag),
				Status: aws.ToString(getOut.VpcOrigin.Status),
				Config: *getOut.VpcOrigin.VpcOriginEndpointConfig,
				Tags:   tags,
			}, nil
		}

		if aws.ToBool(out.VpcOriginList.IsTruncated) {
			marker = out.VpcOriginList.NextMarker
			// defensive: a truncated response with no marker would otherwise loop forever
			if aws.ToString(marker) == "" {
				return nil, nil
			}
		} else {
			return nil, nil
		}
	}
}

// CreateVpcOrigin creates a new VPC origin with the supplied endpoint config and tags.
func (c Cloudfront) CreateVpcOrigin(ctx context.Context, config *cftypes.VpcOriginEndpointConfig, tags map[string]string) (*cloudfront.CreateVpcOriginOutput, error) {
	out, err := c.Client.CreateVpcOrigin(ctx, &cloudfront.CreateVpcOriginInput{
		VpcOriginEndpointConfig: config,
		Tags:                    toTags(tags),
	})
	if err != nil {
		return nil, errors.Wrap(err, "error creating cloudfront vpc origin")
	}
	return out, nil
}

// UpdateVpcOriginConfig updates an existing VPC origin's endpoint config using
// optimistic concurrency via the supplied ETag (IfMatch). Returns the new ETag.
func (c Cloudfront) UpdateVpcOriginConfig(ctx context.Context, id, etag string, config *cftypes.VpcOriginEndpointConfig) (string, error) {
	out, err := c.UpdateVpcOrigin(ctx, &cloudfront.UpdateVpcOriginInput{
		Id:                      aws.String(id),
		IfMatch:                 aws.String(etag),
		VpcOriginEndpointConfig: config,
	})
	if err != nil {
		return "", errors.Wrapf(err, "error updating cloudfront vpc origin %v", id)
	}
	return aws.ToString(out.ETag), nil
}

// DeleteVpcOrigin deletes an existing VPC origin. The VPC origin must be in the
// Deployed state and not referenced by any distribution.
func (c Cloudfront) DeleteVpcOrigin(ctx context.Context, id, etag string) error {
	_, err := c.Client.DeleteVpcOrigin(ctx, &cloudfront.DeleteVpcOriginInput{
		Id:      aws.String(id),
		IfMatch: aws.String(etag),
	})
	if err != nil {
		return errors.Wrapf(err, "error deleting cloudfront vpc origin %v", id)
	}
	return nil
}

// VpcOriginTags returns the tags for the VPC origin identified by arn.
func (c Cloudfront) VpcOriginTags(ctx context.Context, arn string) (map[string]string, error) {
	out, err := c.ListTagsForResource(ctx, &cloudfront.ListTagsForResourceInput{
		Resource: aws.String(arn),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "error listing tags for cloudfront vpc origin %v", arn)
	}

	tags := make(map[string]string)
	if out.Tags != nil {
		for _, t := range out.Tags.Items {
			tags[aws.ToString(t.Key)] = aws.ToString(t.Value)
		}
	}
	return tags, nil
}

// AddVpcOriginTags adds/updates the supplied tags on the VPC origin.
func (c Cloudfront) AddVpcOriginTags(ctx context.Context, arn string, tags map[string]string) error {
	if len(tags) == 0 {
		return nil
	}
	_, err := c.TagResource(ctx, &cloudfront.TagResourceInput{
		Resource: aws.String(arn),
		Tags:     toTags(tags),
	})
	if err != nil {
		return errors.Wrapf(err, "error tagging cloudfront vpc origin %v", arn)
	}
	return nil
}

// DeleteVpcOriginTags removes the supplied tag keys from the VPC origin.
func (c Cloudfront) DeleteVpcOriginTags(ctx context.Context, arn string, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	_, err := c.UntagResource(ctx, &cloudfront.UntagResourceInput{
		Resource: aws.String(arn),
		TagKeys:  &cftypes.TagKeys{Items: keys},
	})
	if err != nil {
		return errors.Wrapf(err, "error untagging cloudfront vpc origin %v", arn)
	}
	return nil
}

// ExistingFunction holds an existing CloudFront function's identifying info, current
// configuration, code (all read from the DEVELOPMENT stage), and tags, used for
// comparison and updates.
type ExistingFunction struct {
	ARN    string
	ETag   string
	Status string
	Config cftypes.FunctionConfig
	Code   []byte
	Tags   map[string]string
}

// FindFunctionByName returns the function with the given name, reading its config and
// code from the DEVELOPMENT stage (the stage UpdateFunction writes to). Returns
// (nil, nil) when the function does not exist.
func (c Cloudfront) FindFunctionByName(ctx context.Context, name string) (*ExistingFunction, error) {
	desc, err := c.DescribeFunction(ctx, &cloudfront.DescribeFunctionInput{
		Name:  aws.String(name),
		Stage: cftypes.FunctionStageDevelopment,
	})
	if err != nil {
		var nsfe *cftypes.NoSuchFunctionExists
		if errors.As(err, &nsfe) {
			return nil, nil
		}
		return nil, errors.Wrapf(err, "error describing cloudfront function %q", name)
	}

	if desc.FunctionSummary == nil || desc.FunctionSummary.FunctionMetadata == nil || desc.FunctionSummary.FunctionConfig == nil {
		return nil, errors.Errorf("cloudfront function %q has an incomplete response", name)
	}

	code, err := c.GetFunction(ctx, &cloudfront.GetFunctionInput{
		Name:  aws.String(name),
		Stage: cftypes.FunctionStageDevelopment,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "error getting cloudfront function code %q", name)
	}

	arn := aws.ToString(desc.FunctionSummary.FunctionMetadata.FunctionARN)

	tags, err := c.FunctionTags(ctx, arn)
	if err != nil {
		return nil, err
	}

	return &ExistingFunction{
		ARN:    arn,
		ETag:   aws.ToString(desc.ETag),
		Status: aws.ToString(desc.FunctionSummary.Status),
		Config: *desc.FunctionSummary.FunctionConfig,
		Code:   code.FunctionCode,
		Tags:   tags,
	}, nil
}

// FunctionTags returns the tags for the function identified by arn.
func (c Cloudfront) FunctionTags(ctx context.Context, arn string) (map[string]string, error) {
	out, err := c.ListTagsForResource(ctx, &cloudfront.ListTagsForResourceInput{
		Resource: aws.String(arn),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "error listing tags for cloudfront function %v", arn)
	}

	tags := make(map[string]string)
	if out.Tags != nil {
		for _, t := range out.Tags.Items {
			tags[aws.ToString(t.Key)] = aws.ToString(t.Value)
		}
	}
	return tags, nil
}

// AddFunctionTags adds/updates the supplied tags on the function.
func (c Cloudfront) AddFunctionTags(ctx context.Context, arn string, tags map[string]string) error {
	if len(tags) == 0 {
		return nil
	}
	_, err := c.TagResource(ctx, &cloudfront.TagResourceInput{
		Resource: aws.String(arn),
		Tags:     toTags(tags),
	})
	if err != nil {
		return errors.Wrapf(err, "error tagging cloudfront function %v", arn)
	}
	return nil
}

// DeleteFunctionTags removes the supplied tag keys from the function.
func (c Cloudfront) DeleteFunctionTags(ctx context.Context, arn string, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	_, err := c.UntagResource(ctx, &cloudfront.UntagResourceInput{
		Resource: aws.String(arn),
		TagKeys:  &cftypes.TagKeys{Items: keys},
	})
	if err != nil {
		return errors.Wrapf(err, "error untagging cloudfront function %v", arn)
	}
	return nil
}

// LiveFunctionCode returns the function's code at the LIVE (published) stage, or
// (nil, nil) if the function has never been published.
func (c Cloudfront) LiveFunctionCode(ctx context.Context, name string) ([]byte, error) {
	out, err := c.GetFunction(ctx, &cloudfront.GetFunctionInput{
		Name:  aws.String(name),
		Stage: cftypes.FunctionStageLive,
	})
	if err != nil {
		var nsfe *cftypes.NoSuchFunctionExists
		if errors.As(err, &nsfe) {
			return nil, nil
		}
		return nil, errors.Wrapf(err, "error getting live cloudfront function code %q", name)
	}
	return out.FunctionCode, nil
}

// CreateFunction creates a new function in the DEVELOPMENT stage and returns its ETag,
// which the caller needs to publish it. Tags are applied atomically at creation.
func (c Cloudfront) CreateFunction(ctx context.Context, name string, config *cftypes.FunctionConfig, code []byte, tags map[string]string) (string, error) {
	input := &cloudfront.CreateFunctionInput{
		Name:           aws.String(name),
		FunctionConfig: config,
		FunctionCode:   code,
	}
	if len(tags) > 0 {
		input.Tags = toTags(tags)
	}
	out, err := c.Client.CreateFunction(ctx, input)
	if err != nil {
		return "", errors.Wrapf(err, "error creating cloudfront function %q", name)
	}
	return aws.ToString(out.ETag), nil
}

// UpdateFunctionCode updates the function's config and code in the DEVELOPMENT stage
// using optimistic concurrency via the supplied ETag (IfMatch). Returns the new ETag.
func (c Cloudfront) UpdateFunctionCode(ctx context.Context, name, etag string, config *cftypes.FunctionConfig, code []byte) (string, error) {
	out, err := c.UpdateFunction(ctx, &cloudfront.UpdateFunctionInput{
		Name:           aws.String(name),
		IfMatch:        aws.String(etag),
		FunctionConfig: config,
		FunctionCode:   code,
	})
	if err != nil {
		return "", errors.Wrapf(err, "error updating cloudfront function %q", name)
	}
	return aws.ToString(out.ETag), nil
}

// PublishFunctionVersion publishes the function's DEVELOPMENT stage to LIVE. The etag
// must be the current DEVELOPMENT-stage ETag (from create, update, or describe).
func (c Cloudfront) PublishFunctionVersion(ctx context.Context, name, etag string) error {
	_, err := c.PublishFunction(ctx, &cloudfront.PublishFunctionInput{
		Name:    aws.String(name),
		IfMatch: aws.String(etag),
	})
	if err != nil {
		return errors.Wrapf(err, "error publishing cloudfront function %q", name)
	}
	return nil
}

// DeleteFunctionByETag deletes the function. Fails if the function is still associated
// with a distribution.
func (c Cloudfront) DeleteFunctionByETag(ctx context.Context, name, etag string) error {
	_, err := c.DeleteFunction(ctx, &cloudfront.DeleteFunctionInput{
		Name:    aws.String(name),
		IfMatch: aws.String(etag),
	})
	if err != nil {
		return errors.Wrapf(err, "error deleting cloudfront function %q (functions still associated with a distribution cannot be deleted)", name)
	}
	return nil
}

// WaitForFunctionDeployed polls the function's status until it leaves IN_PROGRESS
// (function deploys settle in seconds, unlike distributions), or until the timeout
// elapses. There is no SDK waiter for functions.
func (c Cloudfront) WaitForFunctionDeployed(ctx context.Context, name string) error {
	deadline := time.After(5 * time.Minute)
	for {
		out, err := c.DescribeFunction(ctx, &cloudfront.DescribeFunctionInput{
			Name:  aws.String(name),
			Stage: cftypes.FunctionStageDevelopment,
		})
		if err != nil {
			return errors.Wrapf(err, "error describing cloudfront function %q while waiting for deploy", name)
		}
		if out.FunctionSummary == nil {
			return errors.Errorf("cloudfront function %q returned no summary while waiting for deploy", name)
		}

		status := strings.ToUpper(aws.ToString(out.FunctionSummary.Status))
		if status != "IN_PROGRESS" {
			return nil
		}

		select {
		case <-ctx.Done():
			return errors.Wrapf(ctx.Err(), "context cancelled waiting for cloudfront function %q to deploy", name)
		case <-deadline:
			return errors.Errorf("timed out waiting for cloudfront function %q to deploy", name)
		case <-time.After(5 * time.Second):
		}
	}
}

// WaitForVpcOriginDeployed polls until the VPC origin's status is "Deployed", or until
// the timeout elapses. The SDK provides no waiter for VPC origins, so this polls
// GetVpcOrigin directly. Deploys typically complete within ~15 minutes; the timeout is
// 20 minutes. The context is honored, so callers can cancel earlier.
func (c Cloudfront) WaitForVpcOriginDeployed(ctx context.Context, id string) error {
	const (
		pollInterval = 15 * time.Second
		timeout      = 20 * time.Minute
	)

	deadline := time.Now().Add(timeout)
	status := ""
	for {
		if err := ctx.Err(); err != nil {
			return errors.Wrapf(err, "cancelled waiting for cloudfront vpc origin %q to deploy", id)
		}
		if time.Now().After(deadline) {
			return errors.Errorf("timed out waiting for cloudfront vpc origin %q to deploy (status %q)", id, status)
		}

		out, err := c.GetVpcOrigin(ctx, &cloudfront.GetVpcOriginInput{Id: aws.String(id)})
		if err != nil {
			return errors.Wrapf(err, "error getting cloudfront vpc origin %q while waiting for deploy", id)
		}

		status = ""
		if out.VpcOrigin != nil {
			status = aws.ToString(out.VpcOrigin.Status)
		}
		if status == VpcOriginStatusDeployed {
			return nil
		}
		log.WithFields(log.Fields{"Id": id, "Status": status}).Debug("waiting for cloudfront vpc origin to deploy")

		timer := time.NewTimer(pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return errors.Wrapf(ctx.Err(), "cancelled waiting for cloudfront vpc origin %q to deploy", id)
		case <-timer.C:
		}
	}
}

// toTags converts a string map to the CloudFront Tags type.
func toTags(tags map[string]string) *cftypes.Tags {
	items := make([]cftypes.Tag, 0, len(tags))
	for k, v := range tags {
		items = append(items, cftypes.Tag{Key: aws.String(k), Value: aws.String(v)})
	}
	return &cftypes.Tags{Items: items}
}
