# IAM Role `iam-role` 

Create, update or destroy an IAM role. 

> You can qualify the resource name with a provider prefix or supply the provider name in the standard `provider` field of the resource to indicate the provider to use. If neither is supplied, the `main` is used as the default provider name.

Check out AWS documentation for iam role [here](https://awscli.amazonaws.com/v2/documentation/api/latest/reference/iam/create-role.html). 


| Field | Description | DataType | Required | Default |
|--|--|--|--|--|
|Name (Resource Name)|The resource name is used as the name of the iam role |`string`|Yes||
|`description`|A description of the role|`string`|No||
|`maxSessionDuration`|The maximum session duration (in seconds) that you want to set for the specified role|`int32`|No|`3600`|
|`path`|The path to the role. See [IAM Identifiers](https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_identifiers.html)|`string`|No||
|`trustPolicy`|The JSON trust policy document that grants an entity permission to assume the role. See [IAMPolicyDocument](https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_policies_elements.html) for more details|`IAMPolicyDocument`|No||
|`permissions`|A list of IAM policies attached to the role. See [IAMPolicy](./iam_policy.md) for policy creation|`string`|No||
|`tags`|A key value map of resource tags to be applied to this resource. The `GlobalTags` are always applied, any matching keys are overriden from `tags`|`map[string]string`|No|`{}`|
|`dependsOn`|The `buildit` resources that this resource depends on in the context of the current execution. All resources that are listed in this section will be built before this; while destoryed after this|`[]string`|No|`[]`|

Example:

```yaml 

resources:
  iam-policy:
    buildit/example-core-api-data-refresh-policy:
      description: IAM Policy for example-core-api-data-refresh service IAM Role
      policy: 
      ...

  iam-role:
    example-core-api-data-refresh-role:
      description: taskDef IAM role for example-core-api-data-refresh
      maxSessionDuration: 43200
      path: /buildit/
      trustPolicy:
                {
                  "Version": "2012-10-17",
                  "Statement":
                    [
                      {
                        "Effect": "Allow",
                        "Principal": { "Service": "ecs-tasks.amazonaws.com" },
                        "Action": "sts:AssumeRole",
                      },
                    ],
                }
      permissions:
        - buildit/example-core-api-data-refresh-policy
        - AmazonECSTaskExecutionRolePolicy
      dependsOn:
        - buildit/example-core-api-data-refresh-policy
```
