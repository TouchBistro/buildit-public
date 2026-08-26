# Lambda Function `lambda-function`

This resources creates & updates a lambda function using the `main` provider.

Check out AWS documentation for lambda [here](https://awscli.amazonaws.com/v2/documentation/api/latest/reference/lambda/index.html#description).

| Field                | Description                                                                                                                                                                                                                                                                                                                                                                                                               | DataType            | Required | Default    |
| -------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------- | -------- | ---------- |
| Name (Resource Name) | The resource name is used as the name of the lambda function                                                                                                                                                                                                                                                                                                                                                              | `string`            | Yes      |            |
| `description`        | The description of the lambda function                                                                                                                                                                                                                                                                                                                                                                                    | `string`            | No       | ""         |
| `code`               | see [Code](#code-code) section for more details.                                                                                                                                                                                                                                                                                                                                                                          | `code{}`            | Yes      |            |
| `role`               | The name of the execution IAM Role. Must be supplied in the same AWS provider.                                                                                                                                                                                                                                                                                                                                            | `string`            | Yes      |            |
| `publish`            | Indicates if the change to the `$LATEST` version of this lambda function are to be published as well                                                                                                                                                                                                                                                                                                                      | `bool`              | No       | `false`    |
| `forcePublish`       | This is a `buildit` special field. This forces a`publish-version` operation even if no diffs are detected between the supplied definition and the currently deployed `$LATEST` version. `buildit` does not currently look for diffs between the supplied definition & the highest published version; so to avoid no publish when no changes made to `$LATEST` in the current change-set, use `forcePublish` set to `true` | `bool`              | No       | `false`    |
| `policy`             | see [Policy](#policy-policy) section for more details                                                                                                                                                                                                                                                                                                                                                                     | `policy{}`          | No       |            |
| `architectures`      | A list of instruction-set architectures the function supports. Values can be `arm64` & `x86_64`                                                                                                                                                                                                                                                                                                                           | `[]string`          | No       | `[x86_64]` |
| `environment`        | A map of environment variable names & their values.                                                                                                                                                                                                                                                                                                                                                                       | `map[string]string` | No       | `{}`       |
| `secrets`            | A map of environment variable names & secrets manager name:key. These secrets will be fetched during the Normalize phase of buildit. If a secret does not exist, or if it is not accessible to the buildit `main` provider, it will be quietly skipped. Any keys that already exists in the environment map will be overwritten with the value from the secrets section                                                   | `map[string]string` | No       | `{}`       |
| `ephemeralStorage`   | The amount of ephemeral storage allocated to the lambda function. Default is 512 MB.                                                                                                                                                                                                                                                                                                                                      | `int32`             | No       | 512        |
| `memory`             | The amount of memory to be allocated to the lambda function. Default is 256 MB.                                                                                                                                                                                                                                                                                                                                           | `int32`             | No       | 256        |
| `type`               | The package type for this lambda, Only `Zip` or `Image` are allowed.                                                                                                                                                                                                                                                                                                                                                      | `string`            | No       | `Image`    |
| `imageOverrides`     | see [imageOverrides](#image-overrides-imageoverrides) section for more details.                                                                                                                                                                                                                                                                                                                                           | `imageOverrides{}`  | No       |            |
| `fileSystem`         | Configure connection to an AWS EFS file system. see [fileSystem](#file-system-filesystem) section for more details.                                                                                                                                                                                                                                                                                                       | `imageOverrides{}`  | No       |            |
| `timeoutSeconds`     | The exeuction timeout in seconds. Allowed range is 3 to 900 seconds.                                                                                                                                                                                                                                                                                                                                                      | `int32`             | No       | 3          |
| `hanlder`            | The name of the handler function for this lambda function. This field is only used when package type is `Zip`                                                                                                                                                                                                                                                                                                             | `string`            | Yes\*    |            |
| `runtime`            | Supported runtime string for the lambda; See details of currently supported values [here](https://docs.aws.amazon.com/lambda/latest/dg/lambda-runtimes.html). This field is only used when package type is `Zip`                                                                                                                                                                                                          | `string`            | Yes\*    |            |
| `layers`             | The list of layers (layer references) to be added to this lambda functions. A layer must already exist in the provider scope before it can be referenced in the `lambda-function` resource. This field is only used when package type is `Zip`. See [Layer](#layer-layer) section for more details                                                                                                                        | `[]layer{}`         | No       | `[]`       |
| `logging`            | Configure log collection for this lambda with AWS Cloud Watch logs. see [Logging Configuration](#logging-configuration-logging) section for more details.                                                                                                                                                                                                                                                                 | `logging{}`         | No       | `{}`       |
| `vpc`                | see [VPC](#vpc-config-vpc) section for more details.                                                                                                                                                                                                                                                                                                                                                                      | `vpc{}`             | No       | `{}`       |
| `aliases`            | see [Aliases](#aliases-aliases) section for details.                                                                                                                                                                                                                                                                                                                                                                      | `[]alias{}`         | No       | `[]`       |
| `terminateRecursion` | Enable recursion protection. If `true`, automatically detects and stops infinite recursive loops involving your functions and supported AWS services                                                                                                                                                                                                                                                                      | `bool`              | No       | `true`     |
| `functionUrl`        | Lambda Function URL for the `$LATEST` unpublished revision. see [Function Url](#function-url-functionurl) section for more details                                                                                                                                                                                                                                                                                        | `functionUrl{}`     | No       | `{}`       |
| `dependsOn`          | The `buildit` resources that this resource depends on in the context of the current execution. All resources that are listed in this section will be built before this; while destoryed after this                                                                                                                                                                                                                        | `[]string`          | No       | `[]`       |
| `tags`               | A key value map of resource tags to be applied to this lambda-function resource. The `GlobalTags` are always applied, any matching keys are overriden from `tags`                                                                                                                                                                                                                                                         | `map[string]string` | No       | `{}`       |

## Policy `policy`

The resource-based policy for the lambda function to allow a principal from another AWS account or a service to invoke the lambda function. Policy can be applied to a function or an alias. `buildit` does not support managing policies for lambda versions directly. `buildit` does not require a `Resource` to be supplied as it is set to the correesponding object, lambda function or alias depending on where the `policy` is defined.

| Field       | Description                                                                                                                                                                                                                                                | DataType            | Required | Default |
| ----------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------- | -------- | ------- |
| `sid`       | statement id for the policy statement                                                                                                                                                                                                                      | `string`            | Yes      | `nil`   |
| `principal` | A single principal to be supplied, use map syntax, e.g `"*":` for wildcard `*` principal. Else use `AWS:<arn>` for a principal (user, role, etc) or service. When service is used, also supply a condition to restrict the access to the intended services | `map[string]string` | Yes      | `nil`   |
| `effect`    | affect can be "Allow" or "Deny"                                                                                                                                                                                                                            | `string`            | Yes      | `nil`   |
| `action`    |                                                                                                                                                                                                                                                            | `string`            | No       | `nil`   |
| `condition` | Policy conditions to limit the permission to a certain resource ARN or service. The policy document json syntax is used to supply `<condition> -> <field>: <value>` see examples below.                                                                    | No                  | `nil`    |

Example 1 : allow only the supplied account principals with required permissions to invoke this function

```yaml
policy:
  statement:
    [
      {
        sid: stmt3,
        effect: Allow,
        principal: { AWS: arn:aws:iam::123456789012:root },
        action: lambda:InvokeFunctionUrl,
        condition: { StringEquals: { lambda:FunctionUrlAuthType: AWS_IAM } },
      },
    ]
```

Example 2: allow anyone to invoke lambda. Make it public

```yaml
policy:
  statement:
    [
      {
        sid: stmt3,
        effect: Allow,
        principal: { "*" },
        action: lambda:InvokeFunctionUrl,
        condition: { StringEquals: { lambda:FunctionUrlAuthType: NONE } },
      },
    ]
```

## Code `code`

This section configures how the code for the lambda function is packaged and supplied. If the package type for the lambda function is `Image` a fully-qualified URL of the image repository must be supplied. When updating a lambda function, the Image URL is used to determine if the package has changed.

For a `Zip` package type, the location of the package can be supplied using the S3 bucket name, object key & version (optional). `buildit` only supports configuring lambda functions using an S3 location. When updating, `buildit` fetches the source S3 object metadata (SHA-256 checksum & size in bytes), to compare it with the existing code package supplied by AWS.

> Additional checksums are an Optional features for AWS S3 objects. To ensure `buildit` does not flag an S3 object as different on every `apply`, enable "additional checksum with SHA-256 function" when pushing the lambda pakcage Zip archives to the source S3 bucket.

| Field     | Description                                                                                                                                                                               | DataType | Required | Default |
| --------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------- | -------- | ------- |
| `image`   | The container image repository URI pointing to the image to be used as the code package when the `type` of the lambda is `Image`. This field is required when `type` attribute is `Image` | `string` | Yes\*    |         |
| `bucket`  | The S3 bucket name for the `zip` package. Required when lambda function `type` attributes is `Zip` (not supported yet)                                                                    | `string` | Yes\*    |         |
| `key`     | The S3 key for the `zip` package. Required when lambda function `type` attributes is `Zip` (not supported yet)                                                                            | `string` | No       |         |
| `version` | The S3 version name for the `zip` package (not supported yet)                                                                                                                             | `string` | No       |         |

Example: Lambda function with code provided as a Zip package from S3.

```yaml

resources:
  lambda-function:
    test-fn:
      type: Zip
      description: a test lambda function with code package as Zip
      code:
        bucket: lambda-zip-artifacts
        key: lambda/test-func/${gitSHA}.zip
       :
       :
```

## Image Overrides `imageOverrides`

Container image configuration that override the values in the container image Dockerfile.

| Field        | Description                                                     | DataType   | Required | Default |
| ------------ | --------------------------------------------------------------- | ---------- | -------- | ------- |
| `entrypoint` | The entrypoints to override for the container                   | `[]string` | No       | `[]`    |
| `command`    | The command line parameters that are supplied to the entrypoint | `[]string` | No       | `[]`    |
| `workingDir` | The overriden working directory.                                | `string`   | No       | nil     |

## File System `fileSystem`

Configure a connection to an AWS EFS file system.

| Field            | Description                                                                   | DataType | Required | Default |
| ---------------- | ----------------------------------------------------------------------------- | -------- | -------- | ------- |
| `name`           | The EFS access point to connect to: an access point ARN, a physical ID (`fsap-*`), or a name. A name matches the access point created by a buildit [`efs-filesystem`](./efs_filesystem.md) resource (by its nested access point `name`) or an access point created outside buildit that carries a matching `Name` tag. | `string` | No       | `""`    |
| `localMountPath` | The path where the function can access the file system, starting with `/mnt/` | `string` | No       | `""`    |

## Logging Configuration `logging`

Define Cloud Watch logging configuration for the lamba function.

| Field            | Description                                                                                                                                      | DataType | Required | Default |
| ---------------- | ------------------------------------------------------------------------------------------------------------------------------------------------ | -------- | -------- | ------- |
| `logGroup`       | An existing cloud watch log group to send the logs for this lambda function                                                                      | `string` | Yes      |         |
| `format`         | The format in which that logs are collected, Valid values are `Text` or `JSON`                                                                   | `string` | No       | `Text`  |
| `logLevel`       | When `format` is `JSON`, configure the log break-level for application logs. Valid values are `TRACE`, `DEBUG`, `INFO`, `WARN`, `ERROR`, `FATAL` | `string` | No       | `INFO`  |
| `systemLogLevel` | When `format` is `JSON`, configure the log break-level for system logs. Valid values are `DEBUG`, `INFO`, `WARN`                                 | No       | `INFO`   |

## Layer `layer`

A layer object is used to provide reference a lambda layer.

| Field     | Description                                                                                                                                             | DataType | Required | Default |
| --------- | ------------------------------------------------------------------------------------------------------------------------------------------------------- | -------- | -------- | ------- |
| `arn`     | The AWS `arn` of the layer, without the version segement                                                                                                | `string` | No       |         |
| `version` | The version number of the layer                                                                                                                         | `string` | No       | `[]`    |
| `name`    | The layer name for the layer. This can only be used when the layer is accessible by the `main` `buildit` provider (or, the current AWS Account context) | `string` | No       | `[]`    |

## VPC Config `vpc`

Defines the VPC configuration for a lambda function to allow network connectivity to other resources in the VPC, e.g. databases, caches or network filesystems. If supplied the lambda function will be attached to the supplied VPC. `buildit` does not currently support setting `Ipv6AllowedForDualStack` setting. This

| Field            | Description                                                                                                             | DataType | Required | Default |
| ---------------- | ----------------------------------------------------------------------------------------------------------------------- | -------- | -------- | ------- |
| `name`           | The VPC namein the same account as the lambda function to                                                               | `string` | Yes      |         |
| `securityGroups` | List of security group names to restrict access to VPC resources from the lambda function                               | `string` | No       | `[]`    |
| `subnets`        | List of subnets from the VPC to place the lambda function in. At least one is required when supplying VPC configuration | `string` | Yes      |         |

## Aliases `aliases`

Creates an `alias` for the un-published `$LATEST` or any other published version of the lambda function.

| Field         | Description                                                                                                                                                                                                                                                                                                                                                                                                                    | DataType        | Required | Default |
| ------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | --------------- | -------- | ------- |
| `name`        | The name for the alias                                                                                                                                                                                                                                                                                                                                                                                                         | `string`        | Yes      |         |
| `description` | The description of the alias                                                                                                                                                                                                                                                                                                                                                                                                   | `string`        | No       | ""      |
| `version`     | The lambda function version number to create the alias for. Supply `$LATEST` to create alias for the un-published version.<br/><br/>A `buildit` special value of `$HIGHEST` can be used to create the alias for the currently highest version of the function. This can be used to keep an alias always ponting to the latest published version of a lambda function, without the need of explicitly specifying it as a value. | `string`        | Yes      | ""      |
| `functionUrl` | see [Function Url](#function-url-functionurl) for more details                                                                                                                                                                                                                                                                                                                                                                 | `functionUrl{}` | No       |         |

## Function URL `functionUrl`

Defines a lambda function URL configuration for an un-published `$LATEST` version, or a specific `alias`.

> Currently `buildit` does not support setting `CORS` configuration. Auth type is always set to `NONE`

| Field        | Description                                                                                                                                                                                                                                                                                                                                                                 | DataType | Required | Default    |
| ------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------- | -------- | ---------- |
| `invokeMode` | This value must be set to `buffered` OR `response_stream` (case insensitive). This controls how lambda service invokes the function. With `bufferred` invocation, the results of the function are available when the payload is complete. The limit on the response size is 6 MB. With `response_stream` the results of the invocation are streamed and the limit is 20 MB. | `string` | No       | `buffered` |

### Example spec

```yaml
resources:
  lambda-fuction:
    example-lambda:
      description: my example lambda function
      code:
        image: 123456789012.dkr.ecr.us-east-1.amazonaws.com/fine-func:${gitsha}
      role: example-role
      architectures: [x86_64]
      environment:
        KEY_1: value1
        KEY_2: value2
        KEY_3: value3
      type: image
      publish: true
      forcePublish: false
      ephemeralStorage: 512
      memory: 256
      timeoutSeconds: 60
      functionUrl:
        invokeMode: buffered
      policy:
        statement:
          [
            {
              sid: stmt4,
              effect: Allow,
              principal: { "*" },
              action: lambda:InvokeFunctionUrl,
              condition: { StringEquals: { lambda:FunctionUrlAuthType: NONE } },
            },
          ]
      vpc:
        name: example-vpc
        securityGroups:
          - example-sg
        subnets:
          - example-subnet-1d
          - example-subnet-1c
      aliases:
        - name: production
          description: production alias
          version: 5
          functionUrl:
            invokeMode: buffered
          policy:
            statement:
              [
                {
                  sid: stmt3,
                  effect: Allow,
                  principal: { AWS: arn:aws:iam::123456789012:root },
                  action: lambda:InvokeFunctionUrl,
                  condition:
                    { StringEquals: { lambda:FunctionUrlAuthType: AWS_IAM } },
                },
              ]
        - name: staging
          description: staging alias
          version: $HIGHEST
          functionUrl:
            invokeMode: buffered
        - name: development
          description: development alias
          version: $LATEST
          functionUrl:
            invokeMode: buffered
      tags:
        Name: example-lambda
        group: dev
        owner: team1
      dependsOn:
        - example-role
        - example-sg
```
