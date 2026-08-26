# Standalone Task `standalone-task`

Starts a new task using the specified task definition. Check out AWS documentation for standalone task [here](https://awscli.amazonaws.com/v2/documentation/api/latest/reference/ecs/run-task.html).

> You can qualify the resource name with a provider prefix or supply the provider name in the standard `provider` field of the resource to indicate the provider to use. If neither is supplied, the `main` is used as the default provider name.

| Field                  | Description                                                                                                                                                                                                     | DataType                             | Required | Default |
| ---------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------ | -------- | ------- |
| `launchType`           | The infrastructure that you run your service on. Valid values: `CAP`, `EC2`, `FARGATE`                                                                                                                          | `string`                             | Yes      |         |
| `capacityProviders`    | The capacity provider strategy to use for the service. See [ECSCapacityProviderStrategy](#ecscapacityproviderstrategy)                                                                                          | `[]ECSCapacityProviderStrategy`      | Yes      |         |
| `taskDefName`          | Specify the task definition for the tasks in the service to use                                                                                                                                                 | `string`                             | Yes      |         |
| `clusterName`          | The name of the cluster that you run your service on                                                                                                                                                            | `string`                             | Yes      |         |
| `networkConfiguration` | Only when network mode in the taskdef is `awsvpc`. The network configuration for the task. See [StandaloneTaskNetworkConfiguration](#standalonetasknetworkconfiguration)                                        | `StandaloneTaskNetworkConfiguration` | No       |         |
| `timeoutSeconds`       | The wait time in seconds for the task to complete. Specify `0` to not wait for task completion.                                                                                                                 | `int`                                | No       | 600     |
| `concurrent`           | Enable concurrency for this task. If `true`, starting a new instance of this task will not stop other running instances.                                                                                        | `int`                                | No       | `false` |
| `overrides`            | A list of container overrides in JSON format that specify the name of a container in the specified task definition and the overrides it should receive. See [StandaloneTaskOverrides](#standalonetaskoverrides) | `StandaloneTaskOverrides`            | No       |         |
| `tags`                 | A key value map of resource tags to be applied to this resource. The `GlobalTags` are always applied, any matching keys are overriden from `tags`                                                               | `map[string]string`                  | No       | `{}`    |
| `dependsOn`            | The `buildit` resources that this resource depends on in the context of the current execution. All resources that are listed in this section will be built before this; while destoryed after this              | `[]string`                           | No       | `[]`    |

---

## StandaloneTaskNetworkConfiguration

| Field                | Description                                                                                                         | DataType   | Required | Default    |
| -------------------- | ------------------------------------------------------------------------------------------------------------------- | ---------- | -------- | ---------- |
| `assignPublicIp`     | Determine if the task’s elastic network interface receives a public IP address. Valid values: `DISABLED`, `ENABLED` | `string`   | No       | `DISABLED` |
| `subnetNames`        | The IDs of the subnets associated with the task or service                                                          | `[]string` | No       |            |
| `securityGroupNames` | The IDs of the security groups associated with the task or service                                                  | `[]string` | No       |            |

---

## StandaloneTaskOverrides

| Field                | Description                                                               | DataType                             | Required | Default |
| -------------------- | ------------------------------------------------------------------------- | ------------------------------------ | -------- | ------- |
| `containerOverrides` | See [StandaloneTaskContainerOverrides](#standalonetaskcontaineroverrides) | `[]StandaloneTaskContainerOverrides` | No       |         |

---

## StandaloneTaskContainerOverrides

| Field         | Description                                                                                                          | DataType       | Required | Default |
| ------------- | -------------------------------------------------------------------------------------------------------------------- | -------------- | -------- | ------- |
| `name`        | The name of the container that receives the override                                                                 | `string`       | No       |         |
| `command`     | The command to send to the container that overrides the default command from the Docker image or the task definition | `[]string`     | No       |         |
| `environment` | The environment variables to send to the container. Type `{"name":"","value":""}`                                    | `[]TaskEnvVar` | No       |         |

---

Example:

```yaml
resources:
  standalone-task:
    example-config-db-foreign-refresh-st:
      launchType: CAP
      taskDefName: example-config-db-operations-td
      clusterName: example-cluster
      capacityProviders:
        - base: 1
          weight: 100
          name: FARGATE_SPOT
      networkConfiguration:
        subnetNames:
          - example-private-0-subnet
          - example-private-1-subnet
          - example-private-2-subnet
        securityGroupNames:
          - example-config-worker-svc-sg
      timeoutSeconds: 600
      overrides:
        containerOverrides:
          - name: service
            command: [yarn, db:shards:foreign-refresh]
            environment:
              - name: example
                value: test
```
