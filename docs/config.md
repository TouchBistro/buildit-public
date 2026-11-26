

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
The root `buildit` package exposes the same pipeline the CLI runs: fill a `buildit.ResourceSet`
with the `AddX` / `AddXWithProvider` methods (one pair per resource type, named after the
`resources:` sections), and hand it to `buildit.NewConfig` with the tags.

### Providers

Providers are [awesome](https://github.com/TouchBistro/awesome) `providers.CredsProvider`
values, built with that package's own constructors, and awesome's process-wide registry is the
only place **buildit** looks for them. Building a provider with `providers.New*CredsProvider`
registers it under its name; do that before calling `NewConfig`, which rejects a resource naming
a provider that is not registered (`unknown provider name`) before any AWS call is made on its
behalf. The two reserved names work as follows:

- `default` is the credentials the program runs with. A registered `default` is used as is and
  never replaced by **buildit**. If none is registered, `NewConfig` builds one the way the CLI
  does without flags: the AWS default credentials chain, `us-east-1`, validated with STS. Awesome's
  `init/cmd` package registers `default` from the process's flags and environment when
  blank-imported, so a program that already uses it needs nothing more.
- `main` provisions the resources that name no provider and resolves the references they make.
  **buildit** never writes it: at the time a client is built, `main` is the registered `main` if
  there is one (declared in a config file, or registered by the program), else `default`.

Because the registry is shared by the whole process, configs that need different `default`s must
not be built or applied at the same time: re-registering `default` moves every config's unprefixed
resources to it.

```go
// `default`: the program's own credentials, registered explicitly to pick the region
// and validate up front. Omit this and NewConfig builds one from the default chain.
base, err := providers.NewDefaultCredsProvider(ctx, providers.DefaultCredsProviderName,
    providers.WithRegion("us-east-1"), providers.ValidateProvider())
other, err := providers.NewAssumeRoleCredsProvider(ctx, "example-other",
    providers.WithBaseCredsProvider(base),
    providers.WithAccountId("123456789012"),
    providers.WithRoleName("example-role"))

var rs buildit.ResourceSet
logs := resource.CWLogGroup{Retention: aws.Int32(30)}
err = rs.AddCWLogGroup("example-logs", logs)                    // provisioned by `main`, here `default`
err = rs.AddCWLogGroupWithProvider(other, "example-logs", logs) // provisioned by `example-other`
err = rs.AddCWLogGroup("example-other::example-audit-logs", logs) // the same, by name

cfg, err := buildit.NewConfig(ctx, rs, buildit.WithGlobalTag("team", "example-team"))
if err != nil {
    return err // every validation, graph and provider problem is reported here
}
graph := cfg.Graph() // the same wired graph `buildit plan` would build from the YAML
err = graph.Apply(buildit.ContextWithRetryPolicy(ctx, 3, 20))
```

`NewConfig` behaves exactly as parsing files does: it resolves each resource's provider and
name, applies global and `buildit:` tags, normalizes, validates, and builds the dependency edges.
`AddXWithProvider` is a convenience for the `provider::name` key form: it takes the provider the
registry holds under that name, and fails if an older provider object has since been replaced
there by a newer build of the same name.

When resources fail validation, the error is a `buildit.ValidationErrors`: one entry per failure,
so a program can name every resource that failed without reading a log.

```go
var failed buildit.ValidationErrors
if errors.As(err, &failed) {
    for _, e := range failed {
        var v *resource.ValidationError
        if errors.As(e, &v) {
            fmt.Println(v.ResourceIdentifier, v.Messages)
        }
    }
}
```

A dependency cycle between resources is also an error from `NewConfig`, and from every CLI
command when the config is loaded: `dependency cycle between resources: main::a -> main::b -> main::a`.

Config files can be merged into the same `Config`, rendered with variables and templates the way
the CLI flags of the same names do. A provider declared in a config file's `providers:` section
replaces a registered one of the same name, with a warning.

```go
cfg, err := buildit.NewConfig(ctx, rs,
    buildit.WithConfigPath(".buildit/"),         // --path
    buildit.WithTemplates("templates/"),         // --templates
    buildit.WithVariablesFrom("vars/prod.vars"), // --variables-from
    buildit.WithVariable("retention", "30"),     // --variables
)
```

The `audit` global tag is derived from the config files' contents. A `Config` built only in Go
has nothing to hash, so pass `buildit.WithoutAuditTags()` or set `audit` yourself with
`WithGlobalTag`.
