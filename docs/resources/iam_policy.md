# IAM Policy `iam-policy` 

Create, update or destroy an IAM policy. 

> You can qualify the resource name with a provider prefix or supply the provider name in the standard `provider` field of the resource to indicate the provider to use. If neither is supplied, the `main` is used as the default provider name.

Check out AWS documentation for iam policy [here](https://awscli.amazonaws.com/v2/documentation/api/latest/reference/iam/create-policy.html). 


| Field | Description | DataType | Required | Default |
|--|--|--|--|--|
|Name (Resource Name)|The resource name is used as the name of the iam policy. Consists of the path and policy name. Ex: `buildit/resource-policy`|`string`|Yes||
|`description`|A description of the policy. Description is immutable. After a value is assigned, it cannot be changed|`string`|No||
|`policy`|The JSON policy document that you want to use as the content for the new policy. See [IAMPolicyDocument](https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_policies_elements.html) for more details |`IAMPolicyDocument`|No||
|`tags`|A key value map of resource tags to be applied to this resource. The `GlobalTags` are always applied, any matching keys are overriden from `tags`|`map[string]string`|No|`{}`|
|`dependsOn`|The `buildit` resources that this resource depends on in the context of the current execution. All resources that are listed in this section will be built before this; while destoryed after this|`[]string`|No|`[]`|

Example:

```yaml 

resources:
  iam-policy:
    buildit/example-core-api-data-refresh-policy:
      description: IAM Policy for example-core-api-data-refresh service IAM Role
      policy:
        {
          "Version": "2012-10-17",
          "Statement":
            [
              {
                "Effect": "Allow",
                "Action": "secretsmanager:GetSecretValue",
                "Resource":
                  [
                    "arn:aws:secretsmanager:us-east-1:123456789012:secret:/secrets/example/*",
                  ],
              },
              {
                "Effect": "Allow",
                "Action": [
                  "ssmmessages:CreateControlChannel",
                ],
                "Resource": "*"
              },
              {
                "Effect": "Allow",
                "Action": [
                  "logs:DescribeLogGroups"
                ],
                "Resource": "*"
              }
            ]
      }
```

## Policy Document Comparison

When deciding whether the policy needs an update, `buildit` compares the defined policy document to the one in AWS semantically, not textually. The following representation-only differences are ignored and will not trigger a policy update:

- A bare string and the equivalent single-element array for union-typed fields (`Action`, `NotAction`, `Resource`, `NotResource`, `Principal.*`, `NotPrincipal.*` and condition values). Ex: `"Resource": "*"` equals `"Resource": ["*"]`.
- The ordering of values within those lists. Ex: reordering `Resource` ARNs or `Action` entries.
- The ordering of statements, with or without `Sid`s. IAM evaluates all statements together and an explicit deny always overrides an allow, so statement order never changes the meaning of a policy.
- A single statement object vs an array containing one statement.
- The casing of principal type keys (`AWS`, `Service`, `Federated`, `CanonicalUser`). AWS canonicalizes these when storing a policy, so `service:` in config equals the stored `Service`. This also applies to the `lambda-function` resource policy `principal` field.

Any semantic difference (different ARNs, actions, effects, `Sid`s, principals, conditions, or missing/extra statements) is still detected and applied as an update.
