# CloudWatch Log Group `cloudwatch-loggroup` 

This resources creates a cloudwatch log group.

> You can qualify the resource name with a provider prefix or supply the provider name in the standard `provider` field of the resource to indicate the provider to use. If neither is supplied, the `main` is used as the default provider name.

Check out AWS documentation for cloudwatch log group [here](https://docs.aws.amazon.com/cli/latest/reference/logs/#cli-aws-logs). 


| Field | Description | DataType | Required | Default |
|--|--|--|--|--|
|Name (Resource Name)|The resource name is used as the name of the cloudwatch log group|`string`|Yes||
|`retention`|The number of days to retain the log events in the specified log group|`int32`|No|`731`|
|`metricFilters`|Represents a CloudWatch LogGroup Metric Filter. See [MetricFilters](#metricfilters) section for more details. |`[]CWLogGroupMetricFilter`|No|`[]`|
|`tags`|A key value map of resource tags to be applied to this resource. The `GlobalTags` are always applied, any matching keys are overriden from `tags`|`map[string]string`|No|`{}`|
|`dependsOn`|The `buildit` resources that this resource depends on in the context of the current execution. All resources that are listed in this section will be built before this; while destoryed after this|`[]string`|No|`[]`|

## MetricFilters
Creates or updates a metric filter and associates it with the specified log group. Check out AWS documentation for cloudwatch metric filter [here](https://docs.aws.amazon.com/cli/latest/reference/logs/put-metric-filter.html). 

| Field | Description | DataType | Required | Default |
|--|--|--|--|--|
|`name`|A name for the metric filter |`string`|Yes||
|`pattern`|A filter pattern for extracting metric data out of ingested log events|`string`|No|`""`|
|`metricTransformations`|Represents a CloudWatch LogGroup Metric Filter Transformation. See [MetricTransformations](#metrictransformations) section for more details. |`[]CWLogGroupMetricFilterTransformation`|No|`[]`|

## MetricTransformations
Defines how metric data get emitted from a metric filter. Check out AWS documentation for cloudwatch metric transformation [here](https://docs.aws.amazon.com/cli/latest/reference/logs/put-metric-filter.html). 

| Field | Description | DataType | Required | Default |
|--|--|--|--|--|
|`name`|The name of the CloudWatch metric|`string`|Yes||
|`namespace`|A custom namespace to contain your metric in CloudWatch. Use namespaces to group together metrics that are similar|`string`|Yes||
|`value`|The value to publish to the CloudWatch metric when a filter pattern matches a log event|`string`|Yes||
|`defaultValue`|The value to emit when a filter pattern does not match a log event|`float64`|No|`nil`|
|`dimensions`|The fields to use as dimensions for the metric. One metric filter can include as many as three dimensions|`map[string]string`|Yes||
|`unit`|The unit to assign to the metric |`string`|No|`""`|

> Metrics extracted from log events are charged as custom metrics. To prevent unexpected high charges, do not specify high-cardinality fields such as IPAddress or requestID as dimensions. Each different value found for a dimension is treated as a separate metric and accrues charges as a separate custom metric. CloudWatch Logs disables a metric filter if it generates 1000 different name/value pairs for your specified dimensions within a certain amount of time. This helps to prevent accidental high charges.

Example:

```yaml 

resources:
  cloudwatch-loggroup:
    CloudTrail/DefaultLogGroup:
      metricFilters:
        - name: cis-aws-config-changes
          pattern: >-
            {($.eventSource=config.amazonaws.com) && (($.eventName=StopConfigurationRecorder) || 
            ($.eventName=DeleteDeliveryChannel) || ($.eventName=PutDeliveryChannel) || 
            ($.eventName=PutConfigurationRecorder))}
          metricTransformations:
            - name: cis-aws-config-changes
              namespace: cis-log-metrics
              value: 1
          metricTransformations:
            - name: cis-vpc-changes
              namespace: cis-log-metrics
              value: 1
```

