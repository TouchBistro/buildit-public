package firehose

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
)

// TestFirehoseNormalization verifies that normalization works correctly
// for the new requirements
func TestFirehoseNormalization(t *testing.T) {
	ctx := context.Background()
	providerName := "main"

	t.Run("Iceberg S3 Configuration CloudWatchLoggingOptions initialization", func(t *testing.T) {
		s3Config := FirehoseIcebergS3Configuration{
			// Leave Bucket and Role empty to avoid lookups
		}

		s3Config.normalize(ctx, providerName)

		// Should be initialized to empty struct
		assert.NotNil(t, s3Config.CloudWatchLoggingOptions)
		assert.Nil(t, s3Config.CloudWatchLoggingOptions.Enabled)
		assert.Nil(t, s3Config.CloudWatchLoggingOptions.LogGroupName)
		assert.Nil(t, s3Config.CloudWatchLoggingOptions.LogStreamName)
	})

	t.Run("Iceberg S3 Configuration EncryptionConfiguration initialization", func(t *testing.T) {
		s3Config := FirehoseIcebergS3Configuration{}

		s3Config.normalize(ctx, providerName)

		// Should be initialized with default NoEncryption
		assert.NotNil(t, s3Config.EncryptionConfiguration)
		assert.Equal(t, "NoEncryption", s3Config.EncryptionConfiguration.Type)
		assert.Nil(t, s3Config.EncryptionConfiguration.KMSKeyArn)
	})

	t.Run("ProcessingConfiguration default parameters", func(t *testing.T) {
		procConfig := &FirehoseProcessingConfiguration{
			Enabled: aws.Bool(true),
			Processors: []FirehoseProcessor{
				{
					Type: "Lambda",
					Parameters: map[string]string{
						"LambdaFunctionName": "test-function",
					},
				},
			},
		}

		procConfig.normalize(ctx, providerName, "arn:aws:iam::role:test")

		// Should have default parameters added
		assert.NotNil(t, procConfig.Processors[0].Parameters)
		assert.Contains(t, procConfig.Processors[0].Parameters, "RoleArn")
		assert.Contains(t, procConfig.Processors[0].Parameters, "BufferSizeInMBs")
		assert.Contains(t, procConfig.Processors[0].Parameters, "BufferIntervalInSeconds")
		assert.Contains(t, procConfig.Processors[0].Parameters, "NumberOfRetries")

		// Verify default values
		assert.Equal(t, procConfig.Processors[0].Parameters["RoleArn"], "arn:aws:iam::role:test")
		assert.Equal(t, procConfig.Processors[0].Parameters["BufferSizeInMBs"], "1")
		assert.Equal(t, procConfig.Processors[0].Parameters["BufferIntervalInSeconds"], "60")
		assert.Equal(t, procConfig.Processors[0].Parameters["NumberOfRetries"], "3")

		// Original parameter should still be there
		assert.Equal(t, procConfig.Processors[0].Parameters["LambdaFunctionName"], "test-function")
	})

	t.Run("ProcessingConfiguration with nil Parameters", func(t *testing.T) {
		procConfig := &FirehoseProcessingConfiguration{
			Enabled: aws.Bool(true),
			Processors: []FirehoseProcessor{
				{
					Type: "Lambda",
					// Parameters is nil
				},
			},
		}

		procConfig.normalize(ctx, providerName, "arn:aws:iam::role:test")

		// Should initialize Parameters and add default values
		assert.NotNil(t, procConfig.Processors[0].Parameters)
		assert.Contains(t, procConfig.Processors[0].Parameters, "RoleArn")
		assert.Contains(t, procConfig.Processors[0].Parameters, "BufferSizeInMBs")
		assert.Contains(t, procConfig.Processors[0].Parameters, "BufferIntervalInSeconds")
		assert.Contains(t, procConfig.Processors[0].Parameters, "NumberOfRetries")
	})

	t.Run("ProcessingConfiguration with existing default parameters", func(t *testing.T) {
		procConfig := &FirehoseProcessingConfiguration{
			Enabled: aws.Bool(true),
			Processors: []FirehoseProcessor{
				{
					Type: "Lambda",
					Parameters: map[string]string{
						"LambdaFunctionName": "test-function",
						"RoleArn":            "arn:aws:iam::role:test",
						"BufferSizeInMBs":    "10", // Custom value
					},
				},
			},
		}

		procConfig.normalize(ctx, providerName, "arn:aws:iam::role:test")

		// Should not override existing parameters
		assert.Equal(t, procConfig.Processors[0].Parameters["RoleArn"], "arn:aws:iam::role:test")
		assert.Equal(t, procConfig.Processors[0].Parameters["BufferSizeInMBs"], "10")

		// Should add missing default parameters
		assert.Contains(t, procConfig.Processors[0].Parameters, "BufferIntervalInSeconds")
		assert.Contains(t, procConfig.Processors[0].Parameters, "NumberOfRetries")
		assert.Equal(t, procConfig.Processors[0].Parameters["BufferIntervalInSeconds"], "60")
		assert.Equal(t, procConfig.Processors[0].Parameters["NumberOfRetries"], "3")
	})

	t.Run("ProcessingConfiguration with no processors", func(t *testing.T) {
		procConfig := &FirehoseProcessingConfiguration{
			Enabled:    aws.Bool(false),
			Processors: []FirehoseProcessor{}, // Empty processors
		}

		procConfig.normalize(ctx, providerName, "")

		// Should not add parameters if no processors exist
		assert.Equal(t, len(procConfig.Processors), 0)
	})

	t.Run("Iceberg S3 Configuration with zero BufferingIntervalSeconds", func(t *testing.T) {
		zeroSeconds := int32(0)
		s3Config := FirehoseIcebergS3Configuration{
			BufferingIntervalSeconds: &zeroSeconds, // Explicitly set to 0
		}

		s3Config.normalize(ctx, providerName)

		// Should be normalized to default value
		assert.NotNil(t, s3Config.BufferingIntervalSeconds)
		assert.Equal(t, defaultFirehoseBufferIntervalSeconds, *s3Config.BufferingIntervalSeconds)
	})

	t.Run("Iceberg S3 Configuration with zero BufferingSizeMBs", func(t *testing.T) {
		zeroMBs := int32(0)
		s3Config := FirehoseIcebergS3Configuration{
			BufferingSizeMBs: &zeroMBs, // Explicitly set to 0
		}

		s3Config.normalize(ctx, providerName)

		// Should be normalized to default value
		assert.NotNil(t, s3Config.BufferingSizeMBs)
		assert.Equal(t, defaultFirehoseBufferSizeMBs, *s3Config.BufferingSizeMBs)
	})
}
