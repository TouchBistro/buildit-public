# EFS Access Point (nested under `efs-filesystem`)

An access point is an application-specific entry point into an [EFS file system](./efs_filesystem.md).

Access points are **not a standalone resource**. Declare them as items of the `accessPoints` list on
an [`efs-filesystem`](./efs_filesystem.md): the parent file system supplies the file system id and
creates, updates, and prunes its access points as part of its own apply. Because they are nested
under the file system, there is no separate resource name, `filesystemName`, or `dependsOn` —
ordering against the parent file system is implicit.

An access point is identified within its file system by its `name`, which is the access point's
**`ClientToken`** (its EFS creation token) — mirroring how an [`efs-filesystem`](./efs_filesystem.md)
is identified by its CreationToken. buildit matches a desired access point to the live set **only**
by ClientToken; the `Name` tag is never used for matching and is never auto-derived from `name` —
set a `Name` under `tags` if you want a display name. To adopt an access point created outside
buildit (e.g. in the console, which assigns an opaque `console-<uuid>` ClientToken), set `name` to
that ClientToken — exactly as you set a file system's resource name to its `quickCreated-<uuid>`
CreationToken.

## Immutability

`posixUser` and `rootDirectory` are fixed at creation. A change to either surfaces a diff message
and a warning, but is not applied — destroy and recreate the access point to change them. `tags`
are mutable.

## Fields

| Field | Description | DataType | Required | Default |
| :--- | :--- | :--- | :--- | :--- |
| `name` | The access point's `ClientToken` (creation token) and match identity. Not a tag | `string` | Yes | |
| `posixUser` | POSIX identity applied to all NFS requests through the access point | `object` | No | |
| `rootDirectory` | Directory the access point exposes as its root | `object` | No | |
| `tags` | Tags to apply to the access point. Merged with `globalTags`. A `Name` tag is applied only if set here | `map[string]string` | No | |

### `posixUser`

| Field | Description | DataType | Required |
| :--- | :--- | :--- | :--- |
| `uid` | POSIX user ID | `int` | Yes |
| `gid` | POSIX group ID | `int` | Yes |
| `secondaryGids` | Secondary POSIX group IDs | `[]int` | No |

### `rootDirectory`

| Field | Description | DataType | Required |
| :--- | :--- | :--- | :--- |
| `path` | Path on the file system to expose as the root | `string` | No |
| `creationInfo` | POSIX ownership/permissions used when EFS creates the path | `object` | No |

#### `creationInfo`

| Field | Description | DataType | Required |
| :--- | :--- | :--- | :--- |
| `ownerUid` | POSIX user ID that owns the root directory | `int` | Yes |
| `ownerGid` | POSIX group ID that owns the root directory | `int` | Yes |
| `permissions` | Octal permission string, e.g. `"0755"` | `string` | Yes |

## Example

```yaml
resources:
  efs-filesystem:
    app-fs:
      encrypted: true
      accessPoints:
        - name: app-data
          posixUser:
            uid: 1000
            gid: 1000
            secondaryGids: [2000]
          rootDirectory:
            path: /app
            creationInfo:
              ownerUid: 1000
              ownerGid: 1000
              permissions: "0755"
          tags:
            service: app
```
