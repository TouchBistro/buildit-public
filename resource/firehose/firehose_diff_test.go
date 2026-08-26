package firehose

import (
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
)

func TestDiffExtendedS3Destination(t *testing.T) {
	t.Run("nil comparisons", func(t *testing.T) {
		assert.Empty(t, diffExtendedS3Destination(nil, nil))
		assert.Equal(t, []string{"ExtendedS3Destination: nil -> configured"}, diffExtendedS3Destination(nil, &FirehoseExtendedS3Destination{}))
		assert.Equal(t, []string{"ExtendedS3Destination: configured -> nil"}, diffExtendedS3Destination(&FirehoseExtendedS3Destination{}, nil))
	})

	t.Run("no differences", func(t *testing.T) {
		d1 := &FirehoseExtendedS3Destination{
			BucketArn: aws.String("arn:aws:s3:::bucket"),
			RoleArn:   aws.String("arn:aws:iam::role"),
		}
		d2 := &FirehoseExtendedS3Destination{
			BucketArn: aws.String("arn:aws:s3:::bucket"),
			RoleArn:   aws.String("arn:aws:iam::role"),
		}
		assert.Empty(t, diffExtendedS3Destination(d1, d2))
	})

	t.Run("differences", func(t *testing.T) {
		d1 := &FirehoseExtendedS3Destination{
			BucketArn:        aws.String("arn:aws:s3:::bucket1"),
			RoleArn:          aws.String("arn:aws:iam::role1"),
			BufferingSizeMBs: aws.Int32(5),
		}
		d2 := &FirehoseExtendedS3Destination{
			BucketArn:        aws.String("arn:aws:s3:::bucket2"),
			RoleArn:          aws.String("arn:aws:iam::role1"),
			BufferingSizeMBs: aws.Int32(10),
		}

		diffs := diffExtendedS3Destination(d1, d2)
		assert.Contains(t, diffs, fmt.Sprintf("ExtendedS3Destination.Bucket: %q -> %q", "arn:aws:s3:::bucket1", "arn:aws:s3:::bucket2"))
		assert.Contains(t, diffs, "ExtendedS3Destination.BufferingSizeMBs: 5 -> 10")
	})
}

func TestDiffIcebergDestination(t *testing.T) {
	t.Run("differences", func(t *testing.T) {
		d1 := &FirehoseIcebergDestination{
			CatalogConfiguration: FirehoseIcebergCatalogConfiguration{CatalogArn: aws.String("arn1")},
			RoleArn:              aws.String("arn:aws:iam::role1"),
			S3Configuration:      FirehoseIcebergS3Configuration{BucketArn: aws.String("arn:aws:s3:::bucket1"), RoleArn: aws.String("arn:aws:iam::role1")},
		}
		d2 := &FirehoseIcebergDestination{
			CatalogConfiguration: FirehoseIcebergCatalogConfiguration{CatalogArn: aws.String("arn2")},
			RoleArn:              aws.String("arn:aws:iam::role1"),
			S3Configuration:      FirehoseIcebergS3Configuration{BucketArn: aws.String("arn:aws:s3:::bucket1"), RoleArn: aws.String("arn:aws:iam::role1"), Prefix: aws.String("new_prefix")},
		}

		diffs := diffIcebergDestination(d1, d2)
		assert.Contains(t, diffs, fmt.Sprintf("IcebergDestination.CatalogConfiguration.Catalog: %q -> %q", "arn1", "arn2"))
		assert.Contains(t, diffs, fmt.Sprintf("IcebergDestination.S3Configuration.Prefix: %q -> %q", "", "new_prefix"))
	})

	t.Run("detailed child structure changes", func(t *testing.T) {
		d1 := &FirehoseIcebergDestination{
			CatalogConfiguration: FirehoseIcebergCatalogConfiguration{CatalogArn: aws.String("arn1")},
			RoleArn:              aws.String("arn:aws:iam::role1"),
			S3Configuration: FirehoseIcebergS3Configuration{
				BucketArn: aws.String("arn:aws:s3:::bucket1"),
				RoleArn:   aws.String("arn:aws:iam::role1"),
				CloudWatchLoggingOptions: &FirehoseLoggingOptions{
					Enabled:       aws.Bool(false),
					LogGroupName:  aws.String("group1"),
					LogStreamName: aws.String("stream1"),
				},
				EncryptionConfiguration: &FirehoseEncryptionConfiguration{
					Type: "NoEncryption",
				},
			},
			CloudWatchLoggingOptions: &FirehoseLoggingOptions{
				Enabled:       aws.Bool(false),
				LogGroupName:  aws.String("main_group1"),
				LogStreamName: aws.String("main_stream1"),
			},
			ProcessingConfiguration: &FirehoseProcessingConfiguration{
				Enabled: aws.Bool(true),
				Processors: []FirehoseProcessor{
					{
						Type: "Lambda",
						Parameters: map[string]string{
							"LambdaFunctionName": "func1",
						},
					},
				},
			},
		}
		d2 := &FirehoseIcebergDestination{
			CatalogConfiguration: FirehoseIcebergCatalogConfiguration{CatalogArn: aws.String("arn1")},
			RoleArn:              aws.String("arn:aws:iam::role1"),
			S3Configuration: FirehoseIcebergS3Configuration{
				BucketArn: aws.String("arn:aws:s3:::bucket1"),
				RoleArn:   aws.String("arn:aws:iam::role1"),
				CloudWatchLoggingOptions: &FirehoseLoggingOptions{
					Enabled:       aws.Bool(true),       // Changed
					LogGroupName:  aws.String("group2"), // Changed
					LogStreamName: aws.String("stream1"),
				},
				EncryptionConfiguration: &FirehoseEncryptionConfiguration{
					Type:   "KMS", // Changed
					KMSKey: aws.String("arn:aws:kms:us-east-1:123456789012:key/12345678-1234-1234-1234-123456789012"),
				},
			},
			CloudWatchLoggingOptions: &FirehoseLoggingOptions{
				Enabled:       aws.Bool(true),            // Changed
				LogGroupName:  aws.String("main_group2"), // Changed
				LogStreamName: aws.String("main_stream1"),
			},
			ProcessingConfiguration: &FirehoseProcessingConfiguration{
				Enabled: aws.Bool(true),
				Processors: []FirehoseProcessor{
					{
						Type: "Lambda",
						Parameters: map[string]string{
							"LambdaFunctionName": "func2", // Changed
							"BatchSize":          "100",   // Added
						},
					},
				},
			},
		}

		diffs := diffIcebergDestination(d1, d2)

		// Test detailed S3 logging changes
		assert.Contains(t, diffs, "IcebergDestination.S3Configuration.CloudWatchLoggingOptions.Enabled: false -> true")
		assert.Contains(t, diffs, `IcebergDestination.S3Configuration.CloudWatchLoggingOptions.LogGroupName: "group1" -> "group2"`)

		// Test detailed S3 encryption changes
		assert.Contains(t, diffs, `IcebergDestination.S3Configuration.EncryptionConfiguration.Type: "NoEncryption" -> "KMS"`)
		assert.Contains(t, diffs, `IcebergDestination.S3Configuration.EncryptionConfiguration.KMSKey: "" -> "arn:aws:kms:us-east-1:123456789012:key/12345678-1234-1234-1234-123456789012"`)

		// Test detailed main logging changes
		assert.Contains(t, diffs, "IcebergDestination.CloudWatchLoggingOptions.Enabled: false -> true")
		assert.Contains(t, diffs, `IcebergDestination.CloudWatchLoggingOptions.LogGroupName: "main_group1" -> "main_group2"`)

		// Test detailed processing changes
		assert.Contains(t, diffs, `IcebergDestination.ProcessingConfiguration.Processors[0].Parameters[LambdaFunctionName]: "func1" -> "func2"`)
		assert.Contains(t, diffs, "IcebergDestination.ProcessingConfiguration.Processors[0].Parameters[BatchSize]: added")
	})
}
