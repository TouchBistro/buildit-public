# MSK Worker Configuration `msk-worker-configuration`

This resource creates an Amazon MSK Connect **worker configuration**, which defines the `connect-distributed.properties` used by MSK Connect workers. It allows you to define the `content` of the configuration and reference secrets securely.

> You can qualify the resource name with a provider prefix or supply the provider name in the standard `provider` field of the resource to indicate the provider to use. If neither is supplied, `main` is used as the default provider name.

Check out AWS documentation for worker configuration [here](https://awscli.amazonaws.com/v2/documentation/api/latest/reference/kafkaconnect/create-worker-configuration.html).

| Field                | Description                                                                                                                                                                                        | DataType            | Required | Default |
| -------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------- | -------- | ------- |
| Name (Resource Name) | The resource name is used as the name of the MSK worker configuration                                                                                                                              | `string`            | Yes      |         |
| `description`        | A description for the worker configuration                                                                                                                                                         | `string`            | No       | `""`    |
| `secrets`            | A map of key-secretsManagerName:Key. Any keys that already exists in the `content` map will be overwritten with the value from the secrets section                                               | `map[string]string` | No       | `{}`    |
| `content`            | A map of key-value pairs that will be converted into the contents of the worker configuration                                                                                                      | `map[string]string` | No       | `{}`    |
| `dependsOn`          | The `buildit` resources that this resource depends on in the context of the current execution. All resources that are listed in this section will be built before this; while destoryed after this | `[]string`          | No       | `[]`    |
| `tags`               | A key value map of resource tags to be applied to this resource. The `GlobalTags` are always applied, any matching keys are overriden from `tags`                                                  | `map[string]string` | No       | `{}`    |

## Example

```yaml
resources:
  msk-worker-configuration:
    test-config:
      description: Testing /buildit/
      secrets:
        test: test-secret:secret-001
      content:
        key.converter: org.apache.kafka.connect.json.JsonConverter
        value.converter: org.apache.kafka.connect.json.JsonConverter
        consumer.max.partition.fetch.bytes: 2000
        consumer.max.poll.records: 3000
      tags:
        created: manually-test
```
