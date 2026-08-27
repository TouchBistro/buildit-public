# Resource Implementation Guide

Rules for modifying resources in `resource/`.

## Core Resource Interface

All resources must implement the base resource interface:

```go
Key() Key                            // Unique identifier within buildit context
Identifier() string                  // Human-readable identifier
Apply(ctx context.Context) error     // Create/update resource
Destroy(ctx context.Context) error   // Delete resource
```

### Extended Interface: `ComparableResource`

Resources that need diff detection implement `ComparableResource`:

```go
Compare(ctx context.Context) (ResourceDiff, error) // Check for diffs between desired and actual state
```

### `ResourceDiff` Interface

All diffs implement the `ResourceDiff` interface:

```go
Differences() []string              // Human-readable list of differences
AWSResource() any                   // The actual AWS resource used for comparison
```

## Implementation Patterns

### Resource Structure Pattern

```go
type ResourceName struct {
    BaseResource `yaml:",inline"`
    Field1       string            `yaml:"field1"`
    Field2       []string          `yaml:"field2"`
    GlobalTags   map[string]string `yaml:"-"`  // Don't serialize
    DependsOn    []Key             `yaml:"dependsOn"`
}

// Implement required interfaces
func (r ResourceName) Key() Key {
    return NewKey(r.Context.ProviderName, r.Identifier())
}

func (r ResourceName) Identifier() string {
    return r.Field1 // or some other unique identifier
}
```

**`BaseResource` must be embedded with `yaml:",inline"`.** yaml.v2 does not auto-inline embedded structs (unlike `encoding/json`), so without the tag the resource-level `provider` field is silently dropped and the resource falls back to the main provider. `TestResourceProviderFieldIsParsed` in `config/provider_field_test.go` enforces this for every resource type registered in `resourcesConfig`.

### Apply Method Pattern

1. Call `Compare(ctx)` to get diffs.
2. If no diffs, log "no updates required" and return.
3. If resource doesn't exist, call `apply()` helper.
4. If diffs exist for existing resource, call `applyDiffs()` helper.
5. Log success with colored output.

### Normalize Method Pattern

- Set default values for optional fields.
- Sanitize/format input values.
- Merge global tags with resource-specific tags.
- Resolve ARNs or identifiers from names.
- **Note**: Normalize is for data preparation; Validate is for business rules.

### Validate Method Pattern

- Return `ValidationError` with descriptive messages.
- Build up error message slice, return all errors at once.
- Check required fields and enum values.

### Diff Implementation Pattern

```go
func (r ResourceName) Compare(ctx context.Context) (ResourceDiff, error) {
    existing, err := r.fetchExisting(ctx)
    // ... handle errors ...
    if existing == nil {
        return &ResourceNameDiff{Messages: []string{"resource does not exist"}}, nil
    }
    // Compare fields and return nil if no differences
}
```

### Tag Comparison and Diff Pattern

Use the shared tag diff helpers in `resource/resource_tags.go` for all resource tag comparisons.

```go
if tagDiff := TagDiffForContext(ctx, awsTags, r.Tags); tagDiff.HasChanges() {
    diffs.tagsDiff = true
    diffs.tagDiff = tagDiff
    diffs.Messages = append(diffs.Messages, TagDiffSummary(awsTags, diffs.tagDiff)...)
}
```

Rules:

- Always use `TagDiffForContext(ctx, currentTags, desiredTags)` so ignored keys from `util.IGNORE_TAGS` are respected.
- Always append detailed messages with `TagDiffSummary(currentTags, tagDiff)...`, not a count-only summary.
- Do not use `util.StringMap(...).Equals(...)` or `Convert(...)` for tag diffs in `resource/` compare paths.
- In `applyDiffs()`, apply add/update calls using `tagDiff.Upserts()`.
- In `applyDiffs()`, apply deletes using `tagDiff.Deleted` or `tagDiff.DeletedKeys()` depending on SDK API shape.
- Never hand a parent's tag map straight to a nested child (a load balancer listener or rule, an EFS
  access point, an EventBridge target). Pass it through `InheritedTags` so the parent's
  `buildit:resource-id` does not leak onto resources that are not top-level buildit resources — a
  lookup by that id would otherwise resolve to the parent plus every child under it.
- The `buildit:` tag prefix is reserved for buildit's own keys. `ResourceTags.Merge` strips it from a
  resource's own tags, so nothing in `resource/` should be writing one; the values arrive through
  `GlobalTags`, which `config` populates.

## Pattern: Name-to-ARN Resolution

AWS SDK requires **ARNs**, but `buildit` YAML often uses **Names** or **IDs**. Use the `awsw` package standardized tiered lookup pattern.

### Standardized Lookup Methods

Location: `awsw/[service].go`
Signature: `func (s ServiceWrapper) [Resource]ArnForIdentifier(ctx context.Context, identifier string) (*string, error)`

### Common Resource Type Configuration to ARN

```
type MyResource struct {
    ...
    OtherResource    string  `yaml:"OtherResource"`
    OtherResourceArn *string `yaml:"-"`
    ...
}
```

**Requirements:**

1. Use `awsw.ParseIdentifier` for `<provider>::<resource>` support.
2. Tiered resolution: ARN -> ID -> Name Tag/Listing.
3. Logic must reside in `awsw`, never hardcoded in `resource/`.
4. The lookup registry must be updated with each new method.

### Lookup Registry

| Service              | Method                                                                           | Support Tiers                        |
| :------------------- | :------------------------------------------------------------------------------- | :----------------------------------- |
| **ACM**              | `CertificateArnForIdentifier` (`NewACM` regional; `NewACMGlobal` pinned us-east-1 for CloudFront viewer certs) | ARN, ID (UUID), Domain Name          |
| **Bedrock**          | `ApplicationInferenceProfileByName`                                              | Name                                 |
| **CloudFront**       | `VpcOriginByIdentifier`, `VpcOriginIdForIdentifier`, `FunctionArnForIdentifier` | ID, Name / ARN, Name                 |
| **CloudFront**       | `FindVpcOriginByName` — lifecycle lookup: returns `(nil, nil)` when absent       | Name                                 |
| **CloudFront**       | `FindFunctionByName` — lifecycle lookup: returns `(nil, nil)` when absent        | Name                                 |
| **CloudFront**       | `CachePolicyIdForIdentifier`, `OriginRequestPolicyIdForIdentifier`, `ResponseHeadersPolicyIdForIdentifier` | ID (UUID), Name                      |
| **CloudWatch**       | `AlarmArnForIdentifier`, `LogGroupArnForIdentifier`                              | ARN, Name                            |
| **DynamoDB**         | `TableArnForIdentifier`                                                          | ARN, Name                            |
| **EC2**              | `VpcArnForIdentifier`, `SubnetArnForIdentifier`, `SecurityGroupArnForIdentifier` | ARN, ID, Name Tag                    |
| **ECS**              | `ClusterArnForIdentifier`                                                        | ARN, Name                            |
| **IAM**              | `RoleArnForIdentifier`, `PolicyArnForIdentifier`                                 | ARN, Name                            |
| **Lambda**           | `FunctionArnForIdentifier`                                                       | ARN, Name                            |
| **MSK**              | `ClusterArnForIdentifier`                                                        | ARN, Name                            |
| **Route53**          | `HostedZoneArnForIdentifier`                                                     | ARN, ID, Domain Name                 |
| **S3 / SNS / SQS**   | `BucketArnForIdentifier`, `TopicArnForIdentifier`, `QueueArnForIdentifier`       | ARN, Name                            |
| **SecretsManager**   | `SecretArnForIdentifier`                                                         | ARN, Name/ID                         |
| **ServiceDiscovery** | `ServiceArnForIdentifier`                                                        | ARN, ID, Name                        |
| **SFN**              | `StateMachineArnForIdentifier`                                                   | ARN, Name                            |
| **EFS**              | `FileSystemArnForIdentifier`, `FileSystemIDForIdentifier`, `AccessPointArnForIdentifier`, `AccessPointIDForIdentifier`, `ResolveVolumeIDs` | ARN, FileSystemId/AccessPointId, Creation Token (resource name), Access Point Name/ClientToken |
| **ELBv2**            | `LoadBalancerArnForIdentifier`, `TargetGroupArnForIdentifier`                    | ARN, Name                            |
| **EventBridge**      | `RuleArnForIdentifier`, `BusArnForIdentifier`                                    | ARN, Name                            |
| **Firehose**         | `DeliveryStreamArnForIdentifier`                                                 | ARN, Name                            |
| **Glue**             | `CatalogArnForIdentifier`                                                        | ARN, Catalog ID, Name                |
| **WAFv2**            | `WebACLArnForIdentifier` (global/CLOUDFRONT scope, us-east-1)                    | ARN, Name                            |

## Pattern: EFS Volume References

Any resource whose config mounts EFS (a file system and optionally an access point) must resolve references through the shared `awsw` EFS resolvers — never reimplement the lookup:

- **ID consumers** (e.g. ECS task definitions): call `awsw.EFS.ResolveVolumeIDs(ctx, fsID, apID)`. It is nil-safe and idempotent — physical IDs (`fs-*` / `fsap-*`) pass through without AWS calls. See `TaskDef.resolveEFSVolumeIDs` in `taskdef.go`.
- **ARN consumers** (e.g. Lambda `fileSystem`): call `awsw.EFS.AccessPointArnForIdentifier`. See `Function.resolveFileSystemArn` in `lambda/function.go`.

Rules:

1. **Resolve at `Compare`/`apply` time, never in `Normalize`.** `Normalize` runs at config load for every resource, so a load-time lookup fires on every command and breaks one-run bootstrap (creating an `efs-filesystem` and its consumer in the same apply). Compare-time resolution runs per-resource in dependency-graph order and returns a wrapped error scoped to the resource instead of failing the whole load.
2. **Call the resolver at the top of every lifecycle entry point that needs the physical value** (`Compare`, `apply`, `applyDiffs`). Idempotency makes the repeat calls free.
3. **Store the resolved value through a pointer field** (e.g. `*ContainerVolumes`, `*FileSystem`) so a resolution performed in `Compare` persists into the apply phase despite value receivers.
4. **Resolve before diffing** so the comparison is against the physical IDs/ARNs AWS stores — comparing a name against an `fs-*` ID produces a perpetual diff.

Name matching is shared: a name matches an access point's ClientToken (the identity buildit's `efs-filesystem` assigns from the nested access point `name`) or its `Name` (for access points created outside buildit); ambiguity is an error.

## Lifecycle Integration

- **Normalize Phase**: Resolve ARNs here if needed for state comparison.
- **Apply Phase**: Resolve here if only needed for the final API call.
- **Compare Phase**: Preferred for values needed by both diffing and apply when the referenced resource may be created in the same run — see [EFS Volume References](#pattern-efs-volume-references).
