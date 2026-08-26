# EFS Filesystem `efs-filesystem`

Creates an Amazon EFS file system and manages its performance, protection, and lifecycle
settings, along with its [mount targets](./efs_mount_target.md) and
[access points](./efs_access_point.md) as nested `mountTargets` and `accessPoints` lists. The
children cannot exist without the file system, so they are managed here rather than as standalone
resources and are created, updated, and pruned as part of applying the file system.

> You can qualify the resource name with a provider prefix or supply the provider name in the
> standard `provider` field of the resource to indicate the provider to use. If neither is
> supplied, `main` is used as the default provider name.

The buildit resource name is used as the file system's **CreationToken** (the EFS idempotency
key) and as its `Name` tag, so a given resource maps to a stable file system across runs.

## Matching & import

buildit matches each resource to its live AWS object by a stable identity, so it adopts existing
infrastructure instead of creating duplicates:

- **File system** — matched by its **CreationToken**, which is the buildit resource name. To import
  an existing file system, set the resource name to that file system's CreationToken (a
  console-created file system uses a `quickCreated-<uuid>` token). `kmsKeyId` compares equal whether
  given as an alias, key id, or ARN, so it does not show spurious drift.
- **Mount target** — matched by its **subnet** (a file system has at most one mount target per
  subnet), so an existing mount target in a declared subnet is adopted.
- **Access point** — matched by its **ClientToken**, which is the access point's `name` (the same
  model as the file system's CreationToken). A live access point matches only when its `ClientToken`
  equals the configured `name`; the `Name` tag is never used for matching. To adopt an access point
  created outside buildit — e.g. in the console, which assigns an opaque `console-<uuid>` ClientToken
  — set its `name` to that ClientToken.

The `Name` tag is never auto-derived on any of these resources; it is an ordinary tag, applied only
when set under `tags`. (As with any tag, omitting a `Name` from `tags` while the live resource has
one will remove it.)

Check out AWS documentation for `CreateFileSystem` [here](https://docs.aws.amazon.com/efs/latest/ug/API_CreateFileSystem.html).

## Immutability

`encrypted`, `kmsKeyId`, and `performanceMode` cannot be changed after creation. A change to any
of them surfaces as a diff message and a warning during apply, but is **not** applied — destroy
and recreate the file system to change them.

`throughputMode` / `provisionedThroughputMibps`, `backupPolicy`, `lifecyclePolicies`, and `tags`
are all mutable.

## Fully managed (omit reverts to AWS default)

`backupPolicy` and `lifecyclePolicies` follow a Terraform-style ownership model: your config is the
**full desired state**. Omitting a field does not leave the live value untouched — it reverts the
file system to the AWS default:

- Omitting `backupPolicy` → reverts to **DISABLED** (the `CreateFileSystem` API default).
- Omitting `lifecyclePolicies` → reverts to **no policies**, clearing any live transition rules.

⚠️ This means removing a field is a mutating change: **omitting `backupPolicy` disables automatic
backups on a file system that currently has them enabled**, and omitting `lifecyclePolicies` clears
live transition rules. Keep declaring them to keep them.

For `lifecyclePolicies`, an omitted key, an empty block (`lifecyclePolicies: {}`), and a null value
are equivalent — all mean "no lifecycle policies". A partial block (e.g. only `transitionToIA`)
replaces the whole live configuration with just what you list.

## Fields

| Field | Description | DataType | Required | Default |
| :--- | :--- | :--- | :--- | :--- |
| Name (Resource Name) | The buildit resource name; used as the CreationToken and `Name` tag | `string` | Yes | |
| `encrypted` | Encrypt the file system at rest. Immutable | `bool` | No | `true` |
| `kmsKeyId` | KMS key id/alias/ARN for encryption. Requires `encrypted: true`. Immutable | `string` | No | EFS default key |
| `performanceMode` | `generalPurpose` or `maxIO`. Immutable | `string` | No | `generalPurpose` |
| `throughputMode` | `bursting`, `elastic`, or `provisioned` | `string` | No | `bursting` |
| `provisionedThroughputMibps` | Provisioned throughput in MiBps. Required when `throughputMode: provisioned` | `float` | Conditional | |
| `backupPolicy` | `ENABLED` or `DISABLED` automatic backups (AWS Backup). Omitting reverts to the AWS default | `string` | No | `DISABLED` |
| `lifecyclePolicies` | Storage-class transition rules (see below). Omitting reverts to the AWS default | `object` | No | none (cleared) |
| `tags` | Tags to apply to the file system. Merged with `globalTags` | `map[string]string` | No | |
| `mountTargets` | Mount targets to attach (one per subnet). See [efs-mount-target](./efs_mount_target.md) | `[]object` | No | `[]` |
| `accessPoints` | Access points to expose. See [efs-access-point](./efs_access_point.md) | `[]object` | No | `[]` |
| `dependsOn` | The `buildit` resources this resource depends on | `[]string` | No | `[]` |

> `mountTargets` and `accessPoints` are reconciled to match what you declare: a child present in
> AWS but absent from the list is **deleted**. Omit a list (or leave it empty) only if you intend to
> have no mount targets / access points.

### `lifecyclePolicies`

| Field | Description | Allowed values |
| :--- | :--- | :--- |
| `transitionToIA` | When to move files to Infrequent Access storage | `AFTER_1_DAY`, `AFTER_7_DAYS`, `AFTER_14_DAYS`, `AFTER_30_DAYS`, `AFTER_60_DAYS`, `AFTER_90_DAYS`, `AFTER_180_DAYS`, `AFTER_270_DAYS`, `AFTER_365_DAYS` |
| `transitionToArchive` | When to move files to Archive storage | `AFTER_1_DAY` … `AFTER_365_DAYS` (Elastic throughput + General Purpose only) |
| `transitionToPrimaryStorageClass` | Move files back to Standard on access | `AFTER_1_ACCESS` |

## Examples

Basic encrypted file system:

```yaml
resources:
  efs-filesystem:
    my-shared-fs:
      encrypted: true
      tags:
        Environment: production
```

Provisioned throughput with backups and lifecycle management:

```yaml
resources:
  efs-filesystem:
    analytics-fs:
      encrypted: true
      throughputMode: provisioned
      provisionedThroughputMibps: 50
      backupPolicy: ENABLED
      lifecyclePolicies:
        transitionToIA: AFTER_30_DAYS
        transitionToArchive: AFTER_90_DAYS
        transitionToPrimaryStorageClass: AFTER_1_ACCESS
      tags:
        team: data-platform
```

File system with mount targets and an access point (children are nested):

```yaml
resources:
  efs-filesystem:
    app-fs:
      encrypted: true
      mountTargets:
        - subnetName: example-private-a
          securityGroupNames: [efs-sg]
        - subnetName: example-private-b
          securityGroupNames: [efs-sg]
      accessPoints:
        - name: app-data
          posixUser:
            uid: 1000
            gid: 1000
          rootDirectory:
            path: /app
            creationInfo:
              ownerUid: 1000
              ownerGid: 1000
              permissions: "0755"
```

See [efs-mount-target](./efs_mount_target.md) and [efs-access-point](./efs_access_point.md) for the
full set of nested fields.
