package firehose

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
)

// TestDetailedFirehoseDiffs verifies that detailed diffs are shown for child structs
// instead of generic "changed" messages
func TestDetailedFirehoseDiffs(t *testing.T) {
	t.Run("Iceberg S3 Configuration CloudWatch Logging detailed diffs", func(t *testing.T) {
		s3Config1 := FirehoseIcebergS3Configuration{
			BucketArn: aws.String("arn:aws:s3:::bucket1"),
			RoleArn:   aws.String("arn:aws:iam::role1"),
			CloudWatchLoggingOptions: &FirehoseLoggingOptions{
				Enabled:       aws.Bool(false),
				LogGroupName:  aws.String("group1"),
				LogStreamName: aws.String("stream1"),
			},
		}
		s3Config2 := FirehoseIcebergS3Configuration{
			BucketArn: aws.String("arn:aws:s3:::bucket1"),
			RoleArn:   aws.String("arn:aws:iam::role1"),
			CloudWatchLoggingOptions: &FirehoseLoggingOptions{
				Enabled:       aws.Bool(true),
				LogGroupName:  aws.String("group2"),
				LogStreamName: aws.String("stream1"),
			},
		}

		diffs := diffIcebergS3Configuration(&s3Config1, &s3Config2)

		// Should show detailed field changes, not generic "changed" messages
		assert.Contains(t, diffs, "IcebergDestination.S3Configuration.CloudWatchLoggingOptions.Enabled: false -> true")
		assert.Contains(t, diffs, `IcebergDestination.S3Configuration.CloudWatchLoggingOptions.LogGroupName: "group1" -> "group2"`)

		// Should NOT contain generic "changed" messages
		for _, diff := range diffs {
			assert.NotContains(t, diff, "IcebergDestination.S3Configuration.CloudWatchLoggingOptions changed")
		}
	})

	t.Run("Iceberg S3 Configuration Encryption detailed diffs", func(t *testing.T) {
		s3Config1 := FirehoseIcebergS3Configuration{
			BucketArn: aws.String("arn:aws:s3:::bucket1"),
			RoleArn:   aws.String("arn:aws:iam::role1"),
			EncryptionConfiguration: &FirehoseEncryptionConfiguration{
				Type: "NoEncryption",
			},
		}
		s3Config2 := FirehoseIcebergS3Configuration{
			BucketArn: aws.String("arn:aws:s3:::bucket1"),
			RoleArn:   aws.String("arn:aws:iam::role1"),
			EncryptionConfiguration: &FirehoseEncryptionConfiguration{
				Type:   "KMS",
				KMSKey: aws.String("arn:aws:kms:us-east-1:123456789012:key/12345678-1234-1234-1234-123456789012"),
			},
		}

		diffs := diffIcebergS3Configuration(&s3Config1, &s3Config2)

		// Should show detailed field changes, not generic "changed" messages
		assert.Contains(t, diffs, `IcebergDestination.S3Configuration.EncryptionConfiguration.Type: "NoEncryption" -> "KMS"`)
		assert.Contains(t, diffs, `IcebergDestination.S3Configuration.EncryptionConfiguration.KMSKey: "" -> "arn:aws:kms:us-east-1:123456789012:key/12345678-1234-1234-1234-123456789012"`)

		// Should NOT contain generic "changed" messages
		for _, diff := range diffs {
			assert.NotContains(t, diff, "IcebergDestination.S3Configuration.EncryptionConfiguration changed")
		}
	})

	t.Run("Iceberg Destination Processing Configuration detailed diffs", func(t *testing.T) {
		dest1 := &FirehoseIcebergDestination{
			CatalogConfiguration: FirehoseIcebergCatalogConfiguration{CatalogArn: aws.String("arn1")},
			RoleArn:              aws.String("arn:aws:iam::role1"),
			S3Configuration:      FirehoseIcebergS3Configuration{BucketArn: aws.String("arn:aws:s3:::bucket1"), RoleArn: aws.String("arn:aws:iam::role1")},
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
		dest2 := &FirehoseIcebergDestination{
			CatalogConfiguration: FirehoseIcebergCatalogConfiguration{CatalogArn: aws.String("arn1")},
			RoleArn:              aws.String("arn:aws:iam::role1"),
			S3Configuration:      FirehoseIcebergS3Configuration{BucketArn: aws.String("arn:aws:s3:::bucket1"), RoleArn: aws.String("arn:aws:iam::role1")},
			ProcessingConfiguration: &FirehoseProcessingConfiguration{
				Enabled: aws.Bool(true),
				Processors: []FirehoseProcessor{
					{
						Type: "Lambda",
						Parameters: map[string]string{
							"LambdaFunctionName": "func2",
							"BatchSize":          "100",
						},
					},
				},
			},
		}

		diffs := diffIcebergDestination(dest1, dest2)

		// Should show detailed parameter changes
		assert.Contains(t, diffs, `IcebergDestination.ProcessingConfiguration.Processors[0].Parameters[LambdaFunctionName]: "func1" -> "func2"`)
		assert.Contains(t, diffs, "IcebergDestination.ProcessingConfiguration.Processors[0].Parameters[BatchSize]: added")

		// Should NOT contain generic "changed" messages
		for _, diff := range diffs {
			assert.NotContains(t, diff, "IcebergDestination.ProcessingConfiguration changed")
		}
	})
}
