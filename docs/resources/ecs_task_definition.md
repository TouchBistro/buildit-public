# ECS Task Definition `taskdef`

Create, update or destroy an ECS Task Definition.

> You can qualify the resource name with a provider prefix or supply the provider name in the standard `provider` field of the resource to indicate the provider to use. If neither is supplied, the `main` is used as the default provider name.

Check out AWS documentation for ecs task definition [here](https://awscli.amazonaws.com/v2/documentation/api/latest/reference/ecs/register-task-definition.html).

| Field                     | Description                                                                                                                                                                                        | DataType             | Required | Default                |
| ------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------- | -------- | ---------------------- |
| Name (Resource Name)      | The buildit resource name                                                                                                                                                                          | `string`             | Yes      |                        |
| `name`                    | Name of the task definition                                                                                                                                                                        | `string`             | Yes      |                        |
| `role`                    | The name of the IAM role that containers in this task can assume. All containers in this task are granted the permissions that are specified in this role                                          | `string`             | No       |                        |
| `executionRole`           | The name of the IAM role that grants the Amazon ECS container agent permission to make Amazon Web Services API calls on your behalf                                                                | `string`             | No       | `ecsTaskExecutionRole` |
| `networkMode`             | The Docker networking mode to use for the containers in the task. Valid values: `none`, `bridge`, `awsvpc`, and `host`                                                                             | `string`             | No       | `awsvpc`               |
| `pidMode`                 | The process namespace to use for the containers in the task. Valid values: `host` or `task`                                                                                                        | `string`             | No       |                        |
| `ipcMode`                 | The IPC resource namespace to use for the containers in the task. The valid values are `host`, `task`, or `none`                                                                                   | `string`             | No       |                        |
| `requiresCompatibilities` | The task launch types the task definition was validated against. Valid values: `EC2` , `FARGATE`, and `EXTERNAL`                                                                                   | `[]string`           | Yes      | `["EC2"]`              |
| `taskMemory`              | The amount (in MiB) of memory to present to the container. If your container attempts to exceed the memory specified here, the container is killed                                                 | `string`             | No       | `2048`                 |
| `taskCPU`                 | The number of cpu units reserved for the container                                                                                                                                                 | `string`             | No       | `1024`                 |
| `ephemeralStorage`        | The ephemeral storage settings to use for tasks run with the task definition                                                                                                                       | `int32`              | Yes      |                        |
| `runtimePlatform`         | The runtime platform configuration for the task. See [RuntimePlatform](#runtimeplatform) section for more details.                                                                                 | `RuntimePlatform`    | No       |                        |
| `containers`              | The list of containers in the task definition. See [Container](#container) section for more details.                                                                                               | `[]Container`        | Yes      | `[]`                   |
| `volumes`                 | The list of volumes attached to the containers. See [Container Volumes](#containervolumes) section for more details.                                                                               | `[]ContainerVolumes` | No       | `[]`                   |
| `tags`                    | A key value map of resource tags to be applied to this resource. The `GlobalTags` are always applied, any matching keys are overriden from `tags`                                                  | `map[string]string`  | No       | `{}`                   |
| `dependsOn`               | The `buildit` resources that this resource depends on in the context of the current execution. All resources that are listed in this section will be built before this; while destoryed after this | `[]string`           | No       | `[]`                   |

Example:

```yaml
resources:
  taskdef:
    td-example:
      tags:
        profileid: s3-file-deletion
      role: example-core-api-role
      executionRole: example-core-api-role
      networkMode: awsvpc
      requiresCompatibilities:
        - FARGATE
      taskMemory: 1024
      taskCPU: 512
      containers:
        - name: datadog-agent
          image: public.ecr.aws/datadog/agent:latest
          essential: true
          tagsAsLabels: true
          envVars:
            ECS_FARGATE: true
            DD_DOGSTATSD_TAG_CARDINALITY: orchestrator
            DD_CONTAINER_LABELS_AS_TAGS: '{"groupid": "groupid", "serviceid": "serviceid", "profileid": "profileid", "account": "account", "env": "env"}'
          secrets:
            DD_API_KEY: "EXAMPLE_NAMESPACE:DD_API_KEY::"
        - name: log-router
          image: 123456789012.dkr.ecr.us-east-1.amazonaws.com/example-aws-for-fluent-bit:master
          essential: true
          tagsAsLabels: true
          firelensConfiguration:
            type: fluentbit
            options:
              enable-ecs-log-metadata: true
              config-file-type: file
              config-file-value: /fluent-bit/configs/example-extra.conf
        - name: service
          image: 123456789012.dkr.ecr.us-east-1.amazonaws.com/example-core-api-service:${gitsha}
          essential: true
          command:
            - "node"
            - "./bin/s3_file_deletion"
          mountPoints:
            - sourceVolume: peach-conf
              containerPath: /app/config
            - sourceVolume: git-ssh-files
              containerPath: /app/.ssh
            - sourceVolume: cloudfront-conf
              containerPath: /etc/aws
              dependencyOrder:
          dependsOn:
            - container: config
              condition: success
          portMappings:
            - hostPort: 3000
              containerPort: 3000
              protocol: tcp
            - hostPort: 9229
              containerPort: 9229
              protocol: tcp
          healthcheck:
            command:
              - "CMD-SHELL"
              - "curl localhost:3000/healthcheck"
            interval: 10
            timeout: 5
            startPeriod: 20
            retries: 10
          envVars:
            DD_VERSION: ${gitsha}
            DD_SERVICE: example-core-api-cron-s3-file-deletion
            ...
          tagsAsLabels: true
          logConfiguration:
            logDriver: awsfirelens
            options:
                Name: datadog
                Host: http-intake.logs.datadoghq.com
                ...
            secretOptions:
              apikey: "EXAMPLE_NAMESPACE:DD_API_KEY::"
              ...
      volumes:
        - name: peach-conf
          type: host
        - name: git-ssh-files
          type: host
        - name: datadog-conf
          type: host
        - name: cloudfront-conf
          type: host

```

## Container

Describe the different containers that make up the task

| Field                   | Description                                                                                                                                                                                                     | DataType                   | Required | Default |
| ----------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------- | -------- | ------- |
| `name`                  | The name of a container                                                                                                                                                                                         | `string`                   | Yes      |         |
| `image`                 | The image used to start a container. This string is passed directly to the Docker daemon                                                                                                                        | `string`                   | Yes      |         |
| `cpu`                   | The number of cpu units reserved for the container                                                                                                                                                              | `int32`                    | Yes      |         |
| `memory`                | The amount (in MiB) of memory to present to the container                                                                                                                                                       | `int32`                    | Yes      |         |
| `memoryReservation`     | The soft limit (in MiB) of memory to reserve for the container. Under heavy contention, Docker attempts to keep the container memory to this soft limit                                                         | `int32`                    | Yes      |         |
| `portMappings`          | The list of port mappings for the container. See [ContainerPortMapping](#containerportmapping) section for more details                                                                                         | `[]ContainerPortMapping`   | No       |         |
| `healthcheck`           | The container health check command and associated configuration parameters for the container. See [ContainerHealthcheck](#containerhealthcheck) section for more details                                        | `ContainerHealthcheck`     | No       |         |
| `entrypoint`            | The entry point that’s passed to the container                                                                                                                                                                  | `[]string`                 | No       |         |
| `envVars`               | The environment variables to pass to a container                                                                                                                                                                | `map[string]string`        | No       |         |
| `secrets`               | The secrets to pass to the container                                                                                                                                                                            | `map[string]string`        | No       |         |
| `labels`                | A key/value map of labels to add to the container                                                                                                                                                               | `map[string]string`        | No       |         |
| `tagsAsLabels`          | If `true`, use parent task definition's `tags` as default for `labels`. Override any values declared in `labels`                                                                                                | `bool`                     | No       | `true`  |
| `dependsOn`             | The dependencies defined for container startup and shutdown. See [ContainerDependsOn](#containerdependson) section for more details                                                                             | `[]ContainerDependsOn`     | No       |         |
| `mountPoints`           | The mount points for data volumes in your container. See [ContainerMountPoints](#containermountpoints) section for more details                                                                                 | `[]ContainerMountPoints`   | No       |         |
| `logConfiguration`      | The log configuration specification for the container. See [ContainerLogging](#containerlogging) section for more details                                                                                       | `ContainerLogging`         | Yes      |         |
| `linuxParameters`       | Linux-specific modifications that are applied to the container. See [LinuxParameters](#linuxparameters) section for more details                                                                                | `ContainerLinuxParameters` | No       |         |
| `command`               | The command that is passed to the container. If there are multiple arguments, each argument should be a separated string in the array                                                                           | `[]string`                 | Yes      |         |
| `ulimits`               | A list of ulimits to set in the container. If a ulimit value is specified in a task definition, it overrides the default values set by Docker. See [ContainerUlimit](#containerulimit) section for more details | `[]ContainerUlimit`        | No       |         |
| `essential`             | If the essential parameter of a container is marked as `true` , and that container fails or stops for any reason, all other containers that are part of the task are stopped                                    | `bool`                     | No       | `true`  |
| `privileged`            | When this parameter is `true`, the container is given elevated privileges on the host container instance (similar to the root user)                                                                             | `bool`                     | No       |         |
| `firelensConfiguration` | The FireLens configuration for the container. See [FirelensConfiguration](#firelensconfiguration) section for more details                                                                                      | `FirelensConfiguration`    | Yes      |         |
| `workingDir`            | The working directory to run commands inside the container in                                                                                                                                                   | `string`                   | No       |         |

Example:

```yaml
resources:
  taskdef:
    td-example:
      ...
      containers:
        - name: datadog-agent
          image: public.ecr.aws/datadog/agent:latest
          essential: true
          tagsAsLabels: true
          envVars:
            ECS_FARGATE: true
            DD_DOGSTATSD_TAG_CARDINALITY: orchestrator
            DD_CONTAINER_LABELS_AS_TAGS: '{"groupid": "groupid", "serviceid": "serviceid", "profileid": "profileid", "account": "account", "env": "env"}'
          secrets:
            DD_API_KEY: "EXAMPLE_NAMESPACE:DD_API_KEY::"
        - name: log-router
          image: 123456789012.dkr.ecr.us-east-1.amazonaws.com/example-aws-for-fluent-bit:master
          essential: true
          tagsAsLabels: true
          firelensConfiguration:
            type: fluentbit
            options:
              enable-ecs-log-metadata: true
              config-file-type: file
              config-file-value: /fluent-bit/configs/example-extra.conf
        - name: service
          image: 123456789012.dkr.ecr.us-east-1.amazonaws.com/example-core-api-service:${gitsha}
          essential: true
          command:
            - "node"
            - "./bin/s3_file_deletion"
          mountPoints:
            - sourceVolume: peach-conf
              containerPath: /app/config
            - sourceVolume: git-ssh-files
              containerPath: /app/.ssh
            - sourceVolume: cloudfront-conf
              containerPath: /etc/aws
              dependencyOrder:
          dependsOn:
            - container: config
              condition: success
          portMappings:
            - hostPort: 3000
              containerPort: 3000
              protocol: tcp
            - hostPort: 9229
              containerPort: 9229
              protocol: tcp
          healthcheck:
            command:
              - "CMD-SHELL"
              - "curl localhost:3000/healthcheck"
            interval: 10
            timeout: 5
            startPeriod: 20
            retries: 10
          envVars:
            DD_VERSION: ${gitsha}
            DD_SERVICE: example-core-api-cron-s3-file-deletion
            ...
          tagsAsLabels: true
          logConfiguration:
            logDriver: awsfirelens
            options:
                Name: datadog
                Host: http-intake.logs.datadoghq.com
                ...
            secretOptions:
              apikey: "EXAMPLE_NAMESPACE:DD_API_KEY::"
              ...
      ...

```

## ContainerVolumes

Creates or updates a metric filter and associates it with the specified log group. Check out AWS documentation for cloudwatch metric filter [here](https://docs.aws.amazon.com/cli/latest/reference/logs/put-metric-filter.html).

| Field                        | Description                                                                                                           | DataType | Required | Default |
| ---------------------------- | --------------------------------------------------------------------------------------------------------------------- | -------- | -------- | ------- |
| `name`                       | The name of the volume                                                                                                | `string` | Yes      |         |
| `type`                       | The type of volume. Valid values: `docker`, `host`, `efs`, `fsx`                                                      | `string` | No       | `host`  |
| `sourcePath`                 | Specify the path on the host container instance that’s presented to the container                                     | `string` | No       |         |
| `efsFilesystemId`            | Required if type is `efs`. Accepts either the Amazon EFS file system ID (`fs-*`) or the name of a buildit `efs-filesystem` resource, which is resolved via the file system's creation token | `string` | No       |         |
| `efsAccessPointId`           | If type is `efs`, specify the Amazon EFS access point to use. Accepts either the access point ID (`fsap-*`) or the `name` of an access point defined under a buildit `efs-filesystem` resource. Only declare this when `efsRootDir` is equals to `/` | `string` | No       |         |
| `efsIAMAuthorizationEnabled` | A name for the metric filter                                                                                          | `bool`   | No       | false   |
| `efsRootDir`                 | Default root path for an `efs` type volume                                                                            | `string` | No       |         |

Example:

```yaml
resources:
  taskdef:
    td-example:
      ...
      volumes:
        - name: peach-conf
          type: host
        - name: git-ssh-files
          type: host
        - name: datadog-conf
          type: host
        - name: cloudfront-conf
          type: host

```

EFS volumes may reference the file system and access point either by their physical IDs or by
the names of buildit `efs-filesystem` resources (and the nested access point `name`), so the
task definition does not need to hard-code IDs:

```yaml
resources:
  taskdef:
    td-example:
      ...
      volumes:
        # by physical IDs
        - name: reviewbot-v2-efs
          type: efs
          efsFilesystemId: fs-0763b5806d2a9e5ab
          efsAccessPointId: fsap-02e6b009fcdea54d0
          efsRootDir: /
        # by buildit efs-filesystem resource name and access point name
        - name: reviewbot-v2-efs
          type: efs
          efsFilesystemId: reviewbot-v2-efs
          efsAccessPointId: reviewbot-v2-accesspt
          efsRootDir: /

```

## ContainerPortMapping

Represents the port mappings for the container

| Field           | Description                                                                                              | DataType | Required | Default |
| --------------- | -------------------------------------------------------------------------------------------------------- | -------- | -------- | ------- |
| `containerPort` | The port number on the container that’s bound to the user-specified or automatically assigned `hostPort` | `int32`  | Yes      |         |
| `hostPort`      | The port number on the container instance to reserve for your container                                  | `int32`  | Yes      |         |
| `protocol`      | The protocol used for the port mapping. Valid values: `tcp` and `udp`                                    | `string` | Yes      |         |

Example:

```yaml
resources:
  taskdef:
    td-example:
      ...
      containers:
        ...
        - name: service
          ...
          portMappings:
            - hostPort: 3000
              containerPort: 3000
              protocol: tcp
            - hostPort: 9229
              containerPort: 9229
              protocol: tcp
            ...

```

## ContainerHealthcheck

Represents the container health check command and associated configuration parameters for the container.

| Field         | Description                                                                                                                                                                                                            | DataType   | Required | Default |
| ------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------- | -------- | ------- |
| `command`     | List of commands that the container runs to determine if it is healthy. The string array must start with CMD to run the command arguments directly, or CMD-SHELL to run the command with the container’s default shell | `[]string` | Yes      |         |
| `interval`    | The time period in seconds between each health check execution                                                                                                                                                         | `int32`    | Yes      |         |
| `timeout`     | The time period in seconds to wait for a health check to succeed before it is considered a failure                                                                                                                     | `int32`    | Yes      |         |
| `startPeriod` | The optional grace period to provide containers time to bootstrap before failed health checks count towards the maximum number of retries                                                                              | `int32`    | Yes      |         |
| `retries`     | The number of times to retry a failed health check before the container is considered unhealthy                                                                                                                        | `int32`    | Yes      |         |

Example:

```yaml
resources:
  taskdef:
    td-example:
      ...
      containers:
        ...
        - name: service
          ...
          healthcheck:
            command:
              - "CMD-SHELL"
              - "curl localhost:3000/healthcheck"
            interval: 10
            timeout: 5
            startPeriod: 20
            retries: 10
        ...
```

## ContainerDependsOn

Represents the dependencies defined for container startup and shutdown

| Field       | Description                                                                                        | DataType | Required | Default |
| ----------- | -------------------------------------------------------------------------------------------------- | -------- | -------- | ------- |
| `condition` | The dependency condition of the container. Valid values: `START`, `COMPLETE`, `SUCCESS`, `HEALTHY` | `string` | Yes      |         |
| `container` | The name of a container.                                                                           | `string` | Yes      |         |

Example:

```yaml
resources:
  taskdef:
    td-example:
      ...
      containers:
        ...
        - name: service
          ...
          dependsOn:
            - container: config
              condition: success
        ...
```

## ContainerMountPoints

Represents the mount points for data volumes in your container

| Field           | Description                                                                                                                                       | DataType | Required | Default |
| --------------- | ------------------------------------------------------------------------------------------------------------------------------------------------- | -------- | -------- | ------- |
| `readOnly`      | If this value is `true` , the container has read-only access to the volume. If this value is `false` , then the container can write to the volume | `bool`   | Yes      |         |
| `containerPath` | The path on the container to mount the host volume at                                                                                             | `string` | Yes      |         |
| `sourceVolume`  | The name of the volume to mount. Must be a volume `name` referenced in the name parameter of task definition `volumes`                            | `string` | Yes      |         |

Example:

```yaml
resources:
  taskdef:
    td-example:
      ...
      containers:
        ...
        - name: service
          ...
          mountPoints:
            - sourceVolume: peach-conf
              containerPath: /app/config
            - sourceVolume: git-ssh-files
              containerPath: /app/.ssh
            - sourceVolume: cloudfront-conf
              containerPath: /etc/aws
              dependencyOrder:
            ...
```

## ContainerLogging

Represents the log configuration specification for the container

| Field           | Description                                                                                                                                                                       | DataType            | Required | Default |
| --------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------- | -------- | ------- |
| `logDriver`     | The log driver to use for the container.                                                                                                                                          | `string`            | Yes      |         |
| `options`       | The configuration options to send to the log driver. See [LogConfiguration](https://docs.aws.amazon.com/AmazonECS/latest/APIReference/API_LogConfiguration.html) for more details | `map[string]string` | Yes      |         |
| `secretOptions` | The secrets to pass to the log configuration                                                                                                                                      | `map[string]string` | Yes      |         |

Example:

```yaml
resources:
  taskdef:
    td-example:
      ...
      containers:
        ...
        - name: service
          ...
          logConfiguration:
            logDriver: awsfirelens
            options:
                Name: datadog
                Host: http-intake.logs.datadoghq.com
                ...
        ...
```

## LinuxParameters

Represents Linux-specific modifications that are applied to the default Docker container configuration

| Field                | Description                                                                                                                                                                   | DataType             | Required | Default |
| -------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------- | -------- | ------- |
| `initProcessEnabled` | Enables an init process inside the container that forwards signals and reaps processes. Maps to the --init option to docker run                                               | `bool`               | No       | false   |
| `capabilities`       | The Linux capabilities for the container that have been added to the default configuration provided by Docker. See [KernelCapabilities](#kernelcapabilities) for more details | `KernelCapabilities` | No       |         |

Example:

```yaml
resources:
  taskdef:
    td-example:
      ...
      containers:
        ...
        - name: service
          ...
          linuxParameters:
              initProcessEnabled: true
              capabilities:
                add:
                - SYS_ADMIN
                - SYS_RESOURCE
                drop:
                - SYS_PTRACE
        ...
```

## KernelCapabilities

Represents Linux kernel capabilities

| Field  | Description                                                                                                       | DataType   | Required | Default |
| ------ | ----------------------------------------------------------------------------------------------------------------- | ---------- | -------- | ------- |
| `add`  | The Linux capabilities for the container that have been added to the default configuration provided by Docker     | `[]string` | No       |         |
| `drop` | The Linux capabilities for the container that have been removed from the default configuration provided by Docker | `[]string` | No       |         |

Example:

```yaml
resources:
  taskdef:
    td-example:
      ...
      containers:
        ...
        - name: service
          ...
          linuxParameters:
              capabilities:
                add:
                - SYS_ADMIN
                - SYS_RESOURCE
                drop:
                - SYS_PTRACE
        ...
```

## ContainerUlimit

Represents a ulimit to set in the container. If a ulimit value is specified in a task definition, it overrides the default values set by Docker

| Field       | Description                                                                                                                          | DataType | Required | Default |
| ----------- | ------------------------------------------------------------------------------------------------------------------------------------ | -------- | -------- | ------- |
| `name`      | The type of the ulimit                                                                                                               | `string` | Yes      |         |
| `softLimit` | The soft limit for the ulimit type. The value can be specified in bytes, seconds, or as a count, depending on the type of the ulimit | `int32`  | Yes      |         |
| `hardLimit` | The hard limit for the ulimit type. The value can be specified in bytes, seconds, or as a count, depending on the type of the ulimit | `int32`  | Yes      |         |

Example:

```yaml
resources:
  taskdef:
    td-example:
      ...
      containers:
        ...
        - name: service
          ...
          ulimits:
            - name: nofile
              softLimit: 8192
              hardLimit: 8192
        ...
```

## FirelensConfiguration

Represents the FireLens configuration for the container. This is used to specify and configure a log router for container logs

| Field     | Description                                                   | DataType            | Required | Default     |
| --------- | ------------------------------------------------------------- | ------------------- | -------- | ----------- |
| `type`    | The log router to use. Valid values: `fluentd` or `fluentbit` | `string`            | No       | `fluentbit` |
| `options` | The options to use when configuring the log router            | `map[string]string` | No       |             |

Example:

```yaml
resources:
  taskdef:
    td-example:
      ...
      containers:
        ...
        - name: service
          ...
          firelensConfiguration:
            type: fluentbit
            options:
              enable-ecs-log-metadata: true
              config-file-type: file
              config-file-value: /fluent-bit/configs/example-extra.conf
        ...
```

## RuntimePlatform

Represents the runtime platform configuration for the task

| Field                   | Description                                                                                                                                                                     | DataType | Required | Default |
| ----------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------- | -------- | ------- |
| `cpuArchitecture`       | The CPU architecture for the task. Valid values: `X86_64` or `ARM64`                                                                                                            | `string` | Yes      |         |
| `operatingSystemFamily` | The operating system family for the task. Valid values: `LINUX`, `WINDOWS_SERVER_2019_FULL`, `WINDOWS_SERVER_2019_CORE`, `WINDOWS_SERVER_2022_FULL`, `WINDOWS_SERVER_2022_CORE` | `string` | Yes      |         |

> **Note:** Both `cpuArchitecture` and `operatingSystemFamily` are required when `runtimePlatform` is specified.
> Example:

```yaml
resources:
  taskdef:
    td-example:
      ...
      runtimePlatform:
        cpuArchitecture: ARM64
        operatingSystemFamily: LINUX
      ...
```
