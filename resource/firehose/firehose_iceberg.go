package firehose

import (
	"context"
	"fmt"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/aws/aws-sdk-go-v2/aws"
	firehosetypes "github.com/aws/aws-sdk-go-v2/service/firehose/types"
)

func (d *FirehoseIcebergDestination) normalize(ctx context.Context, providerName string) {
	if d.Role != "" {
		arn, err := awsw.NewIAM(ctx, providerName).RoleArnForName(ctx, d.Role)
		if err != nil {
			panic(err)
		}
		d.RoleArn = arn
	}

	if d.CatalogConfiguration.Catalog != "" {
		arn, err := awsw.NewGlue(ctx, providerName).CatalogArnForIdentifier(ctx, d.CatalogConfiguration.Catalog)
		if err != nil {
			panic(err)
		}
		d.CatalogConfiguration.CatalogArn = arn
	}

	if d.BufferingIntervalSeconds == nil {
		d.BufferingIntervalSeconds = aws.Int32(defaultFirehoseBufferIntervalSeconds)
	}

	if d.BufferingSizeMBs == nil {
		d.BufferingSizeMBs = aws.Int32(defaultFirehoseBufferSizeMBs)
	}

	if d.CloudWatchLoggingOptions == nil {
		d.CloudWatchLoggingOptions = &FirehoseLoggingOptions{
			Enabled: aws.Bool(false),
		}
	} else {
		d.CloudWatchLoggingOptions.normalize()
	}

	// Normalize S3Configuration to ensure buffering values and other defaults are set
	d.S3Configuration.normalize(ctx, providerName)

	if d.ProcessingConfiguration != nil {
		d.ProcessingConfiguration.normalize(ctx, providerName, aws.ToString(d.RoleArn))
	}

	if d.RetryOptions == nil {
		d.RetryOptions = &FirehoseRetryOptions{
			DurationInSeconds: aws.Int32(defaultIcebergRetryDurationSeconds),
		}
	}

	if d.S3BackupMode == nil {
		d.S3BackupMode = aws.String(defaultIcebergS3BackupMode)
	}

	if d.S3BackupConfiguration != nil {
		d.S3BackupConfiguration.normalize(ctx, providerName)
	}

	if d.AppendOnly == nil {
		d.AppendOnly = aws.Bool(false)
	}

	if d.SchemaEvolutionConfiguration != nil {
		if d.SchemaEvolutionConfiguration.Enabled == nil {
			d.SchemaEvolutionConfiguration.Enabled = aws.Bool(false)
		}
	}

	if d.TableCreationConfiguration != nil {
		if d.TableCreationConfiguration.Enabled == nil {
			d.TableCreationConfiguration.Enabled = aws.Bool(false)
		}
	}
}

// validate performs comprehensive validation of Iceberg destination configuration.
// This includes checking required fields, value ranges, and configuration consistency.
func (d FirehoseIcebergDestination) validate() []string {
	var errorMsgs []string

	if d.CatalogConfiguration.Catalog == "" {
		errorMsgs = append(errorMsgs, "Firehose Iceberg destination catalog is required")
	}

	if d.Role == "" {
		errorMsgs = append(errorMsgs, "Firehose Iceberg destination IAM role is required")
	}

	errorMsgs = append(errorMsgs, d.S3Configuration.validate()...)

	if d.BufferingIntervalSeconds != nil {
		if *d.BufferingIntervalSeconds < 60 || *d.BufferingIntervalSeconds > 900 {
			errorMsgs = append(errorMsgs, "icebergDestination.bufferingIntervalSeconds must be between 60 and 900")
		}
	}

	if d.BufferingSizeMBs != nil {
		if *d.BufferingSizeMBs < 1 || *d.BufferingSizeMBs > 128 {
			errorMsgs = append(errorMsgs, "icebergDestination.bufferingSizeMBs must be between 1 and 128")
		}
	}

	if d.CloudWatchLoggingOptions != nil {
		errorMsgs = append(errorMsgs, d.CloudWatchLoggingOptions.validate()...)
	}

	if d.ProcessingConfiguration != nil {
		errorMsgs = append(errorMsgs, d.ProcessingConfiguration.validate()...)
	}

	if d.S3BackupConfiguration != nil {
		errorMsgs = append(errorMsgs, d.S3BackupConfiguration.validate()...)
	}

	// Validate S3BackupMode if provided
	if d.S3BackupMode != nil {
		supportedModes := map[string]bool{
			"AllData":        true,
			"FailedDataOnly": true,
			"Disabled":       true,
		}
		if !supportedModes[aws.ToString(d.S3BackupMode)] {
			errorMsgs = append(errorMsgs, "icebergDestination.s3BackupMode must be one of: AllData, FailedDataOnly, Disabled")
		}
	}

	// Validate DestinationTableConfiguration
	if len(d.DestinationTableConfiguration) == 0 {
		errorMsgs = append(errorMsgs, "icebergDestination.destinationTableConfiguration must contain at least one table")
	} else {
		for i, table := range d.DestinationTableConfiguration {
			if table.DatabaseName == "" {
				errorMsgs = append(errorMsgs, fmt.Sprintf("Firehose Iceberg destination table configuration [%d] database name is required", i))
			}
			if table.TableName == "" {
				errorMsgs = append(errorMsgs, fmt.Sprintf("Firehose Iceberg destination table configuration [%d] table name is required", i))
			}
		}
	}

	return errorMsgs
}

func (d FirehoseIcebergDestination) toAWSConfiguration() *firehosetypes.IcebergDestinationConfiguration {
	cfg := &firehosetypes.IcebergDestinationConfiguration{
		CatalogConfiguration: &firehosetypes.CatalogConfiguration{
			CatalogARN:        d.CatalogConfiguration.CatalogArn,
			WarehouseLocation: d.CatalogConfiguration.WarehouseLocation,
		},
		RoleARN: d.RoleArn,
		S3Configuration: &firehosetypes.S3DestinationConfiguration{
			BucketARN:         d.S3Configuration.BucketArn,
			RoleARN:           d.S3Configuration.RoleArn,
			Prefix:            d.S3Configuration.Prefix,
			ErrorOutputPrefix: d.S3Configuration.ErrorOutputPrefix,
		},
		BufferingHints: &firehosetypes.BufferingHints{
			IntervalInSeconds: d.BufferingIntervalSeconds,
			SizeInMBs:         d.BufferingSizeMBs,
		},
		CloudWatchLoggingOptions: d.CloudWatchLoggingOptions.toAWS(),
		ProcessingConfiguration:  d.ProcessingConfiguration.toAWS(),
		RetryOptions: &firehosetypes.RetryOptions{
			DurationInSeconds: d.RetryOptions.DurationInSeconds,
		},
		S3BackupMode: firehosetypes.IcebergS3BackupMode(aws.ToString(d.S3BackupMode)),
		AppendOnly:   d.AppendOnly,
	}

	// Note: S3BackupConfiguration is handled through the S3BackupMode field
	// The actual backup configuration uses the same S3Configuration for backup purposes
	// when S3BackupMode is set to AllData or FailedDataOnly

	var tables []firehosetypes.DestinationTableConfiguration
	for _, t := range d.DestinationTableConfiguration {
		tables = append(tables, firehosetypes.DestinationTableConfiguration{
			DestinationDatabaseName: aws.String(t.DatabaseName),
			DestinationTableName:    aws.String(t.TableName),
			S3ErrorOutputPrefix:     t.S3ErrorOutputPrefix,
			UniqueKeys:              t.UniqueKeys,
		})
	}
	cfg.DestinationTableConfigurationList = tables

	return cfg
}

func (d FirehoseIcebergDestination) toAWSUpdate() *firehosetypes.IcebergDestinationUpdate {
	upd := &firehosetypes.IcebergDestinationUpdate{
		CatalogConfiguration: &firehosetypes.CatalogConfiguration{
			CatalogARN:        d.CatalogConfiguration.CatalogArn,
			WarehouseLocation: d.CatalogConfiguration.WarehouseLocation,
		},
		RoleARN: d.RoleArn,
		S3Configuration: &firehosetypes.S3DestinationConfiguration{
			BucketARN:         d.S3Configuration.BucketArn,
			RoleARN:           d.S3Configuration.RoleArn,
			Prefix:            d.S3Configuration.Prefix,
			ErrorOutputPrefix: d.S3Configuration.ErrorOutputPrefix,
		},
		BufferingHints: &firehosetypes.BufferingHints{
			IntervalInSeconds: d.BufferingIntervalSeconds,
			SizeInMBs:         d.BufferingSizeMBs,
		},
		CloudWatchLoggingOptions: d.CloudWatchLoggingOptions.toAWS(),
		ProcessingConfiguration:  d.ProcessingConfiguration.toAWS(),
		RetryOptions: &firehosetypes.RetryOptions{
			DurationInSeconds: d.RetryOptions.DurationInSeconds,
		},
		S3BackupMode: firehosetypes.IcebergS3BackupMode(aws.ToString(d.S3BackupMode)),
		AppendOnly:   d.AppendOnly,
	}

	// Note: S3BackupConfiguration is handled through the S3BackupMode field
	// The actual backup configuration uses the same S3Configuration for backup purposes
	// when S3BackupMode is set to AllData or FailedDataOnly

	var tables []firehosetypes.DestinationTableConfiguration
	for _, t := range d.DestinationTableConfiguration {
		tables = append(tables, firehosetypes.DestinationTableConfiguration{
			DestinationDatabaseName: aws.String(t.DatabaseName),
			DestinationTableName:    aws.String(t.TableName),
			S3ErrorOutputPrefix:     t.S3ErrorOutputPrefix,
			UniqueKeys:              t.UniqueKeys,
		})
	}
	upd.DestinationTableConfigurationList = tables

	return upd
}

func icebergDestinationFromDescription(desc *firehosetypes.IcebergDestinationDescription) *FirehoseIcebergDestination {
	if desc == nil {
		return nil
	}

	dest := &FirehoseIcebergDestination{
		CatalogConfiguration: FirehoseIcebergCatalogConfiguration{
			CatalogArn:        desc.CatalogConfiguration.CatalogARN,
			WarehouseLocation: desc.CatalogConfiguration.WarehouseLocation,
		},
		RoleArn:                  desc.RoleARN,
		S3Configuration:          icebergS3FromDescription(desc.S3DestinationDescription),
		BufferingIntervalSeconds: nil,
		BufferingSizeMBs:         nil,
		CloudWatchLoggingOptions: loggingFromDescription(desc.CloudWatchLoggingOptions),
		ProcessingConfiguration:  processingFromDescription(desc.ProcessingConfiguration),
		RetryOptions: &FirehoseRetryOptions{
			DurationInSeconds: desc.RetryOptions.DurationInSeconds,
		},
		S3BackupMode: aws.String(string(desc.S3BackupMode)),
		AppendOnly:   desc.AppendOnly,
	}

	if desc.SchemaEvolutionConfiguration != nil {
		dest.SchemaEvolutionConfiguration = &FirehoseSchemaEvolutionConfiguration{
			Enabled: desc.SchemaEvolutionConfiguration.Enabled,
		}
	}

	if desc.TableCreationConfiguration != nil {
		dest.TableCreationConfiguration = &FirehoseTableCreationConfiguration{
			Enabled: desc.TableCreationConfiguration.Enabled,
		}
	}

	if desc.BufferingHints != nil {
		dest.BufferingIntervalSeconds = desc.BufferingHints.IntervalInSeconds
		dest.BufferingSizeMBs = desc.BufferingHints.SizeInMBs
	}

	var tables []FirehoseIcebergTableConfiguration
	for _, t := range desc.DestinationTableConfigurationList {
		tables = append(tables, FirehoseIcebergTableConfiguration{
			DatabaseName:        aws.ToString(t.DestinationDatabaseName),
			TableName:           aws.ToString(t.DestinationTableName),
			S3ErrorOutputPrefix: t.S3ErrorOutputPrefix,
			UniqueKeys:          t.UniqueKeys,
		})
	}
	dest.DestinationTableConfiguration = tables

	return dest
}

func (s *FirehoseIcebergS3Configuration) normalize(ctx context.Context, providerName string) {
	if s.Bucket != "" {
		arn, err := awsw.NewS3(ctx, providerName).BucketArnForIdentifier(ctx, s.Bucket)
		if err != nil {
			panic(err)
		}
		s.BucketArn = arn
	}

	if s.Role != "" {
		arn, err := awsw.NewIAM(ctx, providerName).RoleArnForName(ctx, s.Role)
		if err != nil {
			panic(err)
		}
		s.RoleArn = arn
	}

	// Handle BufferingIntervalSeconds - set to default if nil or 0
	if s.BufferingIntervalSeconds == nil || (s.BufferingIntervalSeconds != nil && *s.BufferingIntervalSeconds == 0) {
		s.BufferingIntervalSeconds = aws.Int32(defaultFirehoseBufferIntervalSeconds)
	}

	// Handle BufferingSizeMBs - set to default if nil or 0
	if s.BufferingSizeMBs == nil || (s.BufferingSizeMBs != nil && *s.BufferingSizeMBs == 0) {
		s.BufferingSizeMBs = aws.Int32(defaultFirehoseBufferSizeMBs)
	}

	if s.CompressionFormat == nil || len(*s.CompressionFormat) == 0 {
		format := string(firehosetypes.CompressionFormatUncompressed)
		s.CompressionFormat = &format
	} else {
		format := string(normalizeFirehoseCompression(*s.CompressionFormat))
		s.CompressionFormat = aws.String(format)
	}

	if s.CloudWatchLoggingOptions == nil {
		s.CloudWatchLoggingOptions = &FirehoseLoggingOptions{}
	} else {
		s.CloudWatchLoggingOptions.normalize()
	}

	if s.EncryptionConfiguration == nil {
		s.EncryptionConfiguration = &FirehoseEncryptionConfiguration{}
	}
	s.EncryptionConfiguration.normalize(ctx, providerName)
}

func (s FirehoseIcebergS3Configuration) validate() []string {
	var errorMsgs []string

	if s.Bucket == "" {
		errorMsgs = append(errorMsgs, "Firehose Iceberg destination S3 bucket is required")
	}

	if s.Role == "" {
		errorMsgs = append(errorMsgs, "Firehose Iceberg destination S3 role is required")
	}

	if s.BufferingIntervalSeconds != nil {
		if *s.BufferingIntervalSeconds < 60 || *s.BufferingIntervalSeconds > 900 {
			errorMsgs = append(errorMsgs, "icebergDestination.s3Configuration.bufferingIntervalSeconds must be between 60 and 900")
		}
	}

	if s.BufferingSizeMBs != nil {
		if *s.BufferingSizeMBs < 1 || *s.BufferingSizeMBs > 128 {
			errorMsgs = append(errorMsgs, "icebergDestination.s3Configuration.bufferingSizeMBs must be between 1 and 128")
		}
	}

	if s.CompressionFormat != nil {
		if !isSupportedCompression(*s.CompressionFormat) {
			errorMsgs = append(errorMsgs, fmt.Sprintf("unsupported compression format %q", *s.CompressionFormat))
		}
	}

	if s.CloudWatchLoggingOptions != nil {
		errorMsgs = append(errorMsgs, s.CloudWatchLoggingOptions.validate()...)
	}

	if s.EncryptionConfiguration != nil {
		errorMsgs = append(errorMsgs, s.EncryptionConfiguration.validate()...)
	}

	return errorMsgs
}

func icebergS3FromDescription(desc *firehosetypes.S3DestinationDescription) FirehoseIcebergS3Configuration {
	dest := FirehoseIcebergS3Configuration{
		BucketArn:                desc.BucketARN,
		RoleArn:                  desc.RoleARN,
		Prefix:                   desc.Prefix,
		ErrorOutputPrefix:        desc.ErrorOutputPrefix,
		BufferingIntervalSeconds: nil,
		BufferingSizeMBs:         nil,
		CompressionFormat:        aws.String(string(desc.CompressionFormat)),
		CloudWatchLoggingOptions: loggingFromDescription(desc.CloudWatchLoggingOptions),
		EncryptionConfiguration:  encryptionFromDescription(desc.EncryptionConfiguration),
	}

	if desc.BufferingHints != nil {
		dest.BufferingIntervalSeconds = desc.BufferingHints.IntervalInSeconds
		dest.BufferingSizeMBs = desc.BufferingHints.SizeInMBs
	}

	return dest
}
