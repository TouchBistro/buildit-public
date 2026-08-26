# CloudWatch Metric Alarm `cloudwatch-metricalarm` 

This resources creates a cloudwatch metric alarm. 

> You can qualify the resource name with a provider prefix or supply the provider name in the standard `provider` field of the resource to indicate the provider to use. If neither is supplied, the `main` is used as the default provider name.

Check out AWS documentation for cloudwatch metric alarm [here](https://docs.aws.amazon.com/cli/latest/reference/cloudwatch/put-metric-alarm.html). 

> The following attributes/features are not support at the moment:
>- Percentils or ExtendedStatistics
>- Alarms based on a MetricsDataQuery
>- Anomaly Detection Alarms

| Field | Description | DataType | Required | Default |
|--|--|--|--|--|
|Name (Resource Name)|The resource name is used as the name of the metric alarm|`string`|Yes||
|`description`|The description for the alarm|`string`|No|`""`|
|`metricName`|The name for the metric associated with the alarm|`string`|No|`""`|
|`metricNamespace`|Namespace for the metric associated|`string`|No|`""`|
|`statistic`|The statistic for the metric specified in `MetricName`. Supports `average`, `sum`, `minimum`, `maximum`|`string`|No|`sum`|
|`period`|The length, in seconds, used each time the metric specified in `MetricName` is evaluated. Valid values are 10, 30, and any multiple of 60.|`int32`|Yes||
|`evaluationPeriods`|The number of periods over which data is compared to the specified threshold|`int32`|Yes||
|`threshold`|The value against which the specified statistic is compared|`float64`|Yes||
|`comparisonOperator`|The arithmetic operation to use when comparing the specified statistic and threshold. Valid values: `LT`, `LE`, `GT`, `GE`|`string`|Yes||
|`datapointsToAlarm`|The number of data points that must be breaching to trigger the alarm. This is used only if you are setting an "M out of N" alarm. In that case, this value is the M|`int32`|Yes||
|`dimensions`|The dimensions for the metric specified in `MetricName`|`map[string]string`|Yes||
|`actionsEnabled`|Indicates whether actions should be executed during any changes to the alarm state|`bool`|No|`true`|
|`alarmActions`|The actions to execute when this alarm transitions to the `ALARM` state from any other state|`map[string]string`|No|`{}`|
|`okActions`|The actions to execute when this alarm transitions to an `OK` state from any other state|`map[string]string`|No|`{}`|
|`insufficientDataActions`|The actions to execute when this alarm transitions to the `INSUFFICIENT_DATA` state from any other state|`map[string]string`|No|`{}`|
|`treatMissingData`|Sets how this alarm is to handle missing data points. Valid Values: `breaching`, `notBreaching`, `ignore`, `missing`|`string`|Yes||
|`unit`|The unit of measure for the statistic. If you don't specify `Unit` , CloudWatch retrieves all unit types that have been published for the metric and attempts to evaluate the alarm|`string`|No|`""`|
|`tags`|A key value map of resource tags to be applied to this resource. The `GlobalTags` are always applied, any matching keys are overriden from `tags`|`map[string]string`|No|`{}`|
|`dependsOn`|The `buildit` resources that this resource depends on in the context of the current execution. All resources that are listed in this section will be built before this; while destoryed after this|`[]string`|No|`[]`|

Example: metric alarm that scale ECS service when usage is up

```yaml 
  cloudwatch-metricalarm:
    web-example-targets-alarm:
      description: "Service is running a lot of call per minute - Scale-UP"
      metricNamespace: AWS/ApplicationELB
      metricName: RequestCountPerTarget
      dimensions:  # metric data group by dimensions below
        LoadBalancer: web-lb
        TargetGroup: web-example-tg
      statistic: sum #maximum value of all requests | possible: average, sum, minimum, maximum
      period: 60
      evaluationPeriods: 1 # for 1 window/period
      comparisonOperator: GE # if greater-than-or-equal to
      threshold: 1200 # value of 10
      datapointsToAlarm: 1 # a single breach, to cause alarm
      treatMissingData: ignore 
      alarmActions: # when in alarm, take an autoscaling action on the following application-auto-scalig target
        autoscaling: web-example-step-policy-up-1
      okActions:
        autoscaling: web-example-step-policy-down-1
```
