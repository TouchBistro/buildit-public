package awsw

import (
	"context"

	"github.com/TouchBistro/buildit/client"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrock"
	bedrocktypes "github.com/aws/aws-sdk-go-v2/service/bedrock/types"
	"github.com/pkg/errors"
)

type Bedrock struct {
	*bedrock.Client
}

// NewBedrock creates a new instance of Bedrock wrapper
func NewBedrock(ctx context.Context, providerName string) Bedrock {
	return Bedrock{client.Bedrock(ctx, providerName)}
}

// ApplicationInferenceProfileByName paginates list results for application inference profiles
// and returns the summary matching the supplied name, or nil if not found. Matches by name
// only; callers must inspect Status to determine whether the profile is usable.
func (b Bedrock) ApplicationInferenceProfileByName(ctx context.Context, name string) (*bedrocktypes.InferenceProfileSummary, error) {
	var token *string
	for {
		out, err := b.ListInferenceProfiles(ctx, &bedrock.ListInferenceProfilesInput{
			TypeEquals: bedrocktypes.InferenceProfileTypeApplication,
			MaxResults: aws.Int32(100),
			NextToken:  token,
		})
		if err != nil {
			return nil, errors.Wrapf(err, "error listing bedrock application inference profiles")
		}
		for _, p := range out.InferenceProfileSummaries {
			if aws.ToString(p.InferenceProfileName) == name {
				return &p, nil
			}
		}
		if out.NextToken == nil {
			break
		}
		token = out.NextToken
	}
	return nil, nil
}

// CreateApplicationInferenceProfile creates a new application inference profile.
func (b Bedrock) CreateApplicationInferenceProfile(ctx context.Context, input *bedrock.CreateInferenceProfileInput) (*bedrock.CreateInferenceProfileOutput, error) {
	out, err := b.CreateInferenceProfile(ctx, input)
	if err != nil {
		return nil, errors.Wrapf(err, "error creating bedrock application inference profile %v", aws.ToString(input.InferenceProfileName))
	}
	return out, nil
}

// DeleteApplicationInferenceProfile deletes the profile with the supplied ARN. Treats
// ResourceNotFoundException as a no-op to avoid spurious failures when the profile is
// deleted externally between a read and the delete call.
func (b Bedrock) DeleteApplicationInferenceProfile(ctx context.Context, arn string) error {
	_, err := b.DeleteInferenceProfile(ctx, &bedrock.DeleteInferenceProfileInput{
		InferenceProfileIdentifier: aws.String(arn),
	})
	if err != nil {
		var rnfe *bedrocktypes.ResourceNotFoundException
		if errors.As(err, &rnfe) {
			return nil
		}
		return errors.Wrapf(err, "error deleting bedrock application inference profile %v", arn)
	}
	return nil
}

// GetApplicationInferenceProfile fetches a profile by ARN. Returns (nil, nil) when the
// profile no longer exists (ResourceNotFoundException).
func (b Bedrock) GetApplicationInferenceProfile(ctx context.Context, arn string) (*bedrock.GetInferenceProfileOutput, error) {
	out, err := b.GetInferenceProfile(ctx, &bedrock.GetInferenceProfileInput{
		InferenceProfileIdentifier: aws.String(arn),
	})
	if err != nil {
		var rnfe *bedrocktypes.ResourceNotFoundException
		if errors.As(err, &rnfe) {
			return nil, nil
		}
		return nil, errors.Wrapf(err, "error getting bedrock inference profile %v", arn)
	}
	return out, nil
}

// GetProfileTags returns the tag map for the supplied bedrock resource arn.
// Returns (nil, nil) when the resource no longer exists (ResourceNotFoundException),
// matching the idempotent behaviour of the other Bedrock helpers.
func (b Bedrock) GetProfileTags(ctx context.Context, arn string) (map[string]string, error) {
	out, err := b.ListTagsForResource(ctx, &bedrock.ListTagsForResourceInput{
		ResourceARN: aws.String(arn),
	})
	if err != nil {
		var rnfe *bedrocktypes.ResourceNotFoundException
		if errors.As(err, &rnfe) {
			return nil, nil
		}
		return nil, errors.Wrapf(err, "error fetching tags for bedrock resource %v", arn)
	}
	tags := make(map[string]string, len(out.Tags))
	for _, t := range out.Tags {
		tags[aws.ToString(t.Key)] = aws.ToString(t.Value)
	}
	return tags, nil
}

// TagProfile adds/updates the supplied tag map onto the bedrock resource
func (b Bedrock) TagProfile(ctx context.Context, arn string, tags map[string]string) error {
	if len(tags) == 0 {
		return nil
	}
	awsTags := make([]bedrocktypes.Tag, 0, len(tags))
	for k, v := range tags {
		awsTags = append(awsTags, bedrocktypes.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}
	_, err := b.TagResource(ctx, &bedrock.TagResourceInput{
		ResourceARN: aws.String(arn),
		Tags:        awsTags,
	})
	if err != nil {
		return errors.Wrapf(err, "error tagging bedrock resource %v", arn)
	}
	return nil
}

// UntagProfile removes the supplied tag keys from the bedrock resource
func (b Bedrock) UntagProfile(ctx context.Context, arn string, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	_, err := b.UntagResource(ctx, &bedrock.UntagResourceInput{
		ResourceARN: aws.String(arn),
		TagKeys:     keys,
	})
	if err != nil {
		return errors.Wrapf(err, "error untagging bedrock resource %v", arn)
	}
	return nil
}
