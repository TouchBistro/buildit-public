# Bedrock Application Inference Profile `bedrock-application-inference-profile`

This resource creates an Amazon Bedrock **application inference profile** that copies from a foundation model or a system-defined cross-region inference profile. Application profiles are used to track metrics and costs for invocations, and can carry tags for cost allocation and IAM-based access scoping.

> You can qualify the resource name with a provider prefix or supply the provider name in the standard `provider` field of the resource to indicate the provider to use. If neither is supplied, `main` is used as the default.

Check out AWS documentation for `CreateInferenceProfile` [here](https://docs.aws.amazon.com/bedrock/latest/APIReference/API_CreateInferenceProfile.html).

## Immutability

After creation, only **tags** can be updated. Changes to `description` or `modelSource` surface as diff messages but cannot be applied — destroy and recreate the profile to change them.

## Drift detection caveats

AWS does not return the original `modelSource` (the `CopyFrom` value) on read — only the resolved `Models[].ModelArn` list. This affects what `plan` can detect:

- **Foundation-model source** (`modelSource: arn:…:foundation-model/…`): drift is detected — `plan` flags a change when the existing profile's `Models[0].ModelArn` differs from the configured `modelSource`.
- **System-defined inference-profile source** (`modelSource: arn:…:inference-profile/…`, e.g. cross-region profiles): drift is **not** detected. AWS expands the system profile into its underlying foundation models, so `Models[]` never equals the `inference-profile/` ARN you configured. Swapping `modelSource` from one system-defined inference profile to another produces no diff — destroy and recreate the resource to switch.

Tag and name changes are always detected.

## Fields

| Field | Description | DataType | Required | Default |
|--|--|--|--|--|
| Name (Resource Name) | The resource name is used as the inference profile name | `string` | Yes | |
| `description` | Free-form description for the profile. Immutable after creation | `string` | No | `""` |
| `modelSource` | ARN of the foundation model **or** system-defined inference profile to copy from. Must be a valid Bedrock ARN (`arn:aws*:bedrock:...`, including `aws`, `aws-us-gov`, and `aws-cn` partitions) pointing at a `foundation-model/` or `inference-profile/` resource. Immutable after creation. | `string` | Yes | |
| `tags` | Tags to attach to the profile. Merged with `globalTags`. Mutable. | `map[string]string` | No | `{}` |
| `dependsOn` | The `buildit` resources that this resource depends on. Listed deps are built before this and destroyed after. | `[]string` | No | `[]` |

## Example

```yaml
resources:
  bedrock-application-inference-profile:
    example-kitchen-sink-claude-haiku:
      description: "Claude Haiku profile for kitchen-sink service"
      modelSource: arn:aws:bedrock:us-east-1::foundation-model/anthropic.claude-3-haiku-20240307-v1:0
      tags:
        team: platform
        service: kitchen-sink
```

Cross-region system-defined profile source:

```yaml
resources:
  bedrock-application-inference-profile:
    claude-sonnet-cross-region:
      modelSource: arn:aws:bedrock:us-east-1:123456789012:inference-profile/us.anthropic.claude-3-5-sonnet-20240620-v1:0
      tags:
        team: ml-platform
```
