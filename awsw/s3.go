package awsw

import (
	"context"
	"fmt"
	"strings"

	"github.com/TouchBistro/awesome/providers"
	"github.com/TouchBistro/buildit/client"
	"github.com/TouchBistro/buildit/util"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type S3 struct {
	*s3.Client
}

func NewS3(ctx context.Context, providerName string) S3 {
	return S3{client.S3(ctx, providerName)}
}

// GetObjectChecksumAndSize reads the object's SHA256 checksum & size in bytes; passes the Range header options with
// 1-10/* fixed header value to read 10 bytes
func (s S3) GetObjectChecksumAndSize(ctx context.Context, bucket, key string, version *string) (*string, *int64, error) {
	out, err := s.GetObject(ctx, &s3.GetObjectInput{
		Range:        aws.String("1-10/*"), // read 10bytes only
		Bucket:       aws.String(bucket),
		Key:          aws.String(key),
		VersionId:    version,
		ChecksumMode: types.ChecksumModeEnabled,
	})
	if err != nil {
		return nil, nil, err
	}

	return out.ChecksumSHA256, out.ContentLength, nil
}

// GetObjectChecksumAndSize reads the object's checksum; supports SHA256,SHA1,CRC32,RC32C
func (s S3) GetObjectChecksum(ctx context.Context, bucket, key string, version *string) (*string, error) {
	out, err := s.GetObjectAttributes(ctx, &s3.GetObjectAttributesInput{
		Bucket:    aws.String(bucket),
		Key:       aws.String(key),
		VersionId: version,
		ObjectAttributes: []types.ObjectAttributes{
			types.ObjectAttributesChecksum,
		},
	})
	if err != nil {
		return nil, err
	}

	checksum := out.Checksum
	switch {
	case checksum.ChecksumSHA256 != nil:
		return checksum.ChecksumSHA256, nil
	case checksum.ChecksumSHA1 != nil:
		return checksum.ChecksumSHA1, nil
	case checksum.ChecksumCRC32 != nil:
		return checksum.ChecksumCRC32, nil
	case checksum.ChecksumCRC32C != nil:
		return checksum.ChecksumCRC32C, nil
	}

	return nil, nil
}

// BucketRegion returns the region the supplied bucket resides in, via HeadBucket's
// x-amz-bucket-region response header (works regardless of the client's region).
func (s S3) BucketRegion(ctx context.Context, bucket string) (string, error) {
	out, err := s.Client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(bucket),
	})
	if err != nil {
		return "", fmt.Errorf("failed to find S3 bucket %q: %w", bucket, err)
	}
	if aws.ToString(out.BucketRegion) == "" {
		return "", fmt.Errorf("could not determine region for S3 bucket %q", bucket)
	}
	return aws.ToString(out.BucketRegion), nil
}

// BucketArnFromName returns the bucket arn for the supplied bucket name.
func (s S3) BucketArnFromName(ctx context.Context, name string) (*string, error) {
	provider, bucketName := ParseName(name)
	s3Client := s.Client
	if provider != "" {
		s3Client = client.S3(ctx, provider)
	}

	_, err := s3Client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(bucketName),
	})
	if err != nil {
		return nil, err
	}

	arn := fmt.Sprintf("arn:aws:s3:::%s", bucketName)
	return &arn, nil
}

// BucketArnForIdentifier resolves a bucket ARN from an identifier (ARN or name).
func (s S3) BucketArnForIdentifier(ctx context.Context, identifier string) (*string, error) {
	resource, provider := ParseIdentifier(identifier)
	// If it's already an ARN, ParseIdentifier returns it as the resource and provider as empty
	if strings.HasPrefix(resource, "arn:") {
		return &resource, nil
	}

	s3Client := s.Client
	if provider != "" {
		// Verify provider exists to avoid fatal error in client.S3
		if _, err := providers.Get(provider); err != nil {
			return nil, fmt.Errorf("provider %q not found: %w", provider, err)
		}
		s3Client = client.S3(ctx, provider)
	}

	_, err := s3Client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(resource),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to find S3 bucket %q: %w", resource, err)
	}

	arn := fmt.Sprintf("arn:aws:s3:::%s", resource)
	return &arn, nil
}

// CreateBucket creates a new S3 bucket
func (s S3) CreateBucket(ctx context.Context, bucket string) error {
	input := &s3.CreateBucketInput{
		Bucket: aws.String(bucket),
	}

	region := s.Options().Region
	if region != "us-east-1" {
		input.CreateBucketConfiguration = &types.CreateBucketConfiguration{
			LocationConstraint: types.BucketLocationConstraint(region),
		}
	}

	_, err := s.Client.CreateBucket(ctx, input)
	return err
}

// DeleteBucket deletes an S3 bucket
func (s S3) DeleteBucket(ctx context.Context, bucket string) error {
	_, err := s.Client.DeleteBucket(ctx, &s3.DeleteBucketInput{
		Bucket: aws.String(bucket),
	})
	return err
}

// HeadBucket checks if a bucket exists and you have permission to access it.
func (s S3) HeadBucket(ctx context.Context, bucket string) error {
	_, err := s.Client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(bucket),
	})
	return err
}

// PutBucketTagging sets the tags for a bucket
func (s S3) PutBucketTagging(ctx context.Context, bucket string, tags map[string]string) error {
	tagSet := make([]types.Tag, 0, len(tags))
	keys := util.StringMap(tags).Keys()
	for _, k := range keys {
		tagSet = append(tagSet, types.Tag{
			Key:   aws.String(k),
			Value: aws.String(tags[k]),
		})
	}

	_, err := s.Client.PutBucketTagging(ctx, &s3.PutBucketTaggingInput{
		Bucket: aws.String(bucket),
		Tagging: &types.Tagging{
			TagSet: tagSet,
		},
	})
	return err
}

// GetBucketTagging gets the tags for a bucket
func (s S3) GetBucketTagging(ctx context.Context, bucket string) (map[string]string, error) {
	out, err := s.Client.GetBucketTagging(ctx, &s3.GetBucketTaggingInput{
		Bucket: aws.String(bucket),
	})
	if err != nil {
		return nil, err
	}

	tags := make(map[string]string, len(out.TagSet))
	for _, t := range out.TagSet {
		if t.Key != nil && t.Value != nil {
			tags[*t.Key] = *t.Value
		}
	}
	return tags, nil
}

// SetBucketEventBridgeNotification enables or disables EventBridge notifications for a bucket
func (s S3) SetBucketEventBridgeNotification(ctx context.Context, bucket string, enabled bool) error {
	// Get current configuration
	out, err := s.GetBucketNotificationConfiguration(ctx, &s3.GetBucketNotificationConfigurationInput{
		Bucket: aws.String(bucket),
	})
	if err != nil {
		return err
	}

	currentEnabled := out.EventBridgeConfiguration != nil
	if currentEnabled == enabled {
		return nil // No change needed
	}

	config := &types.NotificationConfiguration{
		LambdaFunctionConfigurations: out.LambdaFunctionConfigurations,
		QueueConfigurations:          out.QueueConfigurations,
		TopicConfigurations:          out.TopicConfigurations,
	}

	if enabled {
		config.EventBridgeConfiguration = &types.EventBridgeConfiguration{}
	} else {
		config.EventBridgeConfiguration = nil
	}

	_, err = s.PutBucketNotificationConfiguration(ctx, &s3.PutBucketNotificationConfigurationInput{
		Bucket:                    aws.String(bucket),
		NotificationConfiguration: config,
	})
	return err
}

// EmptyBucket deletes all objects and versions from a bucket
func (s S3) EmptyBucket(ctx context.Context, bucket string) error {
	var nextKeyMarker *string
	var nextVersionIDMarker *string

	for {
		listOutput, err := s.ListObjectVersions(ctx, &s3.ListObjectVersionsInput{
			Bucket:          aws.String(bucket),
			KeyMarker:       nextKeyMarker,
			VersionIdMarker: nextVersionIDMarker,
		})
		if err != nil {
			return err
		}

		var objects []types.ObjectIdentifier
		for _, v := range listOutput.Versions {
			objects = append(objects, types.ObjectIdentifier{
				Key:       v.Key,
				VersionId: v.VersionId,
			})
		}
		for _, d := range listOutput.DeleteMarkers {
			objects = append(objects, types.ObjectIdentifier{
				Key:       d.Key,
				VersionId: d.VersionId,
			})
		}

		if len(objects) > 0 {
			// Delete in batches of 1000
			for i := 0; i < len(objects); i += 1000 {
				end := min(i+1000, len(objects))
				batch := objects[i:end]

				_, err := s.DeleteObjects(ctx, &s3.DeleteObjectsInput{
					Bucket: aws.String(bucket),
					Delete: &types.Delete{
						Objects: batch,
						Quiet:   aws.Bool(true),
					},
				})
				if err != nil {
					return err
				}
			}
		}

		if !aws.ToBool(listOutput.IsTruncated) {
			break
		}
		nextKeyMarker = listOutput.NextKeyMarker
		nextVersionIDMarker = listOutput.NextVersionIdMarker
	}
	return nil
}

// GetBucketEventBridgeNotification checks if EventBridge notifications are enabled for a bucket
func (s S3) GetBucketEventBridgeNotification(ctx context.Context, bucket string) (bool, error) {
	out, err := s.GetBucketNotificationConfiguration(ctx, &s3.GetBucketNotificationConfigurationInput{
		Bucket: aws.String(bucket),
	})
	if err != nil {
		return false, err
	}

	return out.EventBridgeConfiguration != nil, nil
}

// ParseName parses n and returns the provider name and the resource name.
// If there is no provider prefix, an empty provider is returned.
//
// Resource names may optionally be prefixed with a provider with the form
// `provider/resource`.
// NOTE: copied from resource package to avoid circular imports.
func ParseName(n string) (string, string) {
	i := strings.IndexRune(n, '/')
	if i == -1 {
		// No provider prefix
		return "", n
	}
	return n[:i], n[i+1:]
}
