# Application Autoscaling `autoScaling`

Child resource of [EcsService](./ecs_service.md). Setup autoscaling for a service.

| Field | Description | DataType | Required | Default |
|-------|-------------|----------|----------|---------|
|`suspend`|Indicates whether scaling is disabled |``bool``|No| |
|`minCapacity`|Minimum capacity limit for scale-ins |`int32`|No| |
|`maxCapacity`|Maximum capacity limit for scale-outs |`int32`|No| |
|`policies`|List of scaling policies to apply. See [ApplicationAutoScalingPolicy](#applicationautoscalingpolicy) |`[]ApplicationAutoScalingPolicy`|No| |
---
## ApplicationAutoScalingPolicy
Creates or updates a scaling policy for an Auto Scaling group. For scaling based on configurable metrics see [here](https://awscli.amazonaws.com/v2/documentation/api/latest/reference/autoscaling/put-scaling-policy.html). For scaling based on a schedule, see [here](https://awscli.amazonaws.com/v2/documentation/api/latest/reference/application-autoscaling/put-scheduled-action.html).

| Field | Description | DataType | Required | Default |
|-------|-------------|----------|----------|---------|
|`policyName`|Name of the autoscaling policy |`string`|Yes| |
|`policyType`|Type of policy. Valid values: `target-tracking`, `step-scaling`, `scheduled`|`string`|Yes| |
|`coolDown`|Scale in, scale out thresholds for `target-tracking` and `step-scaling`, in seconds |`int32`|No| |
|`disableScaleIn`|Indicates whether scaling in by the target tracking scaling policy is disabled. If scaling in is disabled, the target tracking scaling policy doesn’t remove instances from the Auto Scaling group |`bool`|No| |
|`targetMetricName`|The name of the metric. Valid values: `cpu`, `memory`, `request-count`, `custom` |`string`|No| |
|`targetMetricResource`|Target group name if `targetMetricName` is `request-count` |`string`|No| |
|`targetMetricValue`|Threshold for the target metric to scale |`float64`|No| |
|`targetCustomMetric`|If `targetMetricName` is `custom`, specify the custom metric. See [ApplicationAutoScalingCustomPolicyMetric](#applicationautoscalingcustompolicymetric) |`ApplicationAutoScalingCustomPolicyMetric`|No| |
|`scheduleStartTimeUTC`|If `policyType` is `scheduled`, set the schedule start time |`string`|No| |
|`scheduleEndTimeUTC`|If `policyType` is `scheduled`, set the schedule end time  |`string`|No| |
|`scheduleCronUTC`|If `policyType` is `scheduled`, set the schedule for this action in cron format |`string`|No| |
|`scheduleMin`|If `policyType` is `scheduled`, set the minimum capacity |`int32`|No| |
|`scheduleMax`|If `policyType` is `scheduled`, set the maximum capacity |`int32`|No| |
|`stepMetricAggregation`|If `policyType` is `step-scaling`, the aggregation type for the CloudWatch metrics. Valid values: `Minimum`, `Maximum`, and `Average`  |`string`|No| |
|`stepAdjustmentType`|If `policyType` is `step-scaling`, specifies how the scaling adjustment is interpreted. Valid values: `ChangeInCapacity`, `ExactCapacity`, and `PercentChangeInCapacity`  |`string`|No| |
|`stepMinAdjustment`|If `policyType` is `step-scaling`, the minimum value to scale by when the `stepAdjustmentType` is `PercentChangeInCapacity`  |`int32`|No| |
|`stepAdjustments`|If `policyType` is `step-scaling`, represents a set of adjustments that enable you to scale based on the size of the alarm breach. See [ApplicationAutoScalingStepAdjustment](#applicationautoscalingstepadjustment) |`[]ApplicationAutoScalingStepAdjustment`|No| |
---
## ApplicationAutoScalingCustomPolicyMetric
Represents a custom metric for scaling policy
| Field | Description | DataType | Required | Default |
|-------|-------------|----------|----------|---------|
|`name`|The cloudwatch metric name|`string`|Yes| |
|`namespace`|The cloudwatch metric namespace |`string`|Yes| |
|`statistic`|The statistic of the metric |`string`|Yes| |
|`unit`|The unit of the metric. For a complete list of the units that cloudWatch supports, see [MetricDatum](https://docs.aws.amazon.com/AmazonCloudWatch/latest/APIReference/API_MetricDatum.html) |`string`|No| |
|`dimensions`|The dimensions of the metric |`map[string]string`|No| |
---
## ApplicationAutoScalingStepAdjustment
| Field | Description | DataType | Required | Default |
|-------|-------------|----------|----------|---------|
|`lowerBound`|The lower bound for the difference between the alarm threshold and the CloudWatch metric|`float64`|No| |
|`upperBound`|The upper bound for the difference between the alarm threshold and the CloudWatch metric|`float64`|No| |
|`value`|The amount by which to scale, based on the specified adjustment type. A positive value adds to the current capacity while a negative number removes from the current capacity|`float64`|Yes| |

Example:

```yaml 
resources:
  ecs-service:
    example-service:
      ...
      autoScaling:
        minCapacity: 12
        maxCapacity: 40
        policies:
          # target-tracking
          - policyName: example-autoscaling-policy-1
            policyType: target-tracking
            coolDown: 300
            disableScaleIn: false
            targetMetricName: cpu
            targetMetricValue: 65
          # step-scaling
          - policyName: example-autoscaling-policy-2
            policyType: step-scaling
            coolDown: 60
            disableScaleIn: false
            stepMetricAggregation: average
            stepAdjustmentType: PercentChangeInCapacity
            stepMinAdjustment: 1
            stepAdjustments:
              - lowerBound: 0
                upperBound: 600
                value: 50
              - lowerBound: 600
                upperBound: 1200
                value: 100
              - lowerBound: 1200
                upperBound: 1800
                value: 150
              - lowerBound: 1800
                value: 200
          # scheduled
          - policyName: example-autoscaling-policy-3
            policyType: scheduled
            scheduleStartTimeUTC: 2022-06-13 19:30:00
            scheduleEndTimeUTC: 2047-01-01 00:00:00
            scheduleCronUTC: cron(10 8 ? * * *)
            scheduleMin: 0
            scheduleMax: 0
```
