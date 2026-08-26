# EFS Mount Target (nested under `efs-filesystem`)

A mount target attaches an [EFS file system](./efs_filesystem.md) to a subnet so the file system can
be mounted from that subnet's Availability Zone.

Mount targets are **not a standalone resource**. Declare them as items of the `mountTargets` list on
an [`efs-filesystem`](./efs_filesystem.md): the parent file system supplies the file system id and
creates, updates, and prunes its mount targets as part of its own apply. Because they are nested
under the file system, there is no separate resource name, `filesystemName`, or `dependsOn` —
ordering against the parent file system is implicit.

A file system has **at most one mount target per subnet** (one per Availability Zone), so a mount
target is identified by its subnet. List one entry per subnet you want the file system reachable
from.

Only IPv4 is supported — EFS mount targets default to IPv4 and buildit does not expose an
`IpAddressType`, so dual-stack/IPv6 mount targets aren't configurable through this resource.

## Immutability

`subnetName` and `ipAddress` are fixed at creation. `securityGroupNames` are mutable — a change is
applied in place via `ModifyMountTargetSecurityGroups`.

Leaving `securityGroupNames` empty means **unmanaged**, and this differs by lifecycle: on create AWS
attaches the subnet VPC's default security group, while on update the existing groups are left
untouched (no diff is reported). Supply at least one group for buildit to manage them; there is no
way to reset a mount target back to the default group through an empty list.

## Fields

| Field | Description | DataType | Required | Default |
| :--- | :--- | :--- | :--- | :--- |
| `subnetName` | Name of the subnet (`Name` tag) the mount target lives in | `string` | Yes | |
| `securityGroupNames` | Security groups to attach to the mount target's network interface | `[]string` | No | subnet default SG |
| `ipAddress` | Static IP within the subnet for the mount target. Immutable | `string` | No | auto-assigned |

## Example

```yaml
resources:
  security-group:
    efs-sg:
      description: allow NFS from app tier
      vpcName: example-vpc
      inboundRules:
        - protocol: tcp
          fromPort: 2049
          toPort: 2049
          cidrBlocks: ["10.0.0.0/16"]

  efs-filesystem:
    app-fs:
      encrypted: true
      mountTargets:
        - subnetName: example-private-a
          securityGroupNames:
            - efs-sg
        - subnetName: example-private-b
          securityGroupNames:
            - efs-sg
```
