package firehose

import (
	"github.com/TouchBistro/buildit/resource"
	"github.com/TouchBistro/buildit/util"
)

const (
	defaultFirehoseBufferIntervalSeconds int32 = 300
	defaultFirehoseBufferSizeMBs         int32 = 5

	firehoseEncryptionTypeKMS = "KMS"

	defaultIcebergRetryDurationSeconds int32  = 300
	defaultIcebergS3BackupMode         string = "FailedDataOnly"
)

// FirehoseDeliveryStream represents a Kinesis Data Firehose delivery stream backed by an Extended S3 destination.
type FirehoseDeliveryStream struct {
	resource.BaseResource `yaml:",inline"`
	Name               string                         `yaml:"-"`
	StreamType         string                         `yaml:"type"`
	Destination        *FirehoseExtendedS3Destination `yaml:"extendedS3Destination"`
	IcebergDestination *FirehoseIcebergDestination    `yaml:"icebergDestination"`
	DependsOn          []resource.Key                 `yaml:"dependsOn"`
	Tags               map[string]string              `yaml:"tags"`
	GlobalTags         map[string]string              `yaml:"-"`

	deliveryStreamVersionID *string // populated by fetchExisting
	destinationID           *string // populated by fetchExisting
}

// FirehoseExtendedS3Destination contains configuration for an Extended S3 destination.
// This supports all available Extended S3 destination options including Lambda transformations
type FirehoseExtendedS3Destination struct {
	Bucket                   string                           `yaml:"bucket"`
	BucketArn                *string                          `yaml:"-"`
	Role                     string                           `yaml:"role"`
	RoleArn                  *string                          `yaml:"-"`
	Prefix                   *string                          `yaml:"prefix"`
	ErrorOutputPrefix        *string                          `yaml:"errorOutputPrefix"`
	BufferingIntervalSeconds *int32                           `yaml:"bufferingIntervalSeconds"`
	BufferingSizeMBs         *int32                           `yaml:"bufferingSizeMBs"`
	CompressionFormat        *string                          `yaml:"compressionFormat"`
	CloudWatchLoggingOptions *FirehoseLoggingOptions          `yaml:"cloudWatchLogging"`
	ProcessingConfiguration  *FirehoseProcessingConfiguration `yaml:"processingConfiguration"`
	EncryptionConfiguration  *FirehoseEncryptionConfiguration `yaml:"encryptionConfiguration"`
}

// FirehoseIcebergDestination contains configuration for an Iceberg destination.
// This struct supports all available Iceberg destination options from the AWS SDK.
type FirehoseIcebergDestination struct {
	CatalogConfiguration          FirehoseIcebergCatalogConfiguration   `yaml:"catalogConfiguration"`
	Role                          string                                `yaml:"role"`
	RoleArn                       *string                               `yaml:"-"`
	S3Configuration               FirehoseIcebergS3Configuration        `yaml:"s3Configuration"`
	BufferingIntervalSeconds      *int32                                `yaml:"bufferingIntervalSeconds"`
	BufferingSizeMBs              *int32                                `yaml:"bufferingSizeMBs"`
	CloudWatchLoggingOptions      *FirehoseLoggingOptions               `yaml:"cloudWatchLogging"`
	DestinationTableConfiguration []FirehoseIcebergTableConfiguration   `yaml:"destinationTableConfiguration"`
	ProcessingConfiguration       *FirehoseProcessingConfiguration      `yaml:"processingConfiguration"`
	RetryOptions                  *FirehoseRetryOptions                 `yaml:"retryOptions"`
	S3BackupMode                  *string                               `yaml:"s3BackupMode"`
	S3BackupConfiguration         *FirehoseExtendedS3Destination        `yaml:"s3BackupConfiguration"`
	AppendOnly                    *bool                                 `yaml:"appendOnly"`
	SchemaEvolutionConfiguration  *FirehoseSchemaEvolutionConfiguration `yaml:"schemaEvolutionConfiguration"`
	TableCreationConfiguration    *FirehoseTableCreationConfiguration   `yaml:"tableCreationConfiguration"`
}

// FirehoseIcebergCatalogConfiguration contains configuration for the Iceberg catalog.
type FirehoseIcebergCatalogConfiguration struct {
	Catalog           string  `yaml:"catalog"`
	CatalogArn        *string `yaml:"-"`
	WarehouseLocation *string `yaml:"warehouseLocation"`
}

// FirehoseIcebergS3Configuration contains configuration for the S3 destination in Iceberg.
type FirehoseIcebergS3Configuration struct {
	Bucket                   string                           `yaml:"bucket"`
	BucketArn                *string                          `yaml:"-"`
	Role                     string                           `yaml:"role"`
	RoleArn                  *string                          `yaml:"-"`
	Prefix                   *string                          `yaml:"prefix"`
	ErrorOutputPrefix        *string                          `yaml:"errorOutputPrefix"`
	BufferingIntervalSeconds *int32                           `yaml:"bufferingIntervalSeconds"`
	BufferingSizeMBs         *int32                           `yaml:"bufferingSizeMBs"`
	CompressionFormat        *string                          `yaml:"compressionFormat"`
	CloudWatchLoggingOptions *FirehoseLoggingOptions          `yaml:"cloudWatchLogging"`
	EncryptionConfiguration  *FirehoseEncryptionConfiguration `yaml:"encryptionConfiguration"`
}

// FirehoseIcebergTableConfiguration contains configuration for destination tables.
type FirehoseIcebergTableConfiguration struct {
	DatabaseName        string   `yaml:"databaseName"`
	TableName           string   `yaml:"tableName"`
	S3ErrorOutputPrefix *string  `yaml:"s3ErrorOutputPrefix"`
	UniqueKeys          []string `yaml:"uniqueKeys"`
}

// FirehoseProcessingConfiguration contains configuration for data processing.
// This supports Lambda transformations and other processor types.
type FirehoseProcessingConfiguration struct {
	Enabled    *bool               `yaml:"enabled"`
	Processors []FirehoseProcessor `yaml:"processors"`
}

// FirehoseProcessor contains configuration for a data processor.
// For Lambda transformations, Type should be "Lambda" and Parameters should include:
// - RoleArn: IAM role ARN for Lambda execution
// - BufferSizeInMBs: Buffer size in MBs (default: 1)
// - BufferIntervalInSeconds: Buffer interval in seconds (default: 60)
// - NumberOfRetries: Number of retries for failed records (default: 3)
type FirehoseProcessor struct {
	Type       string            `yaml:"type"`
	Parameters map[string]string `yaml:"parameters"`
}

// FirehoseRetryOptions contains configuration for retries.
type FirehoseRetryOptions struct {
	DurationInSeconds *int32 `yaml:"durationInSeconds"`
}

// FirehoseLoggingOptions controls logging for the delivery stream.
type FirehoseLoggingOptions struct {
	Enabled       *bool   `yaml:"enabled"`
	LogGroupName  *string `yaml:"logGroupName"`
	LogStreamName *string `yaml:"logStreamName"`
}

// FirehoseEncryptionConfiguration controls server-side encryption for the delivery stream.
type FirehoseEncryptionConfiguration struct {
	Type      string  `yaml:"type"` // KMS | NoEncryption
	KMSKey    *string `yaml:"kmsKey"`
	KMSKeyArn *string `yaml:"-"`
}

// FirehoseSchemaEvolutionConfiguration contains configuration for schema evolution.
type FirehoseSchemaEvolutionConfiguration struct {
	Enabled *bool `yaml:"enabled"`
}

// FirehoseTableCreationConfiguration contains configuration for table creation.
type FirehoseTableCreationConfiguration struct {
	Enabled *bool `yaml:"enabled"`
}

// FirehoseDeliveryStreamDiff represents diffs between buildit definition & AWS representation.
type FirehoseDeliveryStreamDiff struct {
	resource.BaseResourceDiff

	destinationDiff bool
	tagsDiff        bool
	tagDiff         util.TagDiffResult
}
