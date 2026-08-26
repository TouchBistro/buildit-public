# ECS Service `ecs-service`

Runs and maintains your desired number of tasks from a specified task definition. Check out AWS documentation for ECS Service [here](https://awscli.amazonaws.com/v2/documentation/api/latest/reference/ecs/create-service.html).

> You can qualify the resource name with a provider prefix or supply the provider name in the standard `provider` field of the resource to indicate the provider to use. If neither is supplied, the `main` is used as the default provider name.

| Field                      | Description                                                                                                                                                                                        | DataType                        | Required | Default                                 |
| -------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------- | -------- | --------------------------------------- |
| `launchType`               | The infrastructure that you run your service on. Valid values: `CAP`, `EC2`, `FARGATE`                                                                                                             | `string`                        | No       | `CAP`                                   |
| `capacityProviders`        | Only when `launchType` is `CAP` and `serviceType` is `REPLICA`. The capacity provider strategy to use for the service. See [ECSCapacityProviderStrategy](#ecscapacityproviderstrategy)             | `[]ECSCapacityProviderStrategy` | No       |                                         |
| `taskDefName`              | Specify the task definition for the tasks in the service to use                                                                                                                                    | `string`                        | Yes      |                                         |
| `clusterName`              | The name of the cluster that you run your service on                                                                                                                                               | `string`                        | Yes      |                                         |
| `serviceType`              | The scheduling strategy to use for the service. Valid values: `REPLICA`, `DAEMON`                                                                                                                  | `string`                        | No       | `REPLICA`                               |
| `desiredCount`             | Only when `serviceType` is `REPLICA`. The number of instantiations of the specified task definition to place and keep running in your service                                                      | `int32`                         | No       | `0`                                     |
| `minHealthyPercent`        | Represents a lower limit on the number of your service’s tasks that must remain in the `RUNNING` state during a deployment, as a percentage of the `desiredCount`                                  | `int32`                         | No       | `100` for `REPLICA`, `0` for `DAEMON`   |
| `maxPercent`               | Represents an upper limit on the number of your service’s tasks that are allowed in the `RUNNING` or `PENDING` state during a deployment, as a percentage of the desiredCount                      | `int32`                         | No       | `200` for `REPLICA`, `100` for `DAEMON` |
| `deploymentCircuitBreaker` | Defines circuit breaker behavior for the service deployment. See [ECSDeploymentCircuitBreaker](#ecsdeploymentcircuitbreaker)                                                                       | `ECSDeploymentCircuitBreaker`   | No       |                                         |
| `assignPublicIp`           | Determine if the task’s elastic network interface receives a public IP address. Valid values: `ENABLED`, `DISABLED`                                                                                | `string`                        | No       | `DISABLED`                              |
| `subnetNames`              | Required for `awsvpc` tasks networking. The IDs of the subnets associated with the task or service.                                                                                                | `[]string`                      | No       |                                         |
| `securityGroupNames`       | Required for `awsvpc` tasks networking. The IDs of the security groups associated with the task or service                                                                                         | `[]string`                      | No       |                                         |
| `loadBalancing`            | A load balancer object representing the load balancers to use with your service. See [ECSServiceLoadBalancing](#ecsserviceloadbalancing)                                                           | `ECSServiceLoadBalancing`       | No       |                                         |
| `serviceDiscovery`         | Enabled CloudMap/Route53 based service discovery for the ECS Service. See [ECSServiceDiscovery](#ecsservicediscovery)                                                                              | `ECSServiceDiscovery`           | No       |                                         |
| `autoScaling`              | Autoscaling configurations for the service. See [ApplicationAutoScaling](./application_autoscaling.md)                                                                                             | `ApplicationAutoScaling`        | No       |                                         |
| `forceNewDeployment`       | If set, will always force an update                                                                                                                                                                | `bool`                          | No       |                                         |
| `checkDeployment`          | Configurate checks for `buildit` deployment. See [ECSServiceCheckDeployment](#ecsservicecheckdeployment)                                                                                           | `ECSServiceCheckDeployment`     | No       |                                         |
| `tags`                     | A key value map of resource tags to be applied to this resource. The `GlobalTags` are always applied, any matching keys are overriden from `tags`                                                  | `map[string]string`             | No       | `{}`                                    |
| `dependsOn`                | The `buildit` resources that this resource depends on in the context of the current execution. All resources that are listed in this section will be built before this; while destoryed after this | `[]string`                      | No       | `[]`                                    |

Example:

```yaml
resources:
  ecs-service:
    example-core-api-svc:
      tags:
        Name: example-core-api-svc
      launchType: CAP
      taskDefName: example-core-api-td
      clusterName: example-cluster
      serviceType: REPLICA
      checkDeployment:
        enabled: true
        timeoutSeconds: 1200
        failedTasksThreshold: 200
      deploymentCircuitBreaker:
        enabled: true
        rollback: true
      desiredCount: 12
      minHealthyPercent: 100
      maxPercent: 200
      subnetNames:
        - example-private-0-subnet
        - example-private-1-subnet
        - example-private-2-subnet
      securityGroupNames:
        - example-core-api-svc-sg
        - example-sg
      autoScaling:
        minCapacity: 12
        maxCapacity: 40
        policies:
          - policyName: example-core-api-autoscaling-policy
            policyType: target-tracking
            coolDown: 300
            disableScaleIn: false
            targetMetricName: cpu
            targetMetricValue: 65
      serviceDiscovery:
        namespace: tb.example
        name: example-core-api
      loadBalancing:
        healthcheckGracePeriod: 5
        targetGroups:
          - containerName: waf
            containerPort: 3002
            targetGroupName: example-core-api-waf-tg
      forceNewDeployment: true
```

---

## ECSCapacityProviderStrategy

| Field    | Description                                                                                                                               | DataType | Required | Default |
| -------- | ----------------------------------------------------------------------------------------------------------------------------------------- | -------- | -------- | ------- |
| `base`   | The base value designates how many tasks, at a minimum, to run on the specified capacity provider                                         | `int32`  | Yes      |         |
| `weight` | The weight value designates the relative percentage of the total number of tasks launched that should use the specified capacity provider | `int32`  | Yes      |         |
| `name`   | The short name of the capacity provider                                                                                                   | `string` | Yes      |         |

---

## ECSDeploymentCircuitBreaker

If you use the deployment circuit breaker, a service deployment will transition to a failed state and stop launching new tasks. If you use the rollback option, the service is rolled back to the last successful deployment
| Field | Description | DataType | Required | Default |
|-------|-------------|----------|----------|---------|
|`enabled`|Determines whether to use the deployment circuit breaker logic for the service|`bool`|No||
|`rollback`|Determines whether to configure Amazon ECS to roll back the service if a service deployment fails|`bool`|No||

---

## ECSServiceLoadBalancing

| Field                    | Description                                                                                                              | DataType                            | Required | Default |
| ------------------------ | ------------------------------------------------------------------------------------------------------------------------ | ----------------------------------- | -------- | ------- |
| `healthcheckGracePeriod` | Time in seconds when no LB healtcheck is performed                                                                       | `int32`                             | No       |         |
| `targetGroups`           | List of target groups to assign this service to. See [ECSServiceTargetGroupAssignment](#ecsservicetargetgroupassignment) | `[]ECSServiceTargetGroupAssignment` | No       |         |

---

## ECSServiceDiscovery

| Field       | Description                                  | DataType | Required | Default |
| ----------- | -------------------------------------------- | -------- | -------- | ------- |
| `name`      | The service discovery name. Must be unique   | `string` | Yes      |         |
| `namespace` | The namespace to be created for this service | `string` | Yes      |         |

---

## ECSServiceCheckDeployment

| Field                  | Description                                                        | DataType | Required | Default |
| ---------------------- | ------------------------------------------------------------------ | -------- | -------- | ------- |
| `enabled`              | Determine if service check if enabled                              | `bool`   | No       |         |
| `timeoutSeconds`       | The time in seconds before the deployment time out                 | `int`    | No       | `600`   |
| `failedTasksThreshold` | The number of tasks that fails for the deployment to reach failure | `int`    | No       | `3`     |

---

## ECSServiceTargetGroupAssignment

| Field             | Description                                                                                        | DataType | Required | Default   |
| ----------------- | -------------------------------------------------------------------------------------------------- | -------- | -------- | --------- |
| `containerName`   | The name of the container (as it appears in a task definition) to associate with the load balancer | `string` | No       | `service` |
| `containerPort`   | The port on the container to associate with the load balancer                                      | `int32`  | No       | `8080`    |
| `targetGroupName` | The name of the target group associated with the service                                           | `string` | No       |           |

---
