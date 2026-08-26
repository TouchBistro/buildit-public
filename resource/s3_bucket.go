package resource

import (
	"context"
	"fmt"
	"strings"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/TouchBistro/buildit/util"
	"github.com/TouchBistro/goutils/color"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

type S3Bucket struct {
	BaseResource `yaml:",inline"`
	Bucket       string            `yaml:"-"`
	Tags         map[string]string `yaml:"tags,omitempty"`
	GlobalTags   map[string]string `yaml:"-"`
	EventBridge  bool              `yaml:"eventbridge,omitempty"`
	ForceDestroy bool              `yaml:"forceDestroy"`
	DependsOn    []Key             `yaml:"dependsOn,omitempty"`
}

// Key returns the unique key for the resource for this buildit context
func (b S3Bucket) Key() Key {
	return NewKey(b.Context.ProviderName, b.Identifier())
}

// Identifier returns the bucket name
func (b S3Bucket) Identifier() string {
	return b.Bucket
}

// Normalize will set any default values or sanitize/clean up any necessary fields.
func (b *S3Bucket) Normalize(ctx context.Context) {
	b.Bucket = strings.TrimPrefix(b.Bucket, "arn:aws:s3:::")

	if b.Tags == nil {
		b.Tags = make(map[string]string)
	}

	ResourceTags(b.Tags).Merge(b.GlobalTags)

	// Bucket was trimmed above, so a resource named by its ARN would carry a resource-id
	// stamped from the untrimmed config key. Re-point it at the name Identifier() returns.
	if _, ok := b.Tags[util.BuilditResourceIDTagKey]; ok {
		b.Tags[util.BuilditResourceIDTagKey] = util.SafeTagValue(b.Bucket)
	}
}

// Validate checks that the input provided is correct
func (b S3Bucket) Validate(ctx context.Context) error {
	if b.Bucket == "" {
		return &ValidationError{
			ResourceIdentifier: b.Identifier(),
			ResourceType:       "S3Bucket",
			Messages:           []string{"bucket name is required"},
		}
	}
	return nil
}

// Apply creates the S3 bucket
func (b S3Bucket) Apply(ctx context.Context) error {
	log.Debugf("creating s3 bucket %v", b.Identifier())

	diffs, err := b.Compare(ctx)
	if err != nil {
		return err
	}

	if diffs == nil {
		log.WithFields(log.Fields{"Bucket": b.Identifier()}).Info("no updates required")
		return nil
	}

	if diffs.AWSResource() != nil {
		return b.applyDiffs(ctx, diffs)
	}

	return b.apply(ctx)
}

func (b S3Bucket) applyDiffs(ctx context.Context, diffs ResourceDiff) error {
	s3Diff, ok := diffs.(*S3BucketDiff)
	if !ok {
		return errors.New("invalid diff type")
	}

	s3Client := awsw.NewS3(ctx, b.Context.ProviderName)

	if s3Diff.tagsDiff {
		err := s3Client.PutBucketTagging(ctx, b.Bucket, b.Tags)
		if err != nil {
			return errors.Wrapf(err, "error putting bucket tagging %v", b.Bucket)
		}
	}

	if s3Diff.eventBridgeDiff {
		err := s3Client.SetBucketEventBridgeNotification(ctx, b.Bucket, b.EventBridge)
		if err != nil {
			return errors.Wrapf(err, "error setting eventbridge notification %v", b.Bucket)
		}
	}

	log.WithFields(log.Fields{
		"Bucket": b.Identifier(),
	}).Infof("%s", color.Yellow("s3 bucket updated"))

	return nil
}

func (b S3Bucket) apply(ctx context.Context) error {
	s3Client := awsw.NewS3(ctx, b.Context.ProviderName)
	err := s3Client.CreateBucket(ctx, b.Bucket)
	if err != nil {
		return errors.Wrapf(err, "error creating bucket %v", b.Bucket)
	}

	if len(b.Tags) > 0 {
		err := s3Client.PutBucketTagging(ctx, b.Bucket, b.Tags)
		if err != nil {
			return errors.Wrapf(err, "error putting bucket tagging %v", b.Bucket)
		}
	}

	if b.EventBridge {
		err := s3Client.SetBucketEventBridgeNotification(ctx, b.Bucket, true)
		if err != nil {
			return errors.Wrapf(err, "error setting eventbridge notification %v", b.Bucket)
		}
	}

	log.WithFields(log.Fields{
		"Bucket": b.Bucket,
	}).Infof("%s", color.Green("s3 bucket created"))

	return nil
}

// Destroy deletes the bucket
func (b S3Bucket) Destroy(ctx context.Context) error {
	s3Client := awsw.NewS3(ctx, b.Context.ProviderName)

	// Check if exists
	exists, err := b.bucketExists(ctx)
	if err != nil {
		return errors.Wrapf(err, "error checking if bucket %v exists", b.Bucket)
	}
	if !exists {
		log.WithFields(log.Fields{"Bucket": b.Identifier()}).Info("bucket does not exist, nothing to destroy")
		return nil
	}

	if b.ForceDestroy {
		log.WithFields(log.Fields{"Bucket": b.Identifier()}).Info("force destroy enabled, emptying bucket")
		err := s3Client.EmptyBucket(ctx, b.Bucket)
		if err != nil {
			return errors.Wrapf(err, "error emptying bucket %v", b.Bucket)
		}
	}

	err = s3Client.DeleteBucket(ctx, b.Bucket)
	if err != nil {
		return errors.Wrapf(err, "error deleting bucket %v", b.Bucket)
	}

	log.WithFields(log.Fields{
		"Bucket": b.Bucket,
	}).Infof("%s", color.Red("s3 bucket deleted"))

	return nil
}

type S3BucketDiff struct {
	BaseResourceDiff
	tagsDiff        bool
	tagDiff         util.TagDiffResult
	eventBridgeDiff bool
}

// bucketInfo represents basic bucket information for diff purposes
type bucketInfo struct {
	BucketName string
	Exists     bool
}

// Compare checks if the bucket exists
func (b S3Bucket) Compare(ctx context.Context) (ResourceDiff, error) {
	exists, err := b.bucketExists(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "error checking if bucket %v exists", b.Bucket)
	}

	if !exists {
		diffs := &S3BucketDiff{}
		diffs.Messages = append(diffs.Messages, "bucket does not exist")
		return diffs, nil
	}

	s3Client := awsw.NewS3(ctx, b.Context.ProviderName)
	existingTags, err := s3Client.GetBucketTagging(ctx, b.Bucket)
	if err != nil {
		// GetBucketTagging returns NoSuchTagSet if no tags exist
		if !strings.Contains(err.Error(), "NoSuchTagSet") {
			return nil, errors.Wrapf(err, "error getting tags for bucket %v", b.Bucket)
		}
	}

	diffs := &S3BucketDiff{
		BaseResourceDiff: BaseResourceDiff{
			Messages: []string{},
			// Store bucket information as the resource representation
			Resource: &bucketInfo{
				BucketName: b.Bucket,
				Exists:     true,
			},
		},
	}

	ebEnabled, err := s3Client.GetBucketEventBridgeNotification(ctx, b.Bucket)
	if err != nil {
		return nil, errors.Wrapf(err, "error getting eventbridge notification for bucket %v", b.Bucket)
	}

	if b.EventBridge != ebEnabled {
		diffs.eventBridgeDiff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("EventBridge notification: %v -> %v", ebEnabled, b.EventBridge))
	}

	if tagDiff := TagDiffForContext(ctx, existingTags, b.Tags); tagDiff.HasChanges() {
		diffs.tagsDiff = true
		diffs.tagDiff = tagDiff
		diffs.Messages = append(diffs.Messages, TagDiffSummary(existingTags, diffs.tagDiff)...)
	}

	if len(diffs.Messages) == 0 {
		return nil, nil
	}

	return diffs, nil
}

func (b S3Bucket) bucketExists(ctx context.Context) (bool, error) {
	s3Client := awsw.NewS3(ctx, b.Context.ProviderName)
	err := s3Client.HeadBucket(ctx, b.Bucket)
	if err != nil {
		var nf *types.NotFound
		if errors.As(err, &nf) {
			return false, nil
		}

		// If it's a 403, we don't have permission to access this bucket
		// Return an error to make this clear to the user
		if strings.Contains(err.Error(), "Forbidden") || strings.Contains(err.Error(), "403") {
			return false, errors.Wrapf(err, "access denied to bucket %v - check IAM permissions", b.Bucket)
		}

		// If we can't determine, return error
		return false, err
	}
	return true, nil
}
