## `MSKConnector`

Represents a configuration for an AWS MSK Connector, including networking, authentication, plugin, and scaling information.

Check out AWS documentation for MSK Connector [here](https://awscli.amazonaws.com/v2/documentation/api/latest/reference/kafkaconnect/create-connector.html).

| `Field`                  | Description                                                                                                                                                                                     | DataType            | Required | Default       |
| ------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------- | -------- | ------------- |
| `description`            | Description of the MSK connector                                                                                                                                                                | `string`            | No       |               |
| `type`                   | Connector type (`PROVISIONED`, `AUTOSCALING`)                                                                                                                                                   | `string`            | No       | `AUTOSCALING` |
| `capacity`               | Connector compute capacity and scaling settings. See [Capacity](#capacity-capacity) section for more details.                                                                                   | `Capacity`          | Yes      |               |
| `secrets`                | A map of key-secretsManagerName:Key. Any keys that already exists in the `connectorConfiguration` will be overwritten with the value from the secrets section                                   | `map[string]string` | No       |               |
| `connectorConfiguration` | A key-value map of connector configuration                                                                                                                                                      | `map[string]string` | No       |               |
| `cluster`                | Name of the associated Kafka cluster                                                                                                                                                            | `string`            | Yes      |               |
| `kafkaConnectVersion`    | Kafka Connect version to use                                                                                                                                                                    | `string`            | Yes      |               |
| `plugin`                 | Name of the custom plugin for the connector                                                                                                                                                     | `string`            | Yes      |               |
| `role`                   | Name of the IAM Role assigned to the connector                                                                                                                                                  | `string`            | Yes      |               |
| `deploymentTimeout`      | The time in seconds before the deployment time out                                                                                                                                              | `int`               | No       | `900`         |
| `logGroup`               | CloudWatch Log Group for connector logs                                                                                                                                                         | `string`            | No       |               |
| `workerConfiguration`    | Name of the worker configuration for the connector                                                                                                                                              | `string`            | Yes      |               |
| `dependsOn`              | The `buildit` resources that this resource depends on in the context of the current execution. All resources that are listed in this section will be built before this and destroyed after this | `[]string`          | No       | `[]`          |
| `tags`                   | A map of AWS tags to apply to the plugin resource                                                                                                                                               | `map[string]string` | No       | `{}`          |

> Internal read-only fields (`GlobalTags`, `_arn`, `_pluginArn`, etc.) and embedded `BaseResource` are not exposed in YAML and excluded from user-facing documentation.

---

## Capacity `Capacity`

Defines the compute and scaling configuration for the MSK Connector.

### Fields

| `Field`                 | Description                                                                             | DataType | Required | Default |
| ----------------------- | --------------------------------------------------------------------------------------- | -------- | -------- | ------- |
| `mcuCount`              | Number of MSK Connector microcontroller units (MCUs) allocated to each connector worker | `int32`  | Yes      |         |
| `workerCount`           | Number of workers (only for `PROVISIONED` type)                                         | `int32`  | Yes      |         |
| `maxWorkerCount`        | Max worker count (only for `AUTOSCALING` type)                                          | `int32`  | Yes      |         |
| `minWorkerCount`        | Min worker count (only for `AUTOSCALING` type)                                          | `int32`  | Yes      |         |
| `scaleInCpuPercentage`  | CPU threshold to scale in (only for `AUTOSCALING` type)                                 | `int32`  | Yes      |         |
| `scaleOutCpuPercentage` | CPU threshold to scale out (only for `AUTOSCALING` type)                                | `int32`  | Yes      |         |

---

## Example

```yaml
msk-connector:
  test-connector:
    description: Testing /buildit/
    type: PROVISIONED
    capacity:
      mcuCount: 2
      workerCount: 2
    secrets:
      test: test-secret:secret-002
    connectorConfiguration:
      "tasks.max": "1"
      "transforms": "unwrap"
      "transforms.unwrap.type": "io.debezium.transforms.ExtractNewRecordState"
      "database.port": "5432"
      "database.user": "[REDACTED]"
      "database.password": "[REDACTED]"
      "database.dbname": "shard003"
    cluster: test-cluster
    kafkaConnectVersion: 3.7.x
    plugin: test-plugin
    role: test-execution-role
    logGroup: /aws/msk/test
    workerConfiguration: test-config
    tags:
      created: manually-test
    dependsOn:
      - test-plugin
      - test-config
```
