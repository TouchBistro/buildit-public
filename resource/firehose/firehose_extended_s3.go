package firehose

import (
	"context"
	"fmt"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/aws/aws-sdk-go-v2/aws"
	firehosetypes "github.com/aws/aws-sdk-go-v2/service/firehose/types"
)

func (d *FirehoseExtendedS3Destination) normalize(ctx context.Context, providerName string) {
	if d.Bucket != "" {
		arn, err := awsw.NewS3(ctx, providerName).BucketArnFromName(ctx, d.Bucket)
		if err != nil {
			panic(err)
		}
		d.BucketArn = arn
	}

	if d.Role != "" {
		arn, err := awsw.NewIAM(ctx, providerName).RoleArnForName(ctx, d.Role)
		if err != nil {
			panic(err)
		}
		d.RoleArn = arn
	}

	if d.BufferingIntervalSeconds == nil {
		d.BufferingIntervalSeconds = aws.Int32(defaultFirehoseBufferIntervalSeconds)
	}

	if d.BufferingSizeMBs == nil {
		d.BufferingSizeMBs = aws.Int32(defaultFirehoseBufferSizeMBs)
	}

	if d.CompressionFormat == nil || len(*d.CompressionFormat) == 0 {
		format := string(firehosetypes.CompressionFormatUncompressed)
		d.CompressionFormat = &format
	} else {
		format := string(normalizeFirehoseCompression(*d.CompressionFormat))
		d.CompressionFormat = aws.String(format)
	}

	if d.CloudWatchLoggingOptions == nil {
		d.CloudWatchLoggingOptions = &FirehoseLoggingOptions{
			Enabled: aws.Bool(false),
		}
	} else {
		d.CloudWatchLoggingOptions.normalize()
	}

	if d.ProcessingConfiguration != nil {
		d.ProcessingConfiguration.normalize(ctx, providerName, aws.ToString(d.RoleArn))
	}

	if d.EncryptionConfiguration == nil {
		d.EncryptionConfiguration = &FirehoseEncryptionConfiguration{
			Type: string(firehosetypes.NoEncryptionConfigNoEncryption),
		}
	} else {
		d.EncryptionConfiguration.normalize(ctx, providerName)
	}
}

func (d FirehoseExtendedS3Destination) validate() []string {
	var errorMsgs []string

	if d.Bucket == "" {
		errorMsgs = append(errorMsgs, "Firehose Extended S3 destination bucket is required")
	}

	if d.Role == "" {
		errorMsgs = append(errorMsgs, "Firehose Extended S3 destination IAM role is required")
	}

	if d.BufferingIntervalSeconds != nil {
		if *d.BufferingIntervalSeconds < 60 || *d.BufferingIntervalSeconds > 900 {
			errorMsgs = append(errorMsgs, "extendedS3Destination.bufferingIntervalSeconds must be between 60 and 900")
		}
	}

	if d.BufferingSizeMBs != nil {
		if *d.BufferingSizeMBs < 1 || *d.BufferingSizeMBs > 128 {
			errorMsgs = append(errorMsgs, "extendedS3Destination.bufferingSizeMBs must be between 1 and 128")
		}
	}

	if d.CompressionFormat != nil {
		if !isSupportedCompression(*d.CompressionFormat) {
			errorMsgs = append(errorMsgs, fmt.Sprintf("unsupported compression format %q", *d.CompressionFormat))
		}
	}

	if d.CloudWatchLoggingOptions != nil {
		errorMsgs = append(errorMsgs, d.CloudWatchLoggingOptions.validate()...)
	}

	if d.ProcessingConfiguration != nil {
		errorMsgs = append(errorMsgs, d.ProcessingConfiguration.validate()...)
	}

	if d.EncryptionConfiguration != nil {
		errorMsgs = append(errorMsgs, d.EncryptionConfiguration.validate()...)
	}

	return errorMsgs
}

func (d FirehoseExtendedS3Destination) toAWSConfiguration() *firehosetypes.ExtendedS3DestinationConfiguration {
	return &firehosetypes.ExtendedS3DestinationConfiguration{
		BucketARN: d.BucketArn,
		RoleARN:   d.RoleArn,
		Prefix:    d.Prefix,
		BufferingHints: &firehosetypes.BufferingHints{
			IntervalInSeconds: d.BufferingIntervalSeconds,
			SizeInMBs:         d.BufferingSizeMBs,
		},
		CompressionFormat:        normalizeFirehoseCompression(aws.ToString(d.CompressionFormat)),
		ErrorOutputPrefix:        d.ErrorOutputPrefix,
		CloudWatchLoggingOptions: d.CloudWatchLoggingOptions.toAWS(),
		ProcessingConfiguration:  d.ProcessingConfiguration.toAWS(),
		EncryptionConfiguration:  d.EncryptionConfiguration.toAWS(),
	}
}

func (d FirehoseExtendedS3Destination) toAWSUpdate() *firehosetypes.ExtendedS3DestinationUpdate {
	return &firehosetypes.ExtendedS3DestinationUpdate{
		BucketARN: d.BucketArn,
		RoleARN:   d.RoleArn,
		Prefix:    d.Prefix,
		BufferingHints: &firehosetypes.BufferingHints{
			IntervalInSeconds: d.BufferingIntervalSeconds,
			SizeInMBs:         d.BufferingSizeMBs,
		},
		CompressionFormat:        normalizeFirehoseCompression(aws.ToString(d.CompressionFormat)),
		ErrorOutputPrefix:        d.ErrorOutputPrefix,
		CloudWatchLoggingOptions: d.CloudWatchLoggingOptions.toAWS(),
		ProcessingConfiguration:  d.ProcessingConfiguration.toAWS(),
		EncryptionConfiguration:  d.EncryptionConfiguration.toAWS(),
	}
}

func firehoseDestinationFromDescription(desc *firehosetypes.ExtendedS3DestinationDescription) *FirehoseExtendedS3Destination {
	if desc == nil {
		return nil
	}
	dest := FirehoseExtendedS3Destination{
		BucketArn:                desc.BucketARN,
		RoleArn:                  desc.RoleARN,
		Prefix:                   desc.Prefix,
		ErrorOutputPrefix:        desc.ErrorOutputPrefix,
		BufferingIntervalSeconds: nil,
		BufferingSizeMBs:         nil,
		CompressionFormat:        aws.String(string(desc.CompressionFormat)),
		CloudWatchLoggingOptions: loggingFromDescription(desc.CloudWatchLoggingOptions),
		ProcessingConfiguration:  processingFromDescription(desc.ProcessingConfiguration),
		EncryptionConfiguration:  encryptionFromDescription(desc.EncryptionConfiguration),
	}

	if desc.BufferingHints != nil {
		dest.BufferingIntervalSeconds = desc.BufferingHints.IntervalInSeconds
		dest.BufferingSizeMBs = desc.BufferingHints.SizeInMBs
	}

	return &dest
}
