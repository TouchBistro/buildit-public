# SQS Queue `sqs-queue` 

This resources creates a sqs queue. 

> You can qualify the resource name with a provider prefix or supply the provider name in the standard `provider` field of the resource to indicate the provider to use. If neither is supplied, the `main` is used as the default provider name.

Check out AWS documentation for sqs queue [here](https://awscli.amazonaws.com/v2/documentation/api/latest/reference/sqs/index.html#cli-aws-sqs). 


| Field | Description | DataType | Required | Default |
|--|--|--|--|--|
|Name (Resource Name)|The resource name is used as the name of the sqs queue. A queue is FIFO if the queue name ends with `.fifo`|`string`|Yes||
|`policy`|IAM policy attached to the queue. See [IAMPolicyDocument](./tbd.md/#) for more details |`IAMPolicyDocument`|No||
|`attributes`|The attributes of the sqs queue. See [Attributes](#attributes) section for more details |`map[string]string`|No|`{}`|
|`tags`|A key value map of resource tags to be applied to this resource. The `GlobalTags` are always applied, any matching keys are overriden from `tags`|`map[string]string`|No|`{}`|
|`dependsOn`|The `buildit` resources that this resource depends on in the context of the current execution. All resources that are listed in this section will be built before this; while destoryed after this|`[]string`|No|`[]`|

## Attributes
The attributes for the sqs queue. Check out AWS documentation for sqs queue attributes [here](https://docs.aws.amazon.com/AWSSimpleQueueService/latest/APIReference/API_SetQueueAttributes.html)

| Field | Description | DataType | Required | Default |
|--|--|--|--|--|
|`delaySeconds`|The length of time, in seconds, for which the delivery of all messages in the queue is delayed |`int`|No||
|`maxMessageBytes`|The limit of how many bytes a message can contain before Amazon SQS rejects it |`int`|No|`262144`|
|`messageRetentionSeconds`|The length of time, in seconds, for which Amazon SQS retains a message |`int`|No|`345600`|
|`receiveMessageWaitSeconds`|The length of time, in seconds, for which a ReceiveMessage action waits for a message to arrive |`int`|No||
|`visibilityTimeoutSeconds`| The visibility timeout for the queue, in seconds |`int`|No|`30`|
|`contentBasedDeduplication`|Enables content-based deduplication |`bool`|No||
|`deduplicationScope`|Specifies whether message deduplication occurs at the message group or queue level. Valid values: `queue`, `messageGroup` |`string`|No|`queue` if FIFO|
|`fifoThroughputLimit`| Specifies whether the FIFO queue throughput quota applies to the entire queue or per message group. Valid values: `perQueue`, `perMessageGroupId` |`string`|No|`perQueue` if FIFO|


Example:

```yaml 

resources:
  sqs-queue:
   test-${rand}-sqs:
     attributes: 
       delaySeconds: 360
       maxMessageBytes: 128000
       messageRetentionSeconds: 86400
       receiveMessageWaitSeconds: 5
       visibilityTimeoutSeconds: 21
       contentBasedDeduplication: no
     policy: 
        {
          "Version": "2012-10-17",
          "Statement": [
            {
              "Effect": "Deny",
              "Principal": {
                "AWS": "arn:aws:iam::123456789012:user/example-user"
              },
              "Action": "sqs:*",
              "Resource": "arn:aws:sqs:us-east-1:${account-id}:test-${rand}-sqs"
            }
          ]
        }
     tags:
       dummy: value 
```
