

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