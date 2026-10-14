

The **buildit** decleration, or config is a domain-specific language written in YAML. It has 3 main sections to define the following:

- AWS credential sources, or providers
- Global Resource Tags
- Resource Definitions

Below is a sample structure of a **buildit** config yaml document:

```yaml 

--- 

providers:
  main: 
    type: role
    accountID: 123456789012
    roleName: buildit-role
  :

globalTags:
  iac:created-with: buildit
  repo: buildit
 
resource:
  :
  :

```

## AWS Credential Sources

AWS credential sources (`providers` section) refers to how **buildit** must look for AWS credentials for building and searching for AWS resources. For more details on how to configure **buildit** providers, see [Providers](./providers.md)

## Global Resource Tags

Global Resource Tags (`globalTags` section ) define a simple key-value map for AWS resource tags to apply to all resources defined in this decleration. Each individual resource can later override or add more `tags` as part of each resource definition.

### Reserved tag keys

The `buildit:` tag prefix is reserved. **buildit** writes its own tags into that namespace, so a
config may not set a key beginning with `buildit:` — neither in `globalTags` nor in an individual
resource's `tags`. Doing so fails config load, before anything is sent to AWS:

```
globalTags in ./infra/base.yml: tag key "buildit:owner" uses the reserved "buildit:" prefix
s3-bucket: my-bucket: tag key "buildit:resource-id" uses the reserved "buildit:" prefix
```

The keys **buildit** currently writes:

| Tag | Value | Applied to |
| :-- | :-- | :-- |
| `buildit:resource-id` | The resource's name as declared in the config, without any provider prefix. Characters AWS does not permit in a tag value are replaced with `_`, so a wildcard certificate `*.example.com` is tagged `_.example.com`. | Every resource **buildit** manages that AWS supports tagging. Nested items — load balancer listeners and rules, EFS access points, EventBridge targets — are not tagged, and do not inherit their parent's value. |

Renaming a resource in the config changes its `buildit:resource-id`, since the value is derived from
the name.

The `audit` tag is also written by **buildit**, but predates this namespace and sits outside it: it
holds a checksum of the config files used for a run, is consumed by `buildit audit`, can be
suppressed with `--no-audit`, and — unlike the reserved keys above — can still be set explicitly in
`globalTags` to override the generated value.

## Resource Definitions

The resource definitions (`resources` section) declare the AWS resources to be provisioned or updated. Each resource is defined in it's own sub-section under `resources`. The example below shows a simple EC2 Security Group definition

For more details on supported resource types & their configuration options see [Supported AWS Resource Types](./resources/resources.md)

Resource names must be unique within a provider **across all resource types**. buildit identifies a resource by `provider::name` only, so an `s3-bucket` and an `sqs-queue` both named `example-thing` under the same provider are the same key. Declaring such a pair (in one file or across several `--path` files) fails validation before anything is planned:

```
resource key main::example-thing is declared twice: s3-bucket "example-thing" and sqs-queue "example-thing" share it; resource names must be unique within a provider across all resource types
```

```yaml 
---

resources:
  security-group:
    my-securitygroup:
      description: a simple security group for my application
      vpcName: demo-vpc
      outboundRules:
        cidrBlocks: 
        - value: 0.0.0.0/0
          description: allow outbound to all
      inboundRules:
      - portRange: 80
        ipProtocol: tcp
        cidrBlocks:
        - value: 0.0.0.0/0
          description: allow http:80 from the world
```
## Building a config from Go

A program that embeds **buildit** as a library does not have to write a `buildit.yml` to disk.
`config.ResourceSet` is the Go form of the `resources:` section, with one map per resource type
keyed the same way as the YAML (`name` or `provider::name`), and `config.NewInternalConfig` takes
it together with the `providers:` and `globalTags:` values:

```go
rs := config.ResourceSet{
    CWLogGroup: map[string]resource.CWLogGroup{
        "example-logs": {Retention: aws.Int32(30)},
    },
}
providers := map[string]*client.AwsProvider{
    "example": {ProviderType: client.ProviderTypeRole, AccountId: "123456789012", RoleName: "example-role"},
}

ic := config.NewInternalConfig(rs, providers, map[string]string{"team": "example-team"})
if err := ic.Generate(ctx, config.RootOptions{NoAuditTags: true}); err != nil {
    return err
}
graph := ic.Graph() // the same wired graph `buildit plan` would build from the YAML
```

`Generate` behaves exactly as it does for parsed files: it initializes the providers, resolves each
resource's provider and name, applies global and `buildit:` tags, normalizes, validates, and builds
the dependency edges. There is no file content to checksum, so the `audit` tag is only meaningful if
you set it in `globalTags` yourself; otherwise pass `NoAuditTags`.
