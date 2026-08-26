# EventBridge Rule `eventbridge-rule`

This resource creates an EventBridge Rule.

> You can qualify the resource name with a provider prefix or supply the provider name in the standard `provider` field of the resource to indicate the provider to use. If neither is supplied, the `main` is used as the default provider name.

Check out AWS documentation for EventBridge Rule [here](https://awscli.amazonaws.com/v2/documentation/api/latest/reference/events/put-rule.html).

| Field | Description | DataType | Required | Default |
|-------|-------------|----------|----------|---------|
|Name (Resource Name)|The resource name is used as the name of the eventbridge rule|`string`|Yes||
| `description` | Description of the rule. | `string` | No |  |
| `eventbusName` | The name of the event bus to associate with this rule| `string` | No | `default` |
| `scheduleExpression` | The scheduling expression | `string` | No |  |
| `eventPattern` | The event pattern | `string` | No |  |
| `enabled` | Indicated if the rule is activated | `bool` | No | `true` |
| `targets` | List of targets for this rule. See [EventBridgeTarget](./eventbridge_target.md) for more details | `[]map[string]string` | No |  |
|`tags`|A key value map of resource tags to be applied to this resource. The `GlobalTags` are always applied, any matching keys are overriden from `tags`|`map[string]string`|No|`{}`|
|`dependsOn`|The `buildit` resources that this resource depends on in the context of the current execution. All resources that are listed in this section will be built before this; while destoryed after this|`[]string`|No|`[]`|

Example:

```yaml
  eventbridge-rule:
    example-task-director-cron-pos-eod-rule:
      tags:
        profileid: pos-eod
      description: trigger example-task-director-cron-pos-eod
      enabled: true
      eventbusName: default
      scheduleExpression: cron(1 * ? * * *)
      targets: 
        - id: example-task-director-cron-pos-eod-svc
          role: example-task-director-cron-eventbridge-role
          targetResource: example-cluster 
          ecsTask:
            launchType: CAP
            taskCount: 1
            taskDefName: example-task-director-cron-pos-eod-td
            subnetNames: 
             - example-private-0-subnet
             - example-private-1-subnet
             - example-private-2-subnet
            securityGroupNames:
             - example-task-director-svc-sg
      dependsOn: 
        - example-task-director-cron-pos-eod-td
```      