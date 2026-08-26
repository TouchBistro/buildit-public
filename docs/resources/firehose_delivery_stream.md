# Firehose Delivery Stream `firehose-delivery-stream`

Create, update or destroy a Kinesis Data Firehose delivery stream.

> You can qualify the resource name with a provider prefix or supply the provider name in the standard `provider` field of the resource to indicate the provider to use. If neither is supplied, the `main` is used as the default provider name.

Check out AWS documentation for firehose delivery stream [here](https://awscli.amazonaws.com/v2/documentation/api/latest/reference/firehose/create-delivery-stream.html).

| Field | Description | DataType | Required | Default |
| :--- | :--- | :--- | :--- | :--- |
| Name (Resource Name) | The buildit resource name | `string` | Yes | |
| `type` | The Kinesis Data Firehose delivery stream type. Valid values: `DirectPut`, `KinesisStreamAsSource` | `string` | No | `DirectPut` |
| `extendedS3Destination` | The configuration for the Extended S3 destination. See [ExtendedS3Destination](#extendeds3destination) section for more details | `ExtendedS3Destination` | No | |
| `icebergDestination` | The configuration for the Iceberg destination. See [IcebergDestination](#icebergdestination) section for more details | `IcebergDestination` | No | |
| `tags` | A key value map of resource tags to be applied to this resource. The `GlobalTags` are always applied, any matching keys are overridden from `tags` | `map[string]string` | No | `{}` |
| `dependsOn` | The `buildit` resources that this resource depends on in the context of the current execution. All resources that are listed in this section will be built before this; while destroyed after this | `[]string` | No | `[]` |

Example:

```yaml
resources:
  firehose-delivery-stream:
    my-stream:
      type: DirectPut
      extendedS3Destination:
        bucketArn: arn:aws:s3:::my-bucket
        roleArn: arn:aws:iam::123456789012:role/firehose-role
        prefix: logs/
        bufferingIntervalSeconds: 300
        bufferingSizeMBs: 5
        compressionFormat: GZIP
        cloudWatchLogging:
          enabled: true
          logGroupName: /aws/kinesisfirehose/my-stream
          logStreamName: s3-delivery
        processingConfiguration:
          enabled: true
          processors:
            - type: Lambda
              parameters:
                LambdaArn: arn:aws:lambda:us-east-1:123456789012:function:my-processor
        encryptionConfiguration:
          type: KMS
          kmsKeyArn: arn:aws:kms:us-east-1:123456789012:key/abcd-1234
      # OR
      icebergDestination:
        catalogConfiguration:
          catalogArn: arn:aws:glue:us-east-1:123456789012:catalog
          warehouseLocation: s3://my-warehouse-bucket/
        roleArn: arn:aws:iam::123456789012:role/firehose-role
        s3Configuration:
          bucketArn: arn:aws:s3:::my-bucket
          roleArn: arn:aws:iam::123456789012:role/s3-role
          prefix: data/
          bufferingIntervalSeconds: 300
          bufferingSizeMBs: 5
          compressionFormat: GZIP
        schemaEvolutionConfiguration:
          enabled: true
        tableCreationConfiguration:
          enabled: true
        destinationTableConfiguration:
          - databaseName: my_db
            tableName: my_table
            uniqueKeys:
              - id
        bufferingIntervalSeconds: 300
        bufferingSizeMBs: 5
        cloudWatchLogging:
          enabled: true
          logGroupName: /aws/kinesisfirehose/my-stream
          logStreamName: iceberg-delivery
      tags:
        env: prod
```

## ExtendedS3Destination

Represents the configuration for an Extended S3 destination.

| Field | Description | DataType | Required | Default |
| :--- | :--- | :--- | :--- | :--- |
| `bucketArn` | The ARN of the S3 bucket | `string` | Yes | |
| `roleArn` | The ARN of the IAM role | `string` | Yes | |
| `prefix` | The "YYYY/MM/DD/HH" time format prefix is automatically used for delivered Amazon S3 files. You can specify an extra prefix to be added in front of the time format prefix | `string` | No | |
| `errorOutputPrefix` | A prefix that Kinesis Data Firehose evaluates and adds to failed records before writing them to S3 | `string` | No | |
| `bufferingIntervalSeconds` | The buffering interval in seconds | `int32` | No | `300` |
| `bufferingSizeMBs` | The buffering size in MBs | `int32` | No | `5` |
| `compressionFormat` | The compression format. Valid values: `UNCOMPRESSED`, `GZIP`, `ZIP`, `Snappy`, `HADOOP_SNAPPY` | `string` | No | `UNCOMPRESSED` |
| `cloudWatchLogging` | The CloudWatch logging options. See [CloudWatchLoggingOptions](#cloudwatchloggingoptions) section for more details | `CloudWatchLoggingOptions` | No | |
| `processingConfiguration` | The data processing configuration. See [ProcessingConfiguration](#processingconfiguration) section for more details | `ProcessingConfiguration` | No | |
| `encryptionConfiguration` | The encryption configuration. See [EncryptionConfiguration](#encryptionconfiguration) section for more details | `EncryptionConfiguration` | No | `type: NoEncryption` |

## IcebergDestination

Represents the configuration for an Iceberg destination.

| Field | Description | DataType | Required | Default |
| :--- | :--- | :--- | :--- | :--- |
| `catalogConfiguration` | The catalog configuration. See [CatalogConfiguration](#catalogconfiguration) | `CatalogConfiguration` | Yes | |
| `roleArn` | The ARN of the IAM role to be assumed by Firehose for calling Iceberg Table operations | `string` | Yes | |
| `s3Configuration` | The S3 configuration. See [IcebergS3Configuration](#icebergs3configuration) | `IcebergS3Configuration` | Yes | |
| `bufferingIntervalSeconds` | The buffering interval in seconds | `int32` | No | |
| `bufferingSizeMBs` | The buffering size in MBs | `int32` | No | |
| `cloudWatchLogging` | The CloudWatch logging options. See [CloudWatchLoggingOptions](#cloudwatchloggingoptions) | `CloudWatchLoggingOptions` | No | |
| `destinationTableConfiguration` | The destination table configuration. See [DestinationTableConfiguration](#destinationtableconfiguration) | `[]DestinationTableConfiguration` | Yes | |
| `processingConfiguration` | The data processing configuration. See [ProcessingConfiguration](#processingconfiguration) | `ProcessingConfiguration` | No | |
| `retryOptions` | The retry options. See [RetryOptions](#retryoptions) | `RetryOptions` | No | |
| `s3BackupMode` | The S3 backup mode | `string` | No | |
| `s3BackupConfiguration` | The S3 backup configuration. See [ExtendedS3Destination](#extendeds3destination) | `ExtendedS3Destination` | No | |
| `appendOnly` | Whether to use append-only mode | `bool` | No | |
| `schemaEvolutionConfiguration` | The schema evolution configuration. See [SchemaEvolutionConfiguration](#schemaevolutionconfiguration) | `SchemaEvolutionConfiguration` | No | |
| `tableCreationConfiguration` | The table creation configuration. See [TableCreationConfiguration](#tablecreationconfiguration) | `TableCreationConfiguration` | No | |

## CatalogConfiguration

Represents the configuration for the Iceberg catalog.

| Field | Description | DataType | Required | Default |
| :--- | :--- | :--- | :--- | :--- |
| `catalogArn` | The ARN of the Glue catalog | `string` | Yes | |
| `warehouseLocation` | The warehouse location | `string` | No | |

## IcebergS3Configuration

Represents the configuration for the S3 destination in Iceberg.

| Field | Description | DataType | Required | Default |
| :--- | :--- | :--- | :--- | :--- |
| `bucketArn` | The ARN of the S3 bucket | `string` | Yes | |
| `roleArn` | The ARN of the IAM role | `string` | Yes | |
| `prefix` | The prefix for the S3 bucket | `string` | No | |
| `errorOutputPrefix` | The error output prefix | `string` | No | |
| `bufferingIntervalSeconds` | The buffering interval in seconds | `int32` | No | |
| `bufferingSizeMBs` | The buffering size in MBs | `int32` | No | |
| `compressionFormat` | The compression format | `string` | No | |
| `cloudWatchLogging` | The CloudWatch logging options. See [CloudWatchLoggingOptions](#cloudwatchloggingoptions) | `CloudWatchLoggingOptions` | No | |
| `encryptionConfiguration` | The encryption configuration. See [EncryptionConfiguration](#encryptionconfiguration) | `EncryptionConfiguration` | No | |

## DestinationTableConfiguration

Represents the configuration for destination tables.

| Field | Description | DataType | Required | Default |
| :--- | :--- | :--- | :--- | :--- |
| `databaseName` | The database name | `string` | Yes | |
| `tableName` | The table name | `string` | Yes | |
| `s3ErrorOutputPrefix` | The S3 error output prefix | `string` | No | |
| `uniqueKeys` | The list of unique keys | `[]string` | No | |

## CloudWatchLoggingOptions

Represents the CloudWatch logging options.

| Field | Description | DataType | Required | Default |
| :--- | :--- | :--- | :--- | :--- |
| `enabled` | Whether CloudWatch logging is enabled | `bool` | No | `false` |
| `logGroupName` | The CloudWatch log group name | `string` | No | |
| `logStreamName` | The CloudWatch log stream name | `string` | No | |

## ProcessingConfiguration

Represents the data processing configuration.

| Field | Description | DataType | Required | Default |
| :--- | :--- | :--- | :--- | :--- |
| `enabled` | Whether data processing is enabled | `bool` | No | |
| `processors` | The list of processors. See [Processor](#processor) | `[]Processor` | No | |

## Processor

Represents a data processor.

| Field | Description | DataType | Required | Default |
| :--- | :--- | :--- | :--- | :--- |
| `type` | The processor type, e.g., `Lambda` | `string` | Yes | |
| `parameters` | The processor parameters | `map[string]string` | Yes | |

## EncryptionConfiguration

Represents the encryption configuration.

| Field | Description | DataType | Required | Default |
| :--- | :--- | :--- | :--- | :--- |
| `type` | The encryption type. Valid values: `NoEncryption`, `KMS` | `string` | Yes | `NoEncryption` |
| `kmsKeyArn` | The ARN of the KMS key. Required if type is `KMS` | `string` | No | |

## RetryOptions

Represents the retry options.

| Field | Description | DataType | Required | Default |
| :--- | :--- | :--- | :--- | :--- |
| `durationInSeconds` | The retry duration in seconds | `int32` | No | |

## SchemaEvolutionConfiguration

Represents the schema evolution configuration.

| Field | Description | DataType | Required | Default |
| :--- | :--- | :--- | :--- | :--- |
| `enabled` | Whether schema evolution is enabled | `bool` | No | |

## TableCreationConfiguration

Represents the table creation configuration.

| Field | Description | DataType | Required | Default |
| :--- | :--- | :--- | :--- | :--- |
| `enabled` | Whether table creation is enabled | `bool` | No | |
