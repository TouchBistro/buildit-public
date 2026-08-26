# CloudWatch Subscription Filter `cloudwatch-subscriptionfilter` 

This resources creates a cloudwatch subscription filter using the `main` provider. 

> You can qualify the resource name with a provider prefix or supply the provider name in the standard `provider` field of the resource to indicate the provider to use. If neither is supplied, the `main` is used as the default provider name. 

Check out AWS documentation for cloudwatch subscription filter [here](https://awscli.amazonaws.com/v2/documentation/api/latest/reference/logs/put-subscription-filter.html). 

| Field | Description | DataType | Required | Default |
|--|--|--|--|--|
|Name (Resource Name)|The resource name is used as the name of the subscription filter|`string`|Yes||
|`filterName`|The name of the filter. If not provided, will be default to `Resource Name`. Allows creating filters with the same name under different log group in the same buildit |`string`|No|` Resource Name`|
|`destination`|Destination to deliver matching log events to. Must be a valid lambda function|`string`|Yes||
|`filterPattern`|A filter pattern for subscribing to a filtered stream of log events. Supported format can be found [here](https://docs.aws.amazon.com/AmazonCloudWatch/latest/logs/FilterAndPatternSyntax.html)|`string`|No|`""`|
|`logGroup`|The name of the log group to ingest logs from|`string`|Yes||
|`dependsOn`|The `buildit` resources that this resource depends on in the context of the current execution. All resources that are listed in this section will be built before this; while destoryed after this|`[]string`|No|`[]`|

Example: subscription filter with filter patterns to match terms in JSON log events

```yaml 
resources:
  cloudwatch-subscriptionfilter:
    example-test-filter:
      destination: example-test-lambda
      filterPattern: '{($.action = "BLOCK")}'
      logGroup: test-group
```
