package firehose

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/TouchBistro/buildit/util"
)

func diffExtendedS3Destination(existing, new *FirehoseExtendedS3Destination) []string {
	var diffs []string
	if existing == nil && new == nil {
		return diffs
	}
	if existing == nil {
		return []string{"ExtendedS3Destination: nil -> configured"}
	}
	if new == nil {
		return []string{"ExtendedS3Destination: configured -> nil"}
	}

	if util.Coalesce(existing.BucketArn, "") != util.Coalesce(new.BucketArn, "") {
		diffs = append(diffs, fmt.Sprintf("ExtendedS3Destination.Bucket: %q -> %q", util.Coalesce(existing.BucketArn, ""), util.Coalesce(new.BucketArn, "")))
	}
	if util.Coalesce(existing.RoleArn, "") != util.Coalesce(new.RoleArn, "") {
		diffs = append(diffs, fmt.Sprintf("ExtendedS3Destination.Role: %q -> %q", util.Coalesce(existing.RoleArn, ""), util.Coalesce(new.RoleArn, "")))
	}
	if !util.StringPtrEquals(existing.Prefix, new.Prefix) {
		diffs = append(diffs, fmt.Sprintf("ExtendedS3Destination.Prefix: %q -> %q", aws.ToString(existing.Prefix), aws.ToString(new.Prefix)))
	}
	if !util.StringPtrEquals(existing.ErrorOutputPrefix, new.ErrorOutputPrefix) {
		diffs = append(diffs, fmt.Sprintf("ExtendedS3Destination.ErrorOutputPrefix: %q -> %q", aws.ToString(existing.ErrorOutputPrefix), aws.ToString(new.ErrorOutputPrefix)))
	}
	if !util.Int32PtrEquals(existing.BufferingIntervalSeconds, new.BufferingIntervalSeconds) {
		diffs = append(diffs, fmt.Sprintf("ExtendedS3Destination.BufferingIntervalSeconds: %d -> %d", aws.ToInt32(existing.BufferingIntervalSeconds), aws.ToInt32(new.BufferingIntervalSeconds)))
	}
	if !util.Int32PtrEquals(existing.BufferingSizeMBs, new.BufferingSizeMBs) {
		diffs = append(diffs, fmt.Sprintf("ExtendedS3Destination.BufferingSizeMBs: %d -> %d", aws.ToInt32(existing.BufferingSizeMBs), aws.ToInt32(new.BufferingSizeMBs)))
	}
	if normalizeFirehoseCompression(aws.ToString(existing.CompressionFormat)) != normalizeFirehoseCompression(aws.ToString(new.CompressionFormat)) {
		diffs = append(diffs, fmt.Sprintf("ExtendedS3Destination.CompressionFormat: %q -> %q", aws.ToString(existing.CompressionFormat), aws.ToString(new.CompressionFormat)))
	}
	diffs = append(diffs, diffExtendedS3Logging(existing.CloudWatchLoggingOptions, new.CloudWatchLoggingOptions)...)
	diffs = append(diffs, diffExtendedS3Processing(existing.ProcessingConfiguration, new.ProcessingConfiguration)...)
	diffs = append(diffs, diffExtendedS3Encryption(existing.EncryptionConfiguration, new.EncryptionConfiguration)...)

	return diffs
}

func diffExtendedS3Logging(existing, new *FirehoseLoggingOptions) []string {
	var diffs []string
	if existing == nil && new == nil {
		return diffs
	}
	if existing == nil {
		return []string{"ExtendedS3Destination.CloudWatchLoggingOptions: nil -> configured"}
	}
	if new == nil {
		return []string{"ExtendedS3Destination.CloudWatchLoggingOptions: configured -> nil"}
	}

	if aws.ToBool(existing.Enabled) != aws.ToBool(new.Enabled) {
		diffs = append(diffs, fmt.Sprintf("ExtendedS3Destination.CloudWatchLoggingOptions.Enabled: %v -> %v", aws.ToBool(existing.Enabled), aws.ToBool(new.Enabled)))
	}
	if !util.StringPtrEquals(existing.LogGroupName, new.LogGroupName) {
		diffs = append(diffs, fmt.Sprintf("ExtendedS3Destination.CloudWatchLoggingOptions.LogGroupName: %q -> %q", aws.ToString(existing.LogGroupName), aws.ToString(new.LogGroupName)))
	}
	if !util.StringPtrEquals(existing.LogStreamName, new.LogStreamName) {
		diffs = append(diffs, fmt.Sprintf("ExtendedS3Destination.CloudWatchLoggingOptions.LogStreamName: %q -> %q", aws.ToString(existing.LogStreamName), aws.ToString(new.LogStreamName)))
	}

	return diffs
}

func diffExtendedS3Processing(existing, new *FirehoseProcessingConfiguration) []string {
	var diffs []string
	if existing == nil && new == nil {
		return diffs
	}
	if existing == nil {
		return []string{"ExtendedS3Destination.ProcessingConfiguration: nil -> configured"}
	}
	if new == nil {
		return []string{"ExtendedS3Destination.ProcessingConfiguration: configured -> nil"}
	}

	if aws.ToBool(existing.Enabled) != aws.ToBool(new.Enabled) {
		diffs = append(diffs, fmt.Sprintf("ExtendedS3Destination.ProcessingConfiguration.Enabled: %v -> %v", aws.ToBool(existing.Enabled), aws.ToBool(new.Enabled)))
	}

	diffs = append(diffs, diffProcessingProcessors("ExtendedS3Destination.ProcessingConfiguration", existing.Processors, new.Processors)...)

	return diffs
}

func diffExtendedS3Encryption(existing, new *FirehoseEncryptionConfiguration) []string {
	var diffs []string
	if existing == nil && new == nil {
		return diffs
	}
	if existing == nil {
		return []string{"ExtendedS3Destination.EncryptionConfiguration: nil -> configured"}
	}
	if new == nil {
		return []string{"ExtendedS3Destination.EncryptionConfiguration: configured -> nil"}
	}

	if !strings.EqualFold(existing.Type, new.Type) {
		diffs = append(diffs, fmt.Sprintf("ExtendedS3Destination.EncryptionConfiguration.Type: %q -> %q", existing.Type, new.Type))
	}
	if strings.EqualFold(existing.Type, firehoseEncryptionTypeKMS) {
		if !util.StringPtrEquals(existing.KMSKey, new.KMSKey) {
			diffs = append(diffs, fmt.Sprintf("ExtendedS3Destination.EncryptionConfiguration.KMSKey: %q -> %q", aws.ToString(existing.KMSKey), aws.ToString(new.KMSKey)))
		}
	}

	return diffs
}

func diffIcebergDestination(existing, new *FirehoseIcebergDestination) []string {
	var diffs []string
	if existing == nil && new == nil {
		return diffs
	}
	if existing == nil {
		return []string{"IcebergDestination: nil -> configured"}
	}
	if new == nil {
		return []string{"IcebergDestination: configured -> nil"}
	}

	diffs = append(diffs, diffIcebergCatalogConfig(existing.CatalogConfiguration, new.CatalogConfiguration)...)
	diffs = append(diffs, diffIcebergRole(util.Coalesce(existing.RoleArn, ""), util.Coalesce(new.RoleArn, ""))...)

	// Call detailed S3Configuration diffs
	s3Diffs := diffIcebergS3Configuration(&existing.S3Configuration, &new.S3Configuration)
	diffs = append(diffs, s3Diffs...)

	diffs = append(diffs, diffIcebergBuffering(existing, new)...)
	diffs = append(diffs, diffIcebergLogging(existing.CloudWatchLoggingOptions, new.CloudWatchLoggingOptions)...)
	diffs = append(diffs, diffIcebergProcessing(existing.ProcessingConfiguration, new.ProcessingConfiguration)...)
	diffs = append(diffs, diffIcebergRetryOptions(existing.RetryOptions, new.RetryOptions)...)
	diffs = append(diffs, diffIcebergBackupMode(existing.S3BackupMode, new.S3BackupMode)...)
	diffs = append(diffs, diffIcebergBackupConfiguration(existing.S3BackupConfiguration, new.S3BackupConfiguration)...)
	diffs = append(diffs, diffIcebergAppendOnly(existing.AppendOnly, new.AppendOnly)...)
	diffs = append(diffs, diffIcebergSchemaEvolution(existing.SchemaEvolutionConfiguration, new.SchemaEvolutionConfiguration)...)
	diffs = append(diffs, diffIcebergTableCreation(existing.TableCreationConfiguration, new.TableCreationConfiguration)...)
	diffs = append(diffs, diffIcebergDestinationTables(existing.DestinationTableConfiguration, new.DestinationTableConfiguration)...)

	return diffs
}

func diffIcebergS3Configuration(existing, new *FirehoseIcebergS3Configuration) []string {
	var diffs []string
	diffs = append(diffs, diffIcebergS3Bucket(util.Coalesce(existing.BucketArn, ""), util.Coalesce(new.BucketArn, ""))...)
	diffs = append(diffs, diffIcebergS3Role(util.Coalesce(existing.RoleArn, ""), util.Coalesce(new.RoleArn, ""))...)
	diffs = append(diffs, diffIcebergS3Prefix(aws.ToString(existing.Prefix), aws.ToString(new.Prefix))...)
	diffs = append(diffs, diffIcebergS3ErrorOutputPrefix(aws.ToString(existing.ErrorOutputPrefix), aws.ToString(new.ErrorOutputPrefix))...)
	diffs = append(diffs, diffIcebergS3BufferingInterval(existing.BufferingIntervalSeconds, new.BufferingIntervalSeconds)...)
	diffs = append(diffs, diffIcebergS3BufferingSize(existing.BufferingSizeMBs, new.BufferingSizeMBs)...)
	diffs = append(diffs, diffIcebergS3Compression(aws.ToString(existing.CompressionFormat), aws.ToString(new.CompressionFormat))...)
	diffs = append(diffs, diffIcebergS3Logging(existing.CloudWatchLoggingOptions, new.CloudWatchLoggingOptions)...)
	diffs = append(diffs, diffIcebergS3Encryption(existing.EncryptionConfiguration, new.EncryptionConfiguration)...)
	return diffs
}

func diffIcebergS3Bucket(existing, new string) []string {
	var diffs []string
	if existing != new {
		diffs = append(diffs, fmt.Sprintf("IcebergDestination.S3Configuration.Bucket: %q -> %q", existing, new))
	}
	return diffs
}

func diffIcebergS3Role(existing, new string) []string {
	var diffs []string
	if existing != new {
		diffs = append(diffs, fmt.Sprintf("IcebergDestination.S3Configuration.Role: %q -> %q", existing, new))
	}
	return diffs
}

func diffIcebergS3Prefix(existing, new string) []string {
	var diffs []string
	if existing != new {
		diffs = append(diffs, fmt.Sprintf("IcebergDestination.S3Configuration.Prefix: %q -> %q", existing, new))
	}
	return diffs
}

func diffIcebergS3ErrorOutputPrefix(existing, new string) []string {
	var diffs []string
	if existing != new {
		diffs = append(diffs, fmt.Sprintf("IcebergDestination.S3Configuration.ErrorOutputPrefix: %q -> %q", existing, new))
	}
	return diffs
}

func diffIcebergS3BufferingInterval(existing, new *int32) []string {
	var diffs []string
	if !util.Int32PtrEquals(existing, new) {
		diffs = append(diffs, fmt.Sprintf("IcebergDestination.S3Configuration.BufferingIntervalSeconds: %d -> %d", aws.ToInt32(existing), aws.ToInt32(new)))
	}
	return diffs
}

func diffIcebergS3BufferingSize(existing, new *int32) []string {
	var diffs []string
	if !util.Int32PtrEquals(existing, new) {
		diffs = append(diffs, fmt.Sprintf("IcebergDestination.S3Configuration.BufferingSizeMBs: %d -> %d", aws.ToInt32(existing), aws.ToInt32(new)))
	}
	return diffs
}

func diffIcebergS3Compression(existing, new string) []string {
	var diffs []string
	if normalizeFirehoseCompression(existing) != normalizeFirehoseCompression(new) {
		diffs = append(diffs, fmt.Sprintf("IcebergDestination.S3Configuration.CompressionFormat: %q -> %q", existing, new))
	}
	return diffs
}

func diffIcebergS3Logging(existing, new *FirehoseLoggingOptions) []string {
	var diffs []string
	if existing == nil && new == nil {
		return diffs
	}
	if existing == nil {
		return []string{"IcebergDestination.S3Configuration.CloudWatchLoggingOptions: nil -> configured"}
	}
	if new == nil {
		return []string{"IcebergDestination.S3Configuration.CloudWatchLoggingOptions: configured -> nil"}
	}

	if aws.ToBool(existing.Enabled) != aws.ToBool(new.Enabled) {
		diffs = append(diffs, fmt.Sprintf("IcebergDestination.S3Configuration.CloudWatchLoggingOptions.Enabled: %v -> %v", aws.ToBool(existing.Enabled), aws.ToBool(new.Enabled)))
	}
	if !util.StringPtrEquals(existing.LogGroupName, new.LogGroupName) {
		diffs = append(diffs, fmt.Sprintf("IcebergDestination.S3Configuration.CloudWatchLoggingOptions.LogGroupName: %q -> %q", aws.ToString(existing.LogGroupName), aws.ToString(new.LogGroupName)))
	}
	if !util.StringPtrEquals(existing.LogStreamName, new.LogStreamName) {
		diffs = append(diffs, fmt.Sprintf("IcebergDestination.S3Configuration.CloudWatchLoggingOptions.LogStreamName: %q -> %q", aws.ToString(existing.LogStreamName), aws.ToString(new.LogStreamName)))
	}

	return diffs
}

func diffIcebergS3Encryption(existing, new *FirehoseEncryptionConfiguration) []string {
	var diffs []string
	if existing == nil && new == nil {
		return diffs
	}
	if existing == nil {
		return []string{"IcebergDestination.S3Configuration.EncryptionConfiguration: nil -> configured"}
	}
	if new == nil {
		return []string{"IcebergDestination.S3Configuration.EncryptionConfiguration: configured -> nil"}
	}

	if !strings.EqualFold(existing.Type, new.Type) {
		diffs = append(diffs, fmt.Sprintf("IcebergDestination.S3Configuration.EncryptionConfiguration.Type: %q -> %q", existing.Type, new.Type))
	}

	// Always check KMSKey when new type is KMS (regardless of existing type)
	if strings.EqualFold(new.Type, firehoseEncryptionTypeKMS) {
		if !util.StringPtrEquals(existing.KMSKey, new.KMSKey) {
			diffs = append(diffs, fmt.Sprintf("IcebergDestination.S3Configuration.EncryptionConfiguration.KMSKey: %q -> %q", aws.ToString(existing.KMSKey), aws.ToString(new.KMSKey)))
		}
	}

	return diffs
}

func diffIcebergCatalogConfig(existing, new FirehoseIcebergCatalogConfiguration) []string {
	var diffs []string
	if util.Coalesce(existing.CatalogArn, "") != util.Coalesce(new.CatalogArn, "") {
		diffs = append(diffs, fmt.Sprintf("IcebergDestination.CatalogConfiguration.Catalog: %q -> %q", util.Coalesce(existing.CatalogArn, ""), util.Coalesce(new.CatalogArn, "")))
	}
	if !util.StringPtrEquals(existing.WarehouseLocation, new.WarehouseLocation) {
		diffs = append(diffs, fmt.Sprintf("IcebergDestination.CatalogConfiguration.WarehouseLocation: %q -> %q", aws.ToString(existing.WarehouseLocation), aws.ToString(new.WarehouseLocation)))
	}
	return diffs
}

func diffIcebergRole(existing, new string) []string {
	var diffs []string
	if existing != new {
		diffs = append(diffs, fmt.Sprintf("IcebergDestination.Role: %q -> %q", existing, new))
	}
	return diffs
}

func diffIcebergBuffering(existing, new *FirehoseIcebergDestination) []string {
	var diffs []string
	if !util.Int32PtrEquals(existing.BufferingIntervalSeconds, new.BufferingIntervalSeconds) {
		diffs = append(diffs, fmt.Sprintf("IcebergDestination.BufferingIntervalSeconds: %d -> %d", aws.ToInt32(existing.BufferingIntervalSeconds), aws.ToInt32(new.BufferingIntervalSeconds)))
	}
	if !util.Int32PtrEquals(existing.BufferingSizeMBs, new.BufferingSizeMBs) {
		diffs = append(diffs, fmt.Sprintf("IcebergDestination.BufferingSizeMBs: %d -> %d", aws.ToInt32(existing.BufferingSizeMBs), aws.ToInt32(new.BufferingSizeMBs)))
	}
	return diffs
}

func diffIcebergLogging(existing, new *FirehoseLoggingOptions) []string {
	var diffs []string
	if existing == nil && new == nil {
		return diffs
	}
	if existing == nil {
		return []string{"IcebergDestination.CloudWatchLoggingOptions: nil -> configured"}
	}
	if new == nil {
		return []string{"IcebergDestination.CloudWatchLoggingOptions: configured -> nil"}
	}

	if aws.ToBool(existing.Enabled) != aws.ToBool(new.Enabled) {
		diffs = append(diffs, fmt.Sprintf("IcebergDestination.CloudWatchLoggingOptions.Enabled: %v -> %v", aws.ToBool(existing.Enabled), aws.ToBool(new.Enabled)))
	}
	if !util.StringPtrEquals(existing.LogGroupName, new.LogGroupName) {
		diffs = append(diffs, fmt.Sprintf("IcebergDestination.CloudWatchLoggingOptions.LogGroupName: %q -> %q", aws.ToString(existing.LogGroupName), aws.ToString(new.LogGroupName)))
	}
	if !util.StringPtrEquals(existing.LogStreamName, new.LogStreamName) {
		diffs = append(diffs, fmt.Sprintf("IcebergDestination.CloudWatchLoggingOptions.LogStreamName: %q -> %q", aws.ToString(existing.LogStreamName), aws.ToString(new.LogStreamName)))
	}

	return diffs
}

func diffIcebergProcessing(existing, new *FirehoseProcessingConfiguration) []string {
	var diffs []string
	if existing == nil && new == nil {
		return diffs
	}
	if existing == nil {
		return []string{"IcebergDestination.ProcessingConfiguration: nil -> configured"}
	}
	if new == nil {
		return []string{"IcebergDestination.ProcessingConfiguration: configured -> nil"}
	}

	if aws.ToBool(existing.Enabled) != aws.ToBool(new.Enabled) {
		diffs = append(diffs, fmt.Sprintf("IcebergDestination.ProcessingConfiguration.Enabled: %v -> %v", aws.ToBool(existing.Enabled), aws.ToBool(new.Enabled)))
	}

	diffs = append(diffs, diffProcessingProcessors("IcebergDestination.ProcessingConfiguration", existing.Processors, new.Processors)...)

	return diffs
}

func diffIcebergRetryOptions(existing, new *FirehoseRetryOptions) []string {
	var diffs []string
	if existing == nil && new == nil {
		return diffs
	}
	if existing == nil {
		return []string{"IcebergDestination.RetryOptions: nil -> configured"}
	}
	if new == nil {
		return []string{"IcebergDestination.RetryOptions: configured -> nil"}
	}

	if !util.Int32PtrEquals(existing.DurationInSeconds, new.DurationInSeconds) {
		diffs = append(diffs, fmt.Sprintf("IcebergDestination.RetryOptions.DurationInSeconds: %d -> %d", aws.ToInt32(existing.DurationInSeconds), aws.ToInt32(new.DurationInSeconds)))
	}
	return diffs
}

func diffIcebergBackupMode(existing, new *string) []string {
	var diffs []string
	if !util.StringPtrEquals(existing, new) {
		diffs = append(diffs, fmt.Sprintf("IcebergDestination.S3BackupMode: %q -> %q", aws.ToString(existing), aws.ToString(new)))
	}
	return diffs
}

func diffIcebergBackupConfiguration(existing, new *FirehoseExtendedS3Destination) []string {
	var diffs []string
	if existing == nil && new == nil {
		return diffs
	}
	if existing == nil {
		return []string{"IcebergDestination.S3BackupConfiguration: nil -> configured"}
	}
	if new == nil {
		return []string{"IcebergDestination.S3BackupConfiguration: configured -> nil"}
	}

	diffs = append(diffs, diffExtendedS3Destination(existing, new)...)
	return diffs
}

func diffIcebergAppendOnly(existing, new *bool) []string {
	var diffs []string
	if aws.ToBool(existing) != aws.ToBool(new) {
		diffs = append(diffs, fmt.Sprintf("IcebergDestination.AppendOnly: %v -> %v", aws.ToBool(existing), aws.ToBool(new)))
	}
	return diffs
}

func diffIcebergSchemaEvolution(existing, new *FirehoseSchemaEvolutionConfiguration) []string {
	var diffs []string
	e1 := false
	if existing != nil {
		e1 = aws.ToBool(existing.Enabled)
	}
	e2 := false
	if new != nil {
		e2 = aws.ToBool(new.Enabled)
	}
	if e1 != e2 {
		diffs = append(diffs, fmt.Sprintf("IcebergDestination.SchemaEvolutionConfiguration.Enabled: %v -> %v", e1, e2))
	}
	return diffs
}

func diffIcebergTableCreation(existing, new *FirehoseTableCreationConfiguration) []string {
	var diffs []string
	e1 := false
	if existing != nil {
		e1 = aws.ToBool(existing.Enabled)
	}
	e2 := false
	if new != nil {
		e2 = aws.ToBool(new.Enabled)
	}
	if e1 != e2 {
		diffs = append(diffs, fmt.Sprintf("IcebergDestination.TableCreationConfiguration.Enabled: %v -> %v", e1, e2))
	}
	return diffs
}

func diffIcebergDestinationTables(existing, new []FirehoseIcebergTableConfiguration) []string {
	var diffs []string
	if len(existing) != len(new) {
		diffs = append(diffs, fmt.Sprintf("IcebergDestination.DestinationTableConfiguration count: %d -> %d", len(existing), len(new)))
	}

	maxLen := len(existing)
	if len(new) > maxLen {
		maxLen = len(new)
	}

	for i := 0; i < maxLen; i++ {
		switch {
		case i >= len(existing):
			diffs = append(diffs, fmt.Sprintf("IcebergDestination.DestinationTableConfiguration[%d]: nil -> configured", i))
			diffs = append(diffs, diffIcebergDestinationTableConfig(i, FirehoseIcebergTableConfiguration{}, new[i])...)
		case i >= len(new):
			diffs = append(diffs, fmt.Sprintf("IcebergDestination.DestinationTableConfiguration[%d]: configured -> nil", i))
			diffs = append(diffs, diffIcebergDestinationTableConfig(i, existing[i], FirehoseIcebergTableConfiguration{})...)
		default:
			diffs = append(diffs, diffIcebergDestinationTableConfig(i, existing[i], new[i])...)
		}
	}
	return diffs
}

func diffIcebergDestinationTableConfig(index int, existing, new FirehoseIcebergTableConfiguration) []string {
	var diffs []string
	if existing.DatabaseName != new.DatabaseName {
		diffs = append(diffs, fmt.Sprintf("IcebergDestination.DestinationTableConfiguration[%d].DatabaseName: %q -> %q", index, existing.DatabaseName, new.DatabaseName))
	}
	if existing.TableName != new.TableName {
		diffs = append(diffs, fmt.Sprintf("IcebergDestination.DestinationTableConfiguration[%d].TableName: %q -> %q", index, existing.TableName, new.TableName))
	}
	if !util.StringPtrEquals(existing.S3ErrorOutputPrefix, new.S3ErrorOutputPrefix) {
		diffs = append(diffs, fmt.Sprintf("IcebergDestination.DestinationTableConfiguration[%d].S3ErrorOutputPrefix: %q -> %q", index, aws.ToString(existing.S3ErrorOutputPrefix), aws.ToString(new.S3ErrorOutputPrefix)))
	}
	if util.DiffStringSlices(existing.UniqueKeys, new.UniqueKeys) {
		diffs = append(diffs, fmt.Sprintf("IcebergDestination.DestinationTableConfiguration[%d].UniqueKeys: %v -> %v", index, existing.UniqueKeys, new.UniqueKeys))
	}
	return diffs
}
