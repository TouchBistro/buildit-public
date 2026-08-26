package firehose

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
)

func TestFirehoseIcebergDestination_Normalize(t *testing.T) {
	ctx := context.Background()
	providerName := "main"

	t.Run("sets defaults", func(t *testing.T) {
		d := &FirehoseIcebergDestination{}
		d.normalize(ctx, providerName)

		assert.NotNil(t, d.BufferingIntervalSeconds)
		assert.Equal(t, int32(300), *d.BufferingIntervalSeconds)
		assert.NotNil(t, d.BufferingSizeMBs)
		assert.Equal(t, int32(5), *d.BufferingSizeMBs)
		assert.NotNil(t, d.CloudWatchLoggingOptions)
		assert.False(t, *d.CloudWatchLoggingOptions.Enabled)
		assert.NotNil(t, d.RetryOptions)
		assert.Equal(t, int32(300), *d.RetryOptions.DurationInSeconds)
		assert.NotNil(t, d.S3BackupMode)
		assert.Equal(t, "FailedDataOnly", *d.S3BackupMode)
		assert.NotNil(t, d.AppendOnly)
		assert.False(t, *d.AppendOnly)
	})
}

func TestFirehoseIcebergDestination_Validate(t *testing.T) {
	t.Run("valid configuration", func(t *testing.T) {
		d := FirehoseIcebergDestination{
			CatalogConfiguration: FirehoseIcebergCatalogConfiguration{
				Catalog: "arn:aws:glue:us-east-1:123456789012:catalog",
			},
			Role: "arn:aws:iam::123456789012:role/firehose_role",
			S3Configuration: FirehoseIcebergS3Configuration{
				Bucket: "arn:aws:s3:::my-bucket",
				Role:   "arn:aws:iam::123456789012:role/s3_role",
			},
			DestinationTableConfiguration: []FirehoseIcebergTableConfiguration{
				{
					DatabaseName: "test_db",
					TableName:    "test_table",
				},
			},
		}
		// Skip normalize here because it triggers AWS client lookups
		errs := d.validate()
		assert.Empty(t, errs)
	})

	t.Run("missing required fields", func(t *testing.T) {
		d := FirehoseIcebergDestination{}
		errs := d.validate()
		assert.Contains(t, errs, "Firehose Iceberg destination catalog is required")
		assert.Contains(t, errs, "Firehose Iceberg destination IAM role is required")
		assert.Contains(t, errs, "Firehose Iceberg destination S3 bucket is required")
		assert.Contains(t, errs, "Firehose Iceberg destination S3 role is required")
	})

	t.Run("invalid buffering hints", func(t *testing.T) {
		d := FirehoseIcebergDestination{
			CatalogConfiguration:     FirehoseIcebergCatalogConfiguration{Catalog: "cat"},
			Role:                     "role",
			S3Configuration:          FirehoseIcebergS3Configuration{Bucket: "bucket", Role: "role"},
			BufferingIntervalSeconds: aws.Int32(10),
			BufferingSizeMBs:         aws.Int32(200),
		}
		errs := d.validate()
		assert.Contains(t, errs, "icebergDestination.bufferingIntervalSeconds must be between 60 and 900")
		assert.Contains(t, errs, "icebergDestination.bufferingSizeMBs must be between 1 and 128")
	})
}
