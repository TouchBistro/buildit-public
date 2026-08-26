package resource

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/TouchBistro/buildit/util"
	"github.com/TouchBistro/goutils/color"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrock"
	bedrocktypes "github.com/aws/aws-sdk-go-v2/service/bedrock/types"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

const (
	bedrockProfileDestroyWaitTimeout  = 90 * time.Second
	bedrockProfileDestroyWaitInterval = 3 * time.Second
	bedrockProfileNameMaxLen          = 64
)

// AWS-enforced pattern for application inference profile names.
var bedrockProfileNameRe = regexp.MustCompile(`^([0-9a-zA-Z][ _-]?)+$`)

// BedrockApplicationInferenceProfile represents an Amazon Bedrock application inference
// profile. Application inference profiles copy from a foundation model ARN or a
// system-defined inference profile ARN to track metrics and costs for invocations.
//
// After creation, only the profile's tags are mutable. Description and modelSource
// are immutable -- surface diff messages but skip applying changes to those fields.
type BedrockApplicationInferenceProfile struct {
	BaseResource `yaml:",inline"`
	Name        string            `yaml:"-"`
	Description string            `yaml:"description"`
	ModelSource string            `yaml:"modelSource"`
	Tags        map[string]string `yaml:"tags"`
	DependsOn   []Key             `yaml:"dependsOn"`
	GlobalTags  map[string]string `yaml:"-"`
}

// Key returns the unique key for the resource for this buildit context
func (r BedrockApplicationInferenceProfile) Key() Key {
	return NewKey(r.Context.ProviderName, r.Identifier())
}

// Identifier returns the unique ID for the inference profile
func (r BedrockApplicationInferenceProfile) Identifier() string {
	return r.Name
}

// Normalize sets defaults and merges global tags
func (r *BedrockApplicationInferenceProfile) Normalize(ctx context.Context) {
	if r.Tags == nil {
		r.Tags = make(map[string]string)
	}
	ResourceTags(r.Tags).Merge(r.GlobalTags)
}

// Validate checks that required fields are present and correctly formatted
func (r BedrockApplicationInferenceProfile) Validate(ctx context.Context) error {
	var msgs []string

	if r.Identifier() == "" {
		msgs = append(msgs, "bedrock application inference profile name cannot be empty")
	} else {
		if len(r.Identifier()) > bedrockProfileNameMaxLen {
			msgs = append(msgs, "bedrock application inference profile name must be 64 characters or fewer")
		}
		if !bedrockProfileNameRe.MatchString(r.Identifier()) {
			msgs = append(msgs, `bedrock application inference profile name must match ^([0-9a-zA-Z][ _-]?)+$`)
		}
	}

	if r.ModelSource == "" {
		msgs = append(msgs, "modelSource is required")
	} else if !strings.HasPrefix(r.ModelSource, "arn:aws") ||
		!strings.Contains(r.ModelSource, ":bedrock:") ||
		(!strings.Contains(r.ModelSource, ":foundation-model/") && !strings.Contains(r.ModelSource, ":inference-profile/")) {
		msgs = append(msgs, "modelSource must be a bedrock foundation-model or inference-profile ARN")
	}

	if msgs == nil {
		return nil
	}
	return &ValidationError{
		ResourceIdentifier: r.Identifier(),
		ResourceType:       "BedrockApplicationInferenceProfile",
		Messages:           msgs,
	}
}

// Apply creates or updates the inference profile on AWS
func (r BedrockApplicationInferenceProfile) Apply(ctx context.Context) error {
	log.Debugf("applying bedrock application inference profile %v", r.Identifier())

	diffs, err := r.Compare(ctx)
	if err != nil {
		return err
	}

	if diffs == nil {
		log.WithFields(log.Fields{
			"Name": r.Identifier(),
		}).Info("no updates required")
		return nil
	}

	if diffs.AWSResource() != nil {
		d, ok := diffs.(*BedrockApplicationInferenceProfileDiff)
		if !ok {
			return errors.New("invalid diff type supplied")
		}
		for _, msg := range diffs.Differences() {
			log.Debug(msg)
		}
		if d.hasMutableChanges() {
			log.WithField("Name", r.Identifier()).Info("bedrock application inference profile already exists, updating")
		}
		if err := r.applyDiffs(ctx, diffs); err != nil {
			return errors.Wrapf(err, "failed to update bedrock application inference profile %v", r.Identifier())
		}
		return nil
	}

	return r.apply(ctx)
}

// Destroy removes the inference profile from AWS
func (r BedrockApplicationInferenceProfile) Destroy(ctx context.Context) error {
	log.Debugf("destroying bedrock application inference profile %v", r.Identifier())

	existing, err := r.fetchExisting(ctx)
	if err != nil {
		return errors.Wrapf(err, "error finding bedrock application inference profile %v", r.Identifier())
	}
	if existing == nil {
		log.WithField("Name", r.Identifier()).Info("bedrock application inference profile does not exist, nothing to destroy, skipping")
		return nil
	}

	arn := aws.ToString(existing.InferenceProfileArn)
	bw := awsw.NewBedrock(ctx, r.Context.ProviderName)
	if err := bw.DeleteApplicationInferenceProfile(ctx, arn); err != nil {
		return errors.Wrapf(err, "error deleting bedrock application inference profile %v", r.Identifier())
	}

	if err := r.waitForDeletion(ctx, arn); err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"Name": r.Identifier(),
		"Arn":  arn,
	}).Info(color.Red("bedrock application inference profile destroyed"))
	return nil
}

// waitForDeletion polls GetInferenceProfile until the profile returns
// ResourceNotFoundException, or the bounded deadline is reached.
func (r BedrockApplicationInferenceProfile) waitForDeletion(ctx context.Context, arn string) error {
	bw := awsw.NewBedrock(ctx, r.Context.ProviderName)
	deadline := time.Now().Add(bedrockProfileDestroyWaitTimeout)

	for {
		if err := ctx.Err(); err != nil {
			return errors.Wrap(err, "context cancelled while waiting for bedrock application inference profile deletion")
		}
		cur, err := bw.GetApplicationInferenceProfile(ctx, arn)
		if err != nil {
			return err
		}
		if cur == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.Errorf("timed out waiting for bedrock application inference profile %v to delete", r.Identifier())
		}
		if err := util.SleepWithContext(ctx, bedrockProfileDestroyWaitInterval); err != nil {
			return errors.Wrap(err, "context cancelled while waiting for bedrock application inference profile deletion")
		}
	}
}

// BedrockApplicationInferenceProfileDiff represents diffs between the buildit
// definition and the AWS representation
type BedrockApplicationInferenceProfileDiff struct {
	BaseResourceDiff

	status          bedrocktypes.InferenceProfileStatus
	statusDiff      bool
	descriptionDiff bool
	modelSourceDiff bool
	tagsDiff        bool
	tagDiff         util.TagDiffResult
}

// hasMutableChanges reports whether the diff contains changes that can actually
// be applied to an existing profile. Only tag changes are mutable; description
// and modelSource require destroy & recreate, and a non-Active status blocks
// any mutation entirely.
func (d *BedrockApplicationInferenceProfileDiff) hasMutableChanges() bool {
	return !d.statusDiff && d.tagsDiff
}

// Compare fetches the existing profile and returns diffs, or nil if no differences
func (r BedrockApplicationInferenceProfile) Compare(ctx context.Context) (ResourceDiff, error) {
	existing, err := r.fetchExisting(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "error fetching aws resource for %v", r.Identifier())
	}

	var awsTags map[string]string
	if existing != nil && existing.Status == bedrocktypes.InferenceProfileStatusActive {
		bw := awsw.NewBedrock(ctx, r.Context.ProviderName)
		awsTags, err = bw.GetProfileTags(ctx, aws.ToString(existing.InferenceProfileArn))
		if err != nil {
			return nil, err
		}
	}

	diffs := r.computeDiff(ctx, existing, awsTags)
	if diffs == nil {
		return nil, nil
	}
	return diffs, nil
}

// computeDiff is the pure diff calculation separated from AWS calls so it can
// be unit-tested with constructed inputs. Returns nil when the existing profile
// matches the desired state.
func (r BedrockApplicationInferenceProfile) computeDiff(ctx context.Context, existing *bedrocktypes.InferenceProfileSummary, awsTags map[string]string) *BedrockApplicationInferenceProfileDiff {
	diffs := &BedrockApplicationInferenceProfileDiff{}

	if existing == nil {
		diffs.Messages = append(diffs.Messages, "bedrock application inference profile does not exist")
		return diffs
	}

	diffs.Resource = existing

	// Non-Active statuses (e.g. CREATING/FAILED/DELETING) indicate the profile is not
	// usable. Surface as a diff but skip field/tag comparison -- the Models list may
	// be empty or stale, and tag mutations will be rejected by AWS.
	if existing.Status != bedrocktypes.InferenceProfileStatusActive {
		diffs.statusDiff = true
		diffs.status = existing.Status
		diffs.Messages = append(diffs.Messages, "profile status is "+string(existing.Status)+"; destroy & recreate may be required")
		return diffs
	}

	diff := false

	if aws.ToString(existing.Description) != r.Description {
		diff = true
		diffs.descriptionDiff = true
		diffs.Messages = append(diffs.Messages, "description is different (immutable; destroy & recreate to change)")
	}

	// AWS does not return the original modelSource (CopyFrom value) on read;
	// only Models[].ModelArn is exposed. We can only detect drift when ModelSource
	// is a foundation-model ARN -- in that case it equals Models[0].ModelArn.
	// When ModelSource is a system-defined inference-profile ARN, the underlying
	// Models[].ModelArn list is the resolved set of foundation models, which won't
	// equal the system-profile ARN -- so drift is undetectable for that case.
	if strings.Contains(r.ModelSource, ":foundation-model/") &&
		len(existing.Models) == 1 &&
		aws.ToString(existing.Models[0].ModelArn) != r.ModelSource {
		diff = true
		diffs.modelSourceDiff = true
		diffs.Messages = append(diffs.Messages, "modelSource is different (immutable; destroy & recreate to change)")
	}

	if tagDiff := TagDiffForContext(ctx, awsTags, r.Tags); tagDiff.HasChanges() {
		diff = true
		diffs.tagsDiff = true
		diffs.tagDiff = tagDiff
		diffs.Messages = append(diffs.Messages, TagDiffSummary(awsTags, diffs.tagDiff)...)
	}

	if !diff {
		return nil
	}
	return diffs
}

// fetchExisting fetches the application inference profile if it exists
func (r BedrockApplicationInferenceProfile) fetchExisting(ctx context.Context) (*bedrocktypes.InferenceProfileSummary, error) {
	return awsw.NewBedrock(ctx, r.Context.ProviderName).ApplicationInferenceProfileByName(ctx, r.Name)
}

// apply creates a new application inference profile
func (r BedrockApplicationInferenceProfile) apply(ctx context.Context) error {
	bw := awsw.NewBedrock(ctx, r.Context.ProviderName)

	input := &bedrock.CreateInferenceProfileInput{
		InferenceProfileName: aws.String(r.Name),
		ModelSource: &bedrocktypes.InferenceProfileModelSourceMemberCopyFrom{
			Value: r.ModelSource,
		},
	}
	if r.Description != "" {
		input.Description = aws.String(r.Description)
	}
	if len(r.Tags) > 0 {
		awsTags := make([]bedrocktypes.Tag, 0, len(r.Tags))
		for k, v := range r.Tags {
			awsTags = append(awsTags, bedrocktypes.Tag{
				Key:   aws.String(k),
				Value: aws.String(v),
			})
		}
		input.Tags = awsTags
	}

	out, err := bw.CreateApplicationInferenceProfile(ctx, input)
	if err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"Name": r.Identifier(),
		"Arn":  aws.ToString(out.InferenceProfileArn),
	}).Info(color.Green("bedrock application inference profile created"))
	return nil
}

// applyDiffs updates tag diffs on an existing profile. Status, description, and
// modelSource issues are logged as warnings since AWS does not support updating
// them in place (or blocks mutation entirely when the profile is non-Active).
func (r BedrockApplicationInferenceProfile) applyDiffs(ctx context.Context, diffs ResourceDiff) error {
	if diffs == nil {
		log.WithField("Name", r.Identifier()).Info("no updates required for bedrock application inference profile")
		return nil
	}

	d, ok := diffs.(*BedrockApplicationInferenceProfileDiff)
	if !ok {
		return errors.New("invalid diff type supplied")
	}

	existing, ok := d.Resource.(*bedrocktypes.InferenceProfileSummary)
	if !ok {
		return errors.Errorf("cannot retrieve existing bedrock application inference profile")
	}

	if d.statusDiff {
		log.WithFields(log.Fields{
			"Name":   r.Identifier(),
			"Status": string(d.status),
		}).Warn("bedrock application inference profile is not Active; skipping update — destroy & recreate may be required")
		return nil
	}

	if d.descriptionDiff {
		log.WithField("Name", r.Identifier()).Warn("bedrock application inference profile description is immutable; destroy & recreate to change")
	}
	if d.modelSourceDiff {
		log.WithField("Name", r.Identifier()).Warn("bedrock application inference profile modelSource is immutable; destroy & recreate to change")
	}

	arn := aws.ToString(existing.InferenceProfileArn)

	if !d.tagsDiff {
		if d.descriptionDiff || d.modelSourceDiff {
			log.WithField("Name", r.Identifier()).Warn("no mutable changes applied; destroy & recreate required to change immutable fields")
		}
		return nil
	}

	bw := awsw.NewBedrock(ctx, r.Context.ProviderName)

	if upserts := d.tagDiff.Upserts(); len(upserts) > 0 {
		if err := bw.TagProfile(ctx, arn, upserts); err != nil {
			return errors.Wrapf(err, "error updating tags for bedrock application inference profile %v", r.Identifier())
		}
	}
	if keys := d.tagDiff.DeletedKeys(); len(keys) > 0 {
		if err := bw.UntagProfile(ctx, arn, keys); err != nil {
			return errors.Wrapf(err, "error removing tags for bedrock application inference profile %v", r.Identifier())
		}
	}

	log.WithFields(log.Fields{
		"Name": r.Identifier(),
		"Arn":  arn,
	}).Info(color.Yellow("bedrock application inference profile updated"))
	return nil
}
