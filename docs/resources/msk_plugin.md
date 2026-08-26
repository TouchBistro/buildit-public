# MSK Plugin `msk-plugin`

This resource creates a custom **MSK Connect plugin**. A plugin is a ZIP or JAR containing connector code and dependencies. This resource specifies where the plugin is stored in S3 and any optional metadata or tags.

> You can qualify the resource name with a provider prefix or supply the provider name in the standard `provider` field of the resource to indicate the provider to use. If neither is supplied, `main` is used as the default provider name.

Check out AWS documentation for MSK Connect custom plugins [here](https://awscli.amazonaws.com/v2/documentation/api/latest/reference/kafkaconnect/create-custom-plugin.html).

| Field                | Description                                                                                                                                                                                     | DataType            | Required | Default |
| -------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------- | -------- | ------- |
| Name (Resource Name) | The resource name is used as the name of the MSK plugin                                                                                                                                         | `string`            | Yes      |         |
| `description`        | A human-readable description of the plugin                                                                                                                                                      | `string`            | No       | `""`    |
| `deploymentTimeout`  | The time in seconds before the deployment time out                                                                                                                                              | `int`               | No       | `60`    |
| `code`               | See [Code](#code-code) section for more details.                                                                                                                                                | `code{}`            | Yes      |         |
| `dependsOn`          | The `buildit` resources that this resource depends on in the context of the current execution. All resources that are listed in this section will be built before this and destroyed after this | `[]string`          | No       | `[]`    |
| `tags`               | A map of AWS tags to apply to the plugin resource                                                                                                                                               | `map[string]string` | No       | `{}`    |

## Code `code`

This section configures how the code for the custom plugin is supplied. Either `JAR` or `ZIP` package type.

> Additional checksums are an Optional features for AWS S3 objects. To ensure `buildit` does not flag an S3 object as different on every `apply`, enable "additional checksum with SHA-256 function" when pushing the code package to the source S3 bucket.

| Field     | Description                              | DataType | Required | Default |
| --------- | ---------------------------------------- | -------- | -------- | ------- |
| `bucket`  | The S3 bucket name for the code package  | `string` | Yes      |         |
| `key`     | The S3 key for the code package          | `string` | Yes      |         |
| `version` | The S3 version name for the code package | `string` | No       |         |

## Example

```yaml
resources:
  msk-plugin:
    test-plugin:
      description: Testing /buildit/
      code:
        bucket: arn:aws:s3:::example-debezium-poc
        key: confluentinc-kafka-connect-s3-10.5.21.zip
      tags:
        created: manually-test
```
