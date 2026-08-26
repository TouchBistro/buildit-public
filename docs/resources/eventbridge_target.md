# EventBridge Target `targets`

Child resource of [EventbridgeRule](./eventbridge_rule.md). Attach a target to an eventbridge rule.

> You can qualify the resource name with a provider prefix or supply the provider name in the standard `provider` field of the resource to indicate the provider to use. If neither is supplied, the `main` is used as the default provider name.

Check out AWS documentation for EventBridge Target [here](https://awscli.amazonaws.com/v2/documentation/api/latest/reference/events/put-targets.html).


| Field | Description | DataType | Required | Default |
|-------|-------------|----------|----------|---------|
| `id` | The `buildit` ID of the target | `string` | Yes |  |
| `role` | The Amazon Resource Name (ARN) of the IAM role to be used for this target when the rule is triggered| `string` | Yes | |
| `targetResource` | The resource name for the target | `string` | Yes |  |
| `type` | The type of target. Valid values: `ecs`,`api`,`sfn`,`lambda`,`firehose` | `string` | No | `ecs` |
| `ecsTask` | Only for type `ecs`. Declared additional configs for ECS task. See [ECSTask](#ecstask) | `EventBridgeEcsTarget` | No |  |
| `httpParameters` | Only for type `api`. Declared additional configs for API targets. See [EventBridgeApiTarget](#eventbridgeapitarget) | `[]EventBridgeApiTarget` | No |  |
| `input` | For `sfn`, `lambda`, and `firehose` types. Raw eventbridge target input configuration. See [EventBridgeGenericInput](#eventbridgegenericinput) for more details | `[]EventBridgeGenericInput` | No |  |
|`tags`|A key value map of resource tags to be applied to this resource. The `GlobalTags` are always applied, any matching keys are overriden from `tags`|`map[string]string`|No|`{}`|
|`dependsOn`|The `buildit` resources that this resource depends on in the context of the current execution. All resources that are listed in this section will be built before this; while destoryed after this|`[]string`|No|`[]`|

Example: Eventbridge rule targeting lambda and ecs

```yaml
resources:
  eventbridge-rule:
    example-test-rule:
      ...
      targets:
        - id: example-pos-partner-feed-cron
          role: example-pos-partner-feed-eventbridge-role
          targetResource: example-cluster
          ecsTask:
            launchType: CAP
            taskCount: 1
            taskDefName: example-pos-partner-feed-td
            subnetNames:
            - example-private-0-subnet
            - example-private-1-subnet
            - example-private-2-subnet
            securityGroupNames:
             - example-pos-partner-feed-svc-sg
        - id: kitchensink
          type: lambda
          targetResource: example-kitchen-sink-lambda
```

Example: Eventbridge rule targeting a Firehose delivery stream with input transformation

```yaml
resources:
  eventbridge-rule:
    my-event-rule:
      ...
      targets:
        - id: my-firehose-target
          type: firehose
          role: my-eventbridge-firehose-role
          targetResource: my-delivery-stream
          input:
            template: '{"eventId": "<eventId>"}'
            pathsMap:
              eventId: "$.id"
```

---
## ECSTask
Extra configuration for `ecs` target type
| Field | Description | DataType | Required | Default |
|-------|-------------|----------|----------|---------|
| `taskCount` | The number of tasks to create | `int32` | No | `1` |
| `taskDefName` | The name of the task definition | `string` | Yes |  |
| `launchType` | Specifies the launch type on which your task is running. Valid values: `EC2`,`Fargate`,`CAP` | `string` | No | `CAP` |
| `capacityProviders` | For `launchType` `CAP`. The capacity provider strategy to use for the task. See [ECSCapacityProviderStrategy](./ecs_service.md#ecscapacityproviderstrategy) | `[]ECSCapacityProviderStrategy` | Yes |  |
| `platformVersion` | Specifies the platform version for the task | `string` | Yes |  |
| `assignPublicIp` | Specifies whether the task’s elastic network interface receives a public IP address. Valid values: `ENABLED`,`DISABLED` | `string` | No |  |
| `subnetNames` | List of subnets associated with the task | `[]string` | No |  |
| `securityGroupNames` | List of security groups associated with the task | `[]string` | No |  |
| `overrides` | Task overrides are a special case for ecs runTask type tagets & it's converted to target invocation `input`. See [TaskOverrides](#taskoverrides) | `TaskOverrides` | Yes |  |

## TaskOverrides
Represents the task overrides for a runtask operation
| Field | Description | DataType | Required | Default |
|-------|-------------|----------|----------|---------|
| `containerOverrides` | List of containers in the task. See [TaskContainerOverrides](#taskcontaineroverrides) | `[]TaskContainerOverrides` | No | |
| `cpu` | CPU unit reserved for the task | `int32` | No | |
| `memory` | Memory unit reserved for the task | `int32` | No | |
Example:

```yaml
      overrides:
        containerOverrides:
          - name: service
            command: [yarn, db:shards:foreign-refresh]
```

## TaskContainerOverrides
Defines the container overrides
| Field | Description | DataType | Required | Default |
|-------|-------------|----------|----------|---------|
| `name` | Name of the container | `string` | No |  |
| `command` | The command that is passed to the container. If there are multiple arguments, each argument should be a separated string in the array | `[]string` | No |  |
| `environment` | The environment variables to send to the container. Type `{"name":"","value":""}` | `[]TaskEnvVar` | No | |
| `cpu` | CPU unit reserved for the container | `int32` | No |  |
| `memory` | Amount of memory (in MiB) present to the container | `int32` | No |  |
| `memoryReservation` | The soft limit (in MiB) of memory to reserve for the container | `int32` | No |  |

---

## EventBridgeApiTarget
Extra configuration for `api` target type
| Field | Description | DataType | Required | Default |
|-------|-------------|----------|----------|---------|
| `headerParameters` | The headers that need to be sent as part of request invoking an API | `map[string]string` | No |  |
| `pathParameters` | The path parameter values to be used to populate path wildcards (“*”) | `[]string` | No |  |
| `queryStringParameters` | The query string keys/values that need to be sent as part of request invoking an API | `map[string]string` | No |  |
| `payload` | The request payload | `string` | No |  |

## Firehose Target
Extra configuration for `firehose` target type

| Field | Description | DataType | Required | Default |
|-------|-------------|----------|----------|---------|
| `targetResource` | The name or ARN of the Kinesis Data Firehose delivery stream | `string` | Yes | |
| `role` | ARN of the IAM role that grants EventBridge permission to put records to the delivery stream | `string` | Yes | |
| `input` | Optional input transformation applied before records are sent to Firehose. See [EventBridgeGenericInput](#eventbridgegenericinput) | `EventBridgeGenericInput` | No | |

## EventBridgeGenericInput
Represents the input configuration for `sfn`, `lambda`, and `firehose` target types.

To pass the **full matched event** to the target without any transformation, omit the `input` field entirely. Specify `input` only when you need to customise what is delivered.

Exactly one of `value`, `jsonPath`, or (`template` + `pathsMap`) may be set at a time.

| Field | Description | DataType | Required | Default |
|-------|-------------|----------|----------|---------|
| `value` | A static JSON string passed to the target verbatim. Nothing from the matched event is forwarded | `string` | No |  |
| `jsonPath` | A JSON path expression (e.g. `$.detail`) used to extract a part of the matched event and pass only that portion to the target | `string` | No |  |
| `template` | A template string with `<placeholder>` tokens that are replaced with values extracted via `pathsMap` before being sent to the target | `string` | No |  |
| `pathsMap` | Map of placeholder name to JSON path, used together with `template` to extract values from the matched event | `map[string]string` | No |  |
