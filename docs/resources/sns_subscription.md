# SNS Subscription `sns-subscription` 

This resources creates a sns subscription. 

> You can qualify the resource name with a provider prefix or supply the provider name in the standard `provider` field of the resource to indicate the provider to use. If neither is supplied, the `main` is used as the default provider name.

Check out AWS documentation for sns subscription [here](https://awscli.amazonaws.com/v2/documentation/api/latest/reference/sns/subscribe.html).


| Field | Description | DataType | Required | Default |
|--|--|--|--|--|
|Name (Resource Name)|The resource name is used as the name of the sns subscription|`string`|Yes||
|`protocol`|The protocol for the sns subscription. Currently only `lambda` is supported|`string`|No|`lambda`|
|`topicName`|The name of the topic to subscribe to|`string`|Yes||
|`endpointName`|The endpoint to be subscribed to the topic|`string`|Yes||
|`dependsOn`|The `buildit` resources that this resource depends on in the context of the current execution. All resources that are listed in this section will be built before this; while destoryed after this|`[]string`|No|`[]`|

Example:

```yaml 
resources:
  sns-subscription:
    example-kitchen-sink-example-complaints:
      protocol: lambda
      topicName: SES-Complaints
      endpointName: example-kitchen-sink-lambda
```