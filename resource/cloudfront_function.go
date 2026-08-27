package resource

import (
	"context"
	"fmt"
	"regexp"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/TouchBistro/buildit/util"
	"github.com/TouchBistro/goutils/color"
	"github.com/aws/aws-sdk-go-v2/aws"
	cftypes "github.com/aws/aws-sdk-go-v2/service/cloudfront/types"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

const (
	// Function runtimes
	CloudfrontFunctionRuntimeJS10 = string(cftypes.FunctionRuntimeCloudfrontJs10)
	CloudfrontFunctionRuntimeJS20 = string(cftypes.FunctionRuntimeCloudfrontJs20)

	// cloudfrontFunctionMaxCodeBytes is the CloudFront API limit for function code size.
	cloudfrontFunctionMaxCodeBytes = 10 * 1024
)

// cloudfrontFunctionNamePattern is the CloudFront API constraint on function names.
var cloudfrontFunctionNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// CloudfrontFunction is a declarative AWS CloudFront Function resource. The function
// code is supplied inline in YAML (the CloudFront API only accepts inline code, capped
// at 10 KB). Apply always publishes the function to the LIVE stage, so the config
// describes what is actually serving traffic; distributions can only associate LIVE
// functions. KeyValueStore associations are not supported.
type CloudfrontFunction struct {
	BaseResource `yaml:",inline"`
	Name         string            `yaml:"-"` // from resource key; the function Name is unique per account
	Comment      *string           `yaml:"comment"`
	Runtime      *string           `yaml:"runtime"`
	Code         string            `yaml:"code"`
	Tags         map[string]string `yaml:"tags"`
	GlobalTags   map[string]string `yaml:"-"`
	DependsOn    []Key             `yaml:"dependsOn"`
}

// Key returns the unique key for the resource for this buildit context.
func (c CloudfrontFunction) Key() Key {
	return NewKey(c.Context.ProviderName, c.Identifier())
}

// Identifier returns the function name.
func (c CloudfrontFunction) Identifier() string {
	return c.Name
}

// Normalize sets default values and merges global tags.
func (c *CloudfrontFunction) Normalize(ctx context.Context) {
	if c.Runtime == nil {
		c.Runtime = aws.String(CloudfrontFunctionRuntimeJS20)
	}
	if c.Tags == nil {
		c.Tags = make(map[string]string)
	}
	ResourceTags(c.Tags).Merge(c.GlobalTags)
}

// Validate checks the resource configuration against CloudFront API constraints.
func (c CloudfrontFunction) Validate(ctx context.Context) error {
	var msgs []string

	if !cloudfrontFunctionNamePattern.MatchString(c.Name) {
		msgs = append(msgs, fmt.Sprintf("name %q must match %v", c.Name, cloudfrontFunctionNamePattern))
	}
	if c.Code == "" {
		msgs = append(msgs, "code is required")
	} else if len(c.Code) > cloudfrontFunctionMaxCodeBytes {
		msgs = append(msgs, fmt.Sprintf("code is %v bytes; the CloudFront limit is %v", len(c.Code), cloudfrontFunctionMaxCodeBytes))
	}
	if !validCloudfrontEnum(c.Runtime, CloudfrontFunctionRuntimeJS10, CloudfrontFunctionRuntimeJS20) {
		msgs = append(msgs, fmt.Sprintf("runtime must be one of: %v, %v", CloudfrontFunctionRuntimeJS10, CloudfrontFunctionRuntimeJS20))
	}

	if len(msgs) > 0 {
		return &ValidationError{
			ResourceIdentifier: c.Identifier(),
			ResourceType:       "cloudfront-function",
			Messages:           msgs,
		}
	}
	return nil
}

// CloudfrontFunctionDiff describes differences between the desired and actual function.
type CloudfrontFunctionDiff struct {
	BaseResourceDiff
	etag          string
	arn           string
	configChanged bool // code, comment, or runtime differs from the DEVELOPMENT stage
	publishOnly   bool // DEVELOPMENT matches desired but LIVE is missing or stale
	tagsDiff      bool
	tagDiff       util.TagDiffResult
}

// Compare checks for differences between the desired configuration and the existing
// function. Returns nil when there are no changes.
func (c CloudfrontFunction) Compare(ctx context.Context) (ResourceDiff, error) {
	log.Debugf("comparing cloudfront function %v", c.Identifier())

	cfClient := awsw.NewCloudfront(ctx, c.Context.ProviderName)
	existing, err := cfClient.FindFunctionByName(ctx, c.Identifier())
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return &CloudfrontFunctionDiff{
			BaseResourceDiff: BaseResourceDiff{Messages: []string{"cloudfront function does not exist"}},
		}, nil
	}

	liveCode, err := cfClient.LiveFunctionCode(ctx, c.Identifier())
	if err != nil {
		return nil, err
	}

	diff := c.computeDiff(ctx, existing, liveCode)
	if diff == nil {
		return nil, nil
	}
	return diff, nil
}

// computeDiff compares the desired state against the existing function. It is pure
// (no AWS calls; ctx only supplies plan-time ignored tag keys) so the comparison can
// be unit tested. Returns nil when identical.
func (c CloudfrontFunction) computeDiff(ctx context.Context, existing *awsw.ExistingFunction, liveCode []byte) *CloudfrontFunctionDiff {
	diff := &CloudfrontFunctionDiff{
		BaseResourceDiff: BaseResourceDiff{Resource: existing},
		etag:             existing.ETag,
		arn:              existing.ARN,
	}

	if string(existing.Code) != c.Code {
		diff.configChanged = true
		diff.Messages = append(diff.Messages, fmt.Sprintf("code: changed (%v -> %v bytes)", len(existing.Code), len(c.Code)))
	}
	if aws.ToString(existing.Config.Comment) != aws.ToString(c.Comment) {
		diff.configChanged = true
		diff.Messages = append(diff.Messages, fmt.Sprintf("comment: %v -> %v", aws.ToString(existing.Config.Comment), aws.ToString(c.Comment)))
	}
	if string(existing.Config.Runtime) != aws.ToString(c.Runtime) {
		diff.configChanged = true
		diff.Messages = append(diff.Messages, fmt.Sprintf("runtime: %v -> %v", existing.Config.Runtime, aws.ToString(c.Runtime)))
	}

	// The DEVELOPMENT stage matches the desired state; make sure it is actually live.
	if !diff.configChanged {
		if liveCode == nil {
			diff.publishOnly = true
			diff.Messages = append(diff.Messages, "function has never been published")
		} else if string(liveCode) != c.Code {
			diff.publishOnly = true
			diff.Messages = append(diff.Messages, "live stage is out of date with the development code")
		}
	}

	if tagDiff := TagDiffForContext(ctx, existing.Tags, c.Tags); tagDiff.HasChanges() {
		diff.tagsDiff = true
		diff.tagDiff = tagDiff
		diff.Messages = append(diff.Messages, TagDiffSummary(existing.Tags, tagDiff)...)
	}

	if !diff.configChanged && !diff.publishOnly && !diff.tagsDiff {
		return nil
	}
	return diff
}

// Apply creates or updates the function and publishes it to the LIVE stage.
func (c CloudfrontFunction) Apply(ctx context.Context) error {
	log.Debugf("applying cloudfront function %v", c.Identifier())

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

// apply creates and publishes a new function.
func (c CloudfrontFunction) apply(ctx context.Context) error {
	cfClient := awsw.NewCloudfront(ctx, c.Context.ProviderName)

	etag, err := cfClient.CreateFunction(ctx, c.Identifier(), c.functionConfig(), []byte(c.Code), c.Tags)
	if err != nil {
		return err
	}

	if err := c.publishAndWait(ctx, cfClient, etag); err != nil {
		return err
	}

	log.WithFields(log.Fields{"Name": c.Identifier()}).Info(color.Green("cloudfront function created"))
	return nil
}

// applyDiffs updates (if the config or code changed), reconciles tags, and publishes an
// existing function. A tags-only change is applied without a republish: the LIVE stage
// is untouched, so no deploy wait is needed.
func (c CloudfrontFunction) applyDiffs(ctx context.Context, diffs ResourceDiff) error {
	fnDiff, ok := diffs.(*CloudfrontFunctionDiff)
	if !ok {
		return errors.New("invalid diff type supplied")
	}

	cfClient := awsw.NewCloudfront(ctx, c.Context.ProviderName)

	if fnDiff.tagsDiff {
		if upserts := fnDiff.tagDiff.Upserts(); len(upserts) > 0 {
			if err := cfClient.AddFunctionTags(ctx, fnDiff.arn, upserts); err != nil {
				return err
			}
		}
		if keys := fnDiff.tagDiff.DeletedKeys(); len(keys) > 0 {
			if err := cfClient.DeleteFunctionTags(ctx, fnDiff.arn, keys); err != nil {
				return err
			}
		}
	}

	if fnDiff.configChanged || fnDiff.publishOnly {
		etag := fnDiff.etag
		if fnDiff.configChanged {
			newETag, err := cfClient.UpdateFunctionCode(ctx, c.Identifier(), etag, c.functionConfig(), []byte(c.Code))
			if err != nil {
				return err
			}
			etag = newETag
		}

		if err := c.publishAndWait(ctx, cfClient, etag); err != nil {
			return err
		}
	}

	log.WithFields(log.Fields{"Name": c.Identifier()}).Info(color.Yellow("cloudfront function updated"))
	return nil
}

// publishAndWait publishes the DEVELOPMENT stage to LIVE and waits for the deploy to settle.
func (c CloudfrontFunction) publishAndWait(ctx context.Context, cfClient awsw.Cloudfront, etag string) error {
	if err := cfClient.PublishFunctionVersion(ctx, c.Identifier(), etag); err != nil {
		return err
	}
	return cfClient.WaitForFunctionDeployed(ctx, c.Identifier())
}

// Destroy deletes the function. No-op if it does not exist.
func (c CloudfrontFunction) Destroy(ctx context.Context) error {
	log.Debugf("destroying cloudfront function %v", c.Identifier())

	cfClient := awsw.NewCloudfront(ctx, c.Context.ProviderName)
	existing, err := cfClient.FindFunctionByName(ctx, c.Identifier())
	if err != nil {
		return err
	}
	if existing == nil {
		log.WithFields(log.Fields{"Name": c.Identifier()}).Info("cloudfront function does not exist, nothing to destroy")
		return nil
	}

	if err := cfClient.DeleteFunctionByETag(ctx, c.Identifier(), existing.ETag); err != nil {
		return err
	}

	log.WithFields(log.Fields{"Name": c.Identifier()}).Info(color.Red("cloudfront function destroyed"))
	return nil
}

// functionConfig builds the SDK FunctionConfig. Comment is a required API member, so an
// unset comment is sent as an empty string.
func (c CloudfrontFunction) functionConfig() *cftypes.FunctionConfig {
	return &cftypes.FunctionConfig{
		Comment: aws.String(aws.ToString(c.Comment)),
		Runtime: cftypes.FunctionRuntime(aws.ToString(c.Runtime)),
	}
}
