package resource

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/TouchBistro/buildit/util"
	"github.com/TouchBistro/goutils/color"
	"github.com/aws/aws-sdk-go-v2/aws"
	cftypes "github.com/aws/aws-sdk-go-v2/service/cloudfront/types"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

// CloudfrontVpcOrigin is a declarative CloudFront VPC origin resource. A VPC origin
// lets a distribution route traffic to a load balancer that is private to a VPC,
// without exposing it to the internet. Distributions reference it by name or id via
// their `origins` entries of type `vpc`.
type CloudfrontVpcOrigin struct {
	BaseResource `yaml:",inline"`
	// Name comes from the resource key; VPC origins have no name-based lookup API, so
	// buildit finds existing ones by listing and matching on Name.
	Name string `yaml:"-"`
	// Target is the ARN or name of the ALB/NLB the VPC origin points at. It may carry
	// a `provider::` prefix to resolve the load balancer via a different provider.
	Target               string            `yaml:"target"`
	HTTPPort             *int32            `yaml:"httpPort"`
	HTTPSPort            *int32            `yaml:"httpsPort"`
	OriginProtocolPolicy *string           `yaml:"originProtocolPolicy"`
	// OriginSSLProtocol is singular: the SDK models protocols as a list, but AWS
	// accepts exactly one for VPC origins (the console offers a single choice; the
	// API rejects multiple entries).
	OriginSSLProtocol *string `yaml:"originSslProtocol"`
	Tags                 map[string]string `yaml:"tags"`
	GlobalTags           map[string]string `yaml:"-"`
	DependsOn            []Key             `yaml:"dependsOn"`
}

// Key returns the unique key for the resource for this buildit context.
func (c CloudfrontVpcOrigin) Key() Key {
	return NewKey(c.Context.ProviderName, c.Identifier())
}

// Identifier returns the VPC origin name.
func (c CloudfrontVpcOrigin) Identifier() string {
	return c.Name
}

// Normalize sets default values mirroring the cloudfront-distribution custom-origin
// defaults (ports 80/443, https-only, TLSv1.2).
func (c *CloudfrontVpcOrigin) Normalize(ctx context.Context) {
	if c.HTTPPort == nil {
		c.HTTPPort = aws.Int32(80)
	}
	if c.HTTPSPort == nil {
		c.HTTPSPort = aws.Int32(443)
	}
	// Enum values are case-insensitive in YAML; canonicalize before defaulting so
	// validation and the AWS API always see the exact CloudFront constant. Unknown
	// values pass through verbatim for Validate to report.
	normalizeCloudfrontEnum(c.OriginProtocolPolicy, CloudfrontOriginProtocolHTTPOnly, CloudfrontOriginProtocolHTTPSOnly, CloudfrontOriginProtocolMatchViewer)
	normalizeCloudfrontEnum(c.OriginSSLProtocol, cloudfrontSSLProtocols...)
	if c.OriginProtocolPolicy == nil {
		c.OriginProtocolPolicy = aws.String(CloudfrontOriginProtocolHTTPSOnly)
	}
	if c.OriginSSLProtocol == nil {
		c.OriginSSLProtocol = aws.String(string(cftypes.SslProtocolTLSv12))
	}

	if c.Tags == nil {
		c.Tags = make(map[string]string)
	}
	ResourceTags(c.Tags).Merge(c.GlobalTags)
}

// Validate checks the VPC origin configuration. It performs structural validation
// only; the load balancer lookup happens at apply time.
func (c CloudfrontVpcOrigin) Validate(ctx context.Context) error {
	var msgs []string

	if c.Target == "" {
		msgs = append(msgs, "target (load balancer ARN or name) is required")
	}

	switch aws.ToString(c.OriginProtocolPolicy) {
	case CloudfrontOriginProtocolHTTPOnly, CloudfrontOriginProtocolHTTPSOnly, CloudfrontOriginProtocolMatchViewer:
	default:
		msgs = append(msgs, fmt.Sprintf("invalid originProtocolPolicy %q", aws.ToString(c.OriginProtocolPolicy)))
	}

	if p := aws.ToString(c.OriginSSLProtocol); p != "" {
		if !slices.Contains(cloudfrontSSLProtocols, p) {
			msgs = append(msgs, fmt.Sprintf("invalid originSslProtocol %q", p))
		}
	}

	if len(msgs) == 0 {
		return nil
	}
	return &ValidationError{
		ResourceIdentifier: c.Identifier(),
		ResourceType:       "CloudFront VPC Origin",
		Messages:           msgs,
	}
}

// Apply creates or updates the VPC origin.
func (c CloudfrontVpcOrigin) Apply(ctx context.Context) error {
	log.Debugf("applying cloudfront vpc origin %v", c.Identifier())

	diffs, err := c.Compare(ctx)
	if err != nil {
		return err
	}

	if diffs == nil {
		log.WithFields(log.Fields{"Name": c.Identifier()}).Info("no updates required")
		return nil
	}

	if diffs.AWSResource() != nil {
		for _, d := range diffs.Differences() {
			log.WithField("Name", c.Identifier()).Info(d)
		}
		return c.applyDiffs(ctx, diffs)
	}

	return c.apply(ctx)
}

// apply creates a new VPC origin and waits for it to deploy.
func (c CloudfrontVpcOrigin) apply(ctx context.Context) error {
	cfg, err := c.generateEndpointConfig(ctx)
	if err != nil {
		return err
	}

	cfClient := awsw.NewCloudfront(ctx, c.Context.ProviderName)
	out, err := cfClient.CreateVpcOrigin(ctx, cfg, c.Tags)
	if err != nil {
		return err
	}
	if out == nil || out.VpcOrigin == nil {
		return errors.Errorf("cloudfront vpc origin %v: create returned no vpc origin", c.Identifier())
	}

	// The deploy wait must stay inline (not WaitableResource.Wait): AWS rejects
	// associating a VPC origin with a distribution while a "crud operation is in
	// progress", so dependents need the Deployed state, not just existence.
	id := aws.ToString(out.VpcOrigin.Id)
	log.WithFields(log.Fields{"Name": c.Identifier(), "Id": id}).Info("waiting for cloudfront vpc origin to deploy")
	if err := cfClient.WaitForVpcOriginDeployed(ctx, id); err != nil {
		return errors.Wrapf(err, "cloudfront vpc origin %v failed to deploy", c.Identifier())
	}

	log.WithFields(log.Fields{"Name": c.Identifier(), "Id": id}).Info(color.Green("cloudfront vpc origin created"))
	return nil
}

// applyDiffs updates an existing VPC origin and reconciles its tags.
func (c CloudfrontVpcOrigin) applyDiffs(ctx context.Context, diffs ResourceDiff) error {
	voDiff, ok := diffs.(*CloudfrontVpcOriginDiff)
	if !ok {
		return errors.New("invalid diff type supplied")
	}

	cfClient := awsw.NewCloudfront(ctx, c.Context.ProviderName)

	if voDiff.configChanged {
		if _, err := cfClient.UpdateVpcOriginConfig(ctx, voDiff.id, voDiff.etag, voDiff.desired); err != nil {
			return err
		}
		// Inline for the same reason as create: updates put the origin back into
		// Deploying, and a dependent distribution cannot associate it in that state.
		log.WithFields(log.Fields{"Name": c.Identifier(), "Id": voDiff.id}).Info("waiting for cloudfront vpc origin to deploy")
		if err := cfClient.WaitForVpcOriginDeployed(ctx, voDiff.id); err != nil {
			return errors.Wrapf(err, "cloudfront vpc origin %v failed to deploy", c.Identifier())
		}
	}

	if voDiff.tagsDiff {
		if upserts := voDiff.tagDiff.Upserts(); len(upserts) > 0 {
			if err := cfClient.AddVpcOriginTags(ctx, voDiff.arn, upserts); err != nil {
				return err
			}
		}
		if keys := voDiff.tagDiff.DeletedKeys(); len(keys) > 0 {
			if err := cfClient.DeleteVpcOriginTags(ctx, voDiff.arn, keys); err != nil {
				return err
			}
		}
	}

	log.WithFields(log.Fields{"Name": c.Identifier(), "Id": voDiff.id}).Info(color.Yellow("cloudfront vpc origin updated"))
	return nil
}

// NOTE: CloudfrontVpcOrigin deliberately does NOT implement WaitableResource. Deferred
// waits run after all Applies (config/dag.go), but AWS rejects associating a VPC origin
// with a distribution while it is still Deploying ("A crud operation is in progress...
// It cannot be associated with the distribution"), so dependents need the inline wait
// in apply/applyDiffs to observe the Deployed state.

// Destroy deletes the VPC origin. No-op if it does not exist. AWS refuses to delete a
// VPC origin that a distribution still references, so distributions must be destroyed
// (or updated) first.
func (c CloudfrontVpcOrigin) Destroy(ctx context.Context) error {
	log.Debugf("destroying cloudfront vpc origin %v", c.Identifier())

	cfClient := awsw.NewCloudfront(ctx, c.Context.ProviderName)
	existing, err := cfClient.FindVpcOriginByName(ctx, c.Identifier())
	if err != nil {
		return err
	}
	if existing == nil {
		log.WithFields(log.Fields{"Name": c.Identifier()}).Info("cloudfront vpc origin does not exist, nothing to destroy")
		return nil
	}

	// Deletes are rejected while a deploy is in progress; wait it out and re-fetch so
	// the delete uses the post-deploy ETag.
	if existing.Status != awsw.VpcOriginStatusDeployed {
		log.WithFields(log.Fields{"Name": c.Identifier(), "Id": existing.Id}).Info("waiting for cloudfront vpc origin to deploy before delete")
		if err := cfClient.WaitForVpcOriginDeployed(ctx, existing.Id); err != nil {
			return errors.Wrapf(err, "cloudfront vpc origin %v failed to reach deployed state before delete", c.Identifier())
		}
		if existing, err = cfClient.FindVpcOriginByName(ctx, c.Identifier()); err != nil {
			return err
		}
		if existing == nil {
			log.WithFields(log.Fields{"Name": c.Identifier()}).Info("cloudfront vpc origin does not exist, nothing to destroy")
			return nil
		}
	}

	if err := cfClient.DeleteVpcOrigin(ctx, existing.Id, existing.ETag); err != nil {
		return err
	}

	log.WithFields(log.Fields{"Name": c.Identifier(), "Id": existing.Id}).Info(color.Red("cloudfront vpc origin destroyed"))
	return nil
}

// CloudfrontVpcOriginDiff captures the diff between desired and existing state.
type CloudfrontVpcOriginDiff struct {
	BaseResourceDiff

	id            string
	arn           string
	etag          string
	desired       *cftypes.VpcOriginEndpointConfig
	configChanged bool
	tagsDiff      bool
	tagDiff       util.TagDiffResult
}

// Compare fetches the existing VPC origin and diffs it against the desired config.
func (c CloudfrontVpcOrigin) Compare(ctx context.Context) (ResourceDiff, error) {
	cfClient := awsw.NewCloudfront(ctx, c.Context.ProviderName)
	existing, err := cfClient.FindVpcOriginByName(ctx, c.Identifier())
	if err != nil {
		return nil, err
	}

	diffs := &CloudfrontVpcOriginDiff{}
	if existing == nil {
		diffs.Messages = append(diffs.Messages, "cloudfront vpc origin does not exist")
		return diffs, nil
	}

	desired, err := c.generateEndpointConfig(ctx)
	if err != nil {
		return nil, err
	}

	diffs.Resource = existing
	diffs.id = existing.Id
	diffs.arn = existing.ARN
	diffs.etag = existing.ETag
	diffs.desired = desired

	configMsgs := compareVpcOriginEndpointConfig(existing.Config, *desired)
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

// generateEndpointConfig builds a complete VpcOriginEndpointConfig from the resource
// definition, resolving the target load balancer to its ARN. The AWS client is only
// constructed when the target actually needs resolving, so ARN targets are AWS-free.
func (c CloudfrontVpcOrigin) generateEndpointConfig(ctx context.Context) (*cftypes.VpcOriginEndpointConfig, error) {
	target, targetProvider := awsw.ParseIdentifier(c.Target)
	if targetProvider == "" {
		targetProvider = c.Context.ProviderName
	}

	endpointArn := target
	if !strings.HasPrefix(endpointArn, "arn:") {
		arn, err := awsw.NewELB(ctx, targetProvider).LoadBalancerArnForIdentifier(ctx, target)
		if err != nil {
			return nil, errors.Wrapf(err, "error resolving load balancer %q for cloudfront vpc origin %v", c.Target, c.Identifier())
		}
		endpointArn = aws.ToString(arn)
	}

	return &cftypes.VpcOriginEndpointConfig{
		Name:                 aws.String(c.Identifier()),
		Arn:                  aws.String(endpointArn),
		HTTPPort:             c.HTTPPort,
		HTTPSPort:            c.HTTPSPort,
		OriginProtocolPolicy: cftypes.OriginProtocolPolicy(aws.ToString(c.OriginProtocolPolicy)),
		OriginSslProtocols: &cftypes.OriginSslProtocols{
			Quantity: aws.Int32(1),
			Items:    []cftypes.SslProtocol{cftypes.SslProtocol(aws.ToString(c.OriginSSLProtocol))},
		},
	}, nil
}

// compareVpcOriginEndpointConfig returns human-readable messages describing the
// differences between an existing endpoint config and the desired one. An empty
// result means no change is required. Name is not compared: existing origins are
// looked up by name, so it always matches.
func compareVpcOriginEndpointConfig(existing, desired cftypes.VpcOriginEndpointConfig) []string {
	var msgs []string

	if aws.ToString(existing.Arn) != aws.ToString(desired.Arn) {
		msgs = append(msgs, fmt.Sprintf("target: %q -> %q", aws.ToString(existing.Arn), aws.ToString(desired.Arn)))
	}
	if aws.ToInt32(existing.HTTPPort) != aws.ToInt32(desired.HTTPPort) {
		msgs = append(msgs, fmt.Sprintf("httpPort: %d -> %d", aws.ToInt32(existing.HTTPPort), aws.ToInt32(desired.HTTPPort)))
	}
	if aws.ToInt32(existing.HTTPSPort) != aws.ToInt32(desired.HTTPSPort) {
		msgs = append(msgs, fmt.Sprintf("httpsPort: %d -> %d", aws.ToInt32(existing.HTTPSPort), aws.ToInt32(desired.HTTPSPort)))
	}
	if existing.OriginProtocolPolicy != desired.OriginProtocolPolicy {
		msgs = append(msgs, fmt.Sprintf("originProtocolPolicy: %q -> %q", existing.OriginProtocolPolicy, desired.OriginProtocolPolicy))
	}

	if e, d := sslProtocolStrings(existing.OriginSslProtocols), sslProtocolStrings(desired.OriginSslProtocols); !slices.Equal(e, d) {
		msgs = append(msgs, fmt.Sprintf("originSslProtocols: %v -> %v", e, d))
	}

	return msgs
}

// sslProtocolStrings returns the protocols as a sorted string slice so the comparison
// is order-insensitive.
func sslProtocolStrings(p *cftypes.OriginSslProtocols) []string {
	if p == nil {
		return nil
	}
	out := make([]string, 0, len(p.Items))
	for _, i := range p.Items {
		out = append(out, string(i))
	}
	slices.Sort(out)
	return out
}
