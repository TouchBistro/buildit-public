package firehose

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	firehosetypes "github.com/aws/aws-sdk-go-v2/service/firehose/types"
)

func (l *FirehoseLoggingOptions) normalize() {
	if l.Enabled == nil {
		l.Enabled = aws.Bool(false)
	}
}

func (l *FirehoseLoggingOptions) validate() []string {
	var msgs []string

	if aws.ToBool(l.Enabled) {
		if aws.ToString(l.LogGroupName) == "" {
			msgs = append(msgs, "cloudWatchLogging.logGroupName is required when logging is enabled")
		}
		if aws.ToString(l.LogStreamName) == "" {
			msgs = append(msgs, "cloudWatchLogging.logStreamName is required when logging is enabled")
		}
	}

	return msgs
}

func (l *FirehoseLoggingOptions) toAWS() *firehosetypes.CloudWatchLoggingOptions {
	if l == nil {
		return nil
	}

	l.normalize()
	return &firehosetypes.CloudWatchLoggingOptions{
		Enabled:       l.Enabled,
		LogGroupName:  l.LogGroupName,
		LogStreamName: l.LogStreamName,
	}
}

func (e *FirehoseEncryptionConfiguration) normalize(ctx context.Context, providerName string) {
	e.Type = strings.ToUpper(strings.TrimSpace(e.Type))
	if e.Type == "" {
		e.Type = string(firehosetypes.NoEncryptionConfigNoEncryption)
	}

	if e.Type != firehoseEncryptionTypeKMS {
		e.Type = string(firehosetypes.NoEncryptionConfigNoEncryption)
		e.KMSKey = nil
		e.KMSKeyArn = nil
	} else if e.KMSKey != nil {
		// KMS lookup is not yet available in awsw, so we use the value as-is.
		// If it's already an ARN, it will work. If it's a name/alias, it might fail at AWS API level
		// unless we add lookup logic later.
		e.KMSKeyArn = e.KMSKey
	}
}

func (e FirehoseEncryptionConfiguration) validate() []string {
	var msgs []string

	if strings.EqualFold(e.Type, firehoseEncryptionTypeKMS) {
		if e.KMSKey == nil || len(aws.ToString(e.KMSKey)) == 0 {
			msgs = append(msgs, "encryptionConfiguration.kmsKey is required when type is KMS")
		}
	} else if e.Type != "" && !strings.EqualFold(e.Type, "NOENCRYPTION") {
		msgs = append(msgs, fmt.Sprintf("encryptionConfiguration.type must be %q or %q", firehoseEncryptionTypeKMS, firehosetypes.NoEncryptionConfigNoEncryption))
	}

	return msgs
}

func (e *FirehoseEncryptionConfiguration) toAWS() *firehosetypes.EncryptionConfiguration {
	if e == nil {
		return &firehosetypes.EncryptionConfiguration{
			NoEncryptionConfig: firehosetypes.NoEncryptionConfigNoEncryption,
		}
	}

	if e.Type == firehoseEncryptionTypeKMS {
		return &firehosetypes.EncryptionConfiguration{
			KMSEncryptionConfig: &firehosetypes.KMSEncryptionConfig{
				AWSKMSKeyARN: e.KMSKeyArn,
			},
		}
	}

	return &firehosetypes.EncryptionConfiguration{
		NoEncryptionConfig: firehosetypes.NoEncryptionConfigNoEncryption,
	}
}

func loggingFromDescription(opts *firehosetypes.CloudWatchLoggingOptions) *FirehoseLoggingOptions {
	if opts == nil {
		return &FirehoseLoggingOptions{
			Enabled: aws.Bool(false),
		}
	}

	return &FirehoseLoggingOptions{
		Enabled:       opts.Enabled,
		LogGroupName:  opts.LogGroupName,
		LogStreamName: opts.LogStreamName,
	}
}

func encryptionFromDescription(enc *firehosetypes.EncryptionConfiguration) *FirehoseEncryptionConfiguration {
	if enc == nil {
		return &FirehoseEncryptionConfiguration{
			Type: string(firehosetypes.NoEncryptionConfigNoEncryption),
		}
	}

	if enc.KMSEncryptionConfig != nil {
		return &FirehoseEncryptionConfiguration{
			Type:      firehoseEncryptionTypeKMS,
			KMSKeyArn: enc.KMSEncryptionConfig.AWSKMSKeyARN,
		}
	}

	return &FirehoseEncryptionConfiguration{
		Type: string(enc.NoEncryptionConfig),
	}
}

func normalizeFirehoseCompression(format string) firehosetypes.CompressionFormat {
	switch strings.ToUpper(format) {
	case "GZIP":
		return firehosetypes.CompressionFormatGzip
	case "ZIP":
		return firehosetypes.CompressionFormatZip
	case "SNAPPY":
		return firehosetypes.CompressionFormatSnappy
	case "HADOOP_SNAPPY", "HADOOP-SNAPPY":
		return firehosetypes.CompressionFormatHadoopSnappy
	case "UNCOMPRESSED":
		return firehosetypes.CompressionFormatUncompressed
	default:
		return firehosetypes.CompressionFormatUncompressed
	}
}

func isSupportedCompression(format string) bool {
	switch normalizeFirehoseCompression(format) {
	case firehosetypes.CompressionFormatGzip,
		firehosetypes.CompressionFormatZip,
		firehosetypes.CompressionFormatSnappy,
		firehosetypes.CompressionFormatHadoopSnappy,
		firehosetypes.CompressionFormatUncompressed:
		return true
	default:
		return false
	}
}
