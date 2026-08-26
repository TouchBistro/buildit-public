package awsw

import (
	"context"

	"fmt"
	"strings"
	"time"

	"github.com/TouchBistro/awesome/providers"
	"github.com/TouchBistro/buildit/client"
	"github.com/TouchBistro/buildit/util"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/efs"
	"github.com/aws/aws-sdk-go-v2/service/efs/types"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

const (
	// efsFileSystemIDPrefix marks a physical EFS file system ID (e.g. fs-0763b5806d2a9e5ab).
	efsFileSystemIDPrefix = "fs-"
	// efsAccessPointIDPrefix marks a physical EFS access point ID (e.g. fsap-02e6b009fcdea54d0).
	efsAccessPointIDPrefix = "fsap-"
	// efsWaitTimeout bounds how long we poll for a file system to reach a target state.
	// File-system transitions (notably leaving provisioned throughput, or large creates)
	// regularly take several minutes, so this is generous; it only guards against a stuck state.
	efsWaitTimeout = 15 * time.Minute
	// efsWaitInterval is the delay between file system state polls.
	efsWaitInterval = 5 * time.Second
)

// efsWaitErr converts a context error into a user-facing timeout/cancellation error for the
// supplied action (e.g. `EFS file system "fs-1" to become available`).
func efsWaitErr(err error, action string) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return errors.Errorf("timed out waiting for %s", action)
	}
	return errors.Wrapf(err, "context cancelled while waiting for %s", action)
}

type EFS struct {
	*efs.Client
}

// NewEFS creates a new instance of EFS wrapper
func NewEFS(ctx context.Context, providerName string) EFS {
	return EFS{client.EFS(ctx, providerName)}
}

// forProvider returns the wrapper scoped to the supplied provider, or the receiver when the
// provider is empty (i.e. the identifier had no <provider>:: prefix).
func (e EFS) forProvider(ctx context.Context, provider string) (EFS, error) {
	if provider == "" {
		return e, nil
	}
	if _, err := providers.Get(provider); err != nil {
		return EFS{}, errors.Wrapf(err, "provider %q not found", provider)
	}
	return NewEFS(ctx, provider), nil
}

// FileSystemArnForIdentifier resolves an EFS File System ARN from an identifier (ARN or FileSystemId).
func (e EFS) FileSystemArnForIdentifier(ctx context.Context, identifier string) (*string, error) {
	resource, provider := ParseIdentifier(identifier)
	if strings.HasPrefix(resource, "arn:") {
		return &resource, nil
	}

	efsService, err := e.forProvider(ctx, provider)
	if err != nil {
		return nil, err
	}

	// 1. Try Lookup by File System ID
	// Verify it exists
	out, err := efsService.DescribeFileSystems(ctx, &efs.DescribeFileSystemsInput{
		FileSystemId: &resource,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to describe file system %q", resource)
	}

	if len(out.FileSystems) == 0 {
		return nil, errors.Errorf("file system %q not found", resource)
	}

	return out.FileSystems[0].FileSystemArn, nil
}

// accessPointForName returns the access point matching the supplied name. The name is compared
// against each access point's ClientToken (the identity buildit's efs-filesystem resource assigns
// from the nested access point `name`, truncated to EFS's 64-char token limit) and its Name (for
// access points created outside buildit that carry a Name tag). A name matching more than one
// access point is an error.
func (e EFS) accessPointForName(ctx context.Context, name string) (*types.AccessPointDescription, error) {
	clientToken := EFSTruncateToken(name)
	var match *types.AccessPointDescription
	var token *string
	for {
		out, err := e.DescribeAccessPoints(ctx, &efs.DescribeAccessPointsInput{
			NextToken: token,
		})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to describe access points while looking up %q", name)
		}
		for i := range out.AccessPoints {
			ap := out.AccessPoints[i]
			if aws.ToString(ap.ClientToken) != clientToken && aws.ToString(ap.Name) != name {
				continue
			}
			if match != nil {
				return nil, errors.Errorf("multiple EFS access points found for name %q; use the fsap- ID instead", name)
			}
			match = &ap
		}
		if out.NextToken == nil {
			break
		}
		token = out.NextToken
	}
	if match == nil {
		return nil, errors.Errorf("EFS access point %q not found", name)
	}
	return match, nil
}

// AccessPointArnForIdentifier resolves an EFS access point ARN from an identifier (ARN,
// AccessPointId, or name). Tiered: an arn:* identifier passes through unchanged; an fsap-* ID is
// described directly; anything else is matched by name (ClientToken or Name, see
// accessPointForName).
func (e EFS) AccessPointArnForIdentifier(ctx context.Context, identifier string) (*string, error) {
	resource, provider := ParseIdentifier(identifier)
	if strings.HasPrefix(resource, "arn:") {
		return &resource, nil
	}

	efsService, err := e.forProvider(ctx, provider)
	if err != nil {
		return nil, err
	}

	if strings.HasPrefix(resource, efsAccessPointIDPrefix) {
		out, err := efsService.DescribeAccessPoints(ctx, &efs.DescribeAccessPointsInput{
			AccessPointId: aws.String(resource),
		})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to describe access point %q", resource)
		}
		if len(out.AccessPoints) == 0 {
			return nil, errors.Errorf("EFS access point %q not found", resource)
		}
		return out.AccessPoints[0].AccessPointArn, nil
	}

	ap, err := efsService.accessPointForName(ctx, resource)
	if err != nil {
		return nil, err
	}
	return ap.AccessPointArn, nil
}

// FileSystemIDForIdentifier resolves an EFS file system ID from an identifier (FileSystemId or
// buildit efs-filesystem resource name). A physical ID (fs-*) is returned unchanged; anything
// else is treated as a resource name and resolved via the file system's CreationToken (the EFS
// idempotency key buildit sets to the resource name at create time).
func (e EFS) FileSystemIDForIdentifier(ctx context.Context, identifier string) (*string, error) {
	resource, provider := ParseIdentifier(identifier)
	if strings.HasPrefix(resource, efsFileSystemIDPrefix) {
		return &resource, nil
	}

	efsService, err := e.forProvider(ctx, provider)
	if err != nil {
		return nil, err
	}

	fs, err := efsService.FileSystemByCreationToken(ctx, resource)
	if err != nil {
		return nil, err
	}
	if fs == nil {
		return nil, errors.Errorf("EFS file system with creation token %q not found", resource)
	}
	return fs.FileSystemId, nil
}

// AccessPointIDForIdentifier resolves an EFS access point ID from an identifier (AccessPointId
// or name). A physical ID (fsap-*) is returned unchanged; anything else is matched by name
// (ClientToken or Name, see accessPointForName).
func (e EFS) AccessPointIDForIdentifier(ctx context.Context, identifier string) (*string, error) {
	resource, provider := ParseIdentifier(identifier)
	if strings.HasPrefix(resource, efsAccessPointIDPrefix) {
		return &resource, nil
	}

	efsService, err := e.forProvider(ctx, provider)
	if err != nil {
		return nil, err
	}

	ap, err := efsService.accessPointForName(ctx, resource)
	if err != nil {
		return nil, err
	}
	return ap.AccessPointId, nil
}

// ResolveVolumeIDs resolves optional EFS volume references (a file system identifier and an
// access point identifier) to their physical IDs. Nil inputs pass through, as do physical IDs
// (fs-* / fsap-*), so the call is idempotent and only reaches AWS for name references. Shared by
// any resource whose config mounts EFS volumes (e.g. taskdef efs volumes).
func (e EFS) ResolveVolumeIDs(ctx context.Context, fileSystemID, accessPointID *string) (*string, *string, error) {
	if fileSystemID != nil {
		id, err := e.FileSystemIDForIdentifier(ctx, *fileSystemID)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "error resolving efs file system reference %v", *fileSystemID)
		}
		fileSystemID = id
	}
	if accessPointID != nil {
		id, err := e.AccessPointIDForIdentifier(ctx, *accessPointID)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "error resolving efs access point reference %v", *accessPointID)
		}
		accessPointID = id
	}
	return fileSystemID, accessPointID, nil
}

// EFSTruncateToken bounds a token to EFS's 64-character CreationToken/ClientToken limit, keeping
// the suffix (the most distinctive part of generated resource names).
func EFSTruncateToken(token string) string {
	if len(token) > 64 {
		return token[len(token)-64:]
	}
	return token
}

// FileSystemByCreationToken returns the file system whose CreationToken matches the supplied
// token, or nil if no such file system exists. CreationToken is the EFS idempotency key (set
// to the buildit resource name at create time); it is immutable and unique per region, so it
// is a stable identity for lookup and import. DescribeFileSystems filters on it server-side.
func (e EFS) FileSystemByCreationToken(ctx context.Context, token string) (*types.FileSystemDescription, error) {
	out, err := e.DescribeFileSystems(ctx, &efs.DescribeFileSystemsInput{
		CreationToken: aws.String(token),
	})
	if err != nil {
		var nf *types.FileSystemNotFound
		if errors.As(err, &nf) {
			return nil, nil
		}
		return nil, errors.Wrapf(err, "failed to describe file systems while looking up creation token %q", token)
	}
	if len(out.FileSystems) == 0 {
		return nil, nil
	}
	return &out.FileSystems[0], nil
}

// AccessPointsForFileSystem returns all access points that belong to the supplied file system.
func (e EFS) AccessPointsForFileSystem(ctx context.Context, fileSystemID string) ([]types.AccessPointDescription, error) {
	var accessPoints []types.AccessPointDescription
	var token *string
	for {
		out, err := e.DescribeAccessPoints(ctx, &efs.DescribeAccessPointsInput{
			FileSystemId: aws.String(fileSystemID),
			NextToken:    token,
		})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to describe access points for file system %q", fileSystemID)
		}
		accessPoints = append(accessPoints, out.AccessPoints...)
		if out.NextToken == nil {
			break
		}
		token = out.NextToken
	}
	return accessPoints, nil
}

// MountTargetsForFileSystem returns all mount targets that belong to the supplied file system.
func (e EFS) MountTargetsForFileSystem(ctx context.Context, fileSystemID string) ([]types.MountTargetDescription, error) {
	var mountTargets []types.MountTargetDescription
	var marker *string
	for {
		out, err := e.DescribeMountTargets(ctx, &efs.DescribeMountTargetsInput{
			FileSystemId: aws.String(fileSystemID),
			Marker:       marker,
		})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to describe mount targets for file system %q", fileSystemID)
		}
		mountTargets = append(mountTargets, out.MountTargets...)
		if out.NextMarker == nil {
			break
		}
		marker = out.NextMarker
	}
	return mountTargets, nil
}

// MountTargetForFileSystemBySubnet returns the file system's mount target in the supplied
// subnet, or nil if none exists. A file system has at most one mount target per subnet.
func (e EFS) MountTargetForFileSystemBySubnet(ctx context.Context, fileSystemID, subnetID string) (*types.MountTargetDescription, error) {
	mountTargets, err := e.MountTargetsForFileSystem(ctx, fileSystemID)
	if err != nil {
		return nil, err
	}
	for i := range mountTargets {
		if aws.ToString(mountTargets[i].SubnetId) == subnetID {
			return &mountTargets[i], nil
		}
	}
	return nil, nil
}

// MountTargetSecurityGroups returns the security group IDs attached to a mount target.
func (e EFS) MountTargetSecurityGroups(ctx context.Context, mountTargetID string) ([]string, error) {
	out, err := e.DescribeMountTargetSecurityGroups(ctx, &efs.DescribeMountTargetSecurityGroupsInput{
		MountTargetId: aws.String(mountTargetID),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to describe security groups for mount target %q", mountTargetID)
	}
	return out.SecurityGroups, nil
}

// BackupPolicyStatus returns the backup policy status for the file system. When no backup
// policy has ever been set, EFS returns PolicyNotFound, which is reported as DISABLED.
func (e EFS) BackupPolicyStatus(ctx context.Context, fileSystemID string) (types.Status, error) {
	out, err := e.DescribeBackupPolicy(ctx, &efs.DescribeBackupPolicyInput{
		FileSystemId: aws.String(fileSystemID),
	})
	if err != nil {
		var nf *types.PolicyNotFound
		if errors.As(err, &nf) {
			return types.StatusDisabled, nil
		}
		return "", errors.Wrapf(err, "failed to describe backup policy for file system %q", fileSystemID)
	}
	if out.BackupPolicy == nil {
		return types.StatusDisabled, nil
	}
	return out.BackupPolicy.Status, nil
}

// LifecyclePolicies returns the lifecycle policies configured for the file system.
func (e EFS) LifecyclePolicies(ctx context.Context, fileSystemID string) ([]types.LifecyclePolicy, error) {
	out, err := e.DescribeLifecycleConfiguration(ctx, &efs.DescribeLifecycleConfigurationInput{
		FileSystemId: aws.String(fileSystemID),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to describe lifecycle configuration for file system %q", fileSystemID)
	}
	return out.LifecyclePolicies, nil
}

// ResourceTags returns the tags for an EFS resource (file system or access point) by id.
func (e EFS) ResourceTags(ctx context.Context, resourceID string) (map[string]string, error) {
	tags := make(map[string]string)
	var token *string
	for {
		out, err := e.ListTagsForResource(ctx, &efs.ListTagsForResourceInput{
			ResourceId: aws.String(resourceID),
			NextToken:  token,
		})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to list tags for EFS resource %q", resourceID)
		}
		for _, t := range out.Tags {
			tags[aws.ToString(t.Key)] = aws.ToString(t.Value)
		}
		if out.NextToken == nil {
			break
		}
		token = out.NextToken
	}
	return tags, nil
}

// AddResourceTags adds or updates the supplied tags on an EFS resource by id.
func (e EFS) AddResourceTags(ctx context.Context, resourceID string, tags map[string]string) error {
	if len(tags) == 0 {
		return nil
	}
	_, err := e.TagResource(ctx, &efs.TagResourceInput{
		ResourceId: aws.String(resourceID),
		Tags:       EFSTagSlice(tags),
	})
	if err != nil {
		return errors.Wrapf(err, "failed to update tags for EFS resource %q", resourceID)
	}
	log.WithField("ResourceId", resourceID).Infof("%v tags added to EFS resource", len(tags))
	return nil
}

// DeleteResourceTags removes the supplied tag keys from an EFS resource by id.
func (e EFS) DeleteResourceTags(ctx context.Context, resourceID string, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	_, err := e.UntagResource(ctx, &efs.UntagResourceInput{
		ResourceId: aws.String(resourceID),
		TagKeys:    keys,
	})
	if err != nil {
		return errors.Wrapf(err, "failed to delete tags for EFS resource %q", resourceID)
	}
	log.WithField("ResourceId", resourceID).Infof("%v tags deleted from EFS resource", len(keys))
	return nil
}

// WaitForFileSystemAvailable blocks until the file system reaches the available state,
// the context is cancelled, or the bounded deadline elapses.
func (e EFS) WaitForFileSystemAvailable(ctx context.Context, fileSystemID string) error {
	ctx, cancel := context.WithTimeout(ctx, efsWaitTimeout)
	defer cancel()
	action := fmt.Sprintf("EFS file system %q to become available", fileSystemID)
	for {
		if err := ctx.Err(); err != nil {
			return efsWaitErr(err, action)
		}
		out, err := e.DescribeFileSystems(ctx, &efs.DescribeFileSystemsInput{
			FileSystemId: aws.String(fileSystemID),
		})
		if err != nil {
			// A deadline that fires mid-describe surfaces as a context error; report it as a timeout.
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				return efsWaitErr(err, action)
			}
			return errors.Wrapf(err, "failed to describe file system %q", fileSystemID)
		}
		if len(out.FileSystems) > 0 {
			switch out.FileSystems[0].LifeCycleState {
			case types.LifeCycleStateAvailable:
				return nil
			case types.LifeCycleStateError:
				return errors.Errorf("EFS file system %q entered error state", fileSystemID)
			}
		}
		if err := util.SleepWithContext(ctx, efsWaitInterval); err != nil {
			return efsWaitErr(err, action)
		}
	}
}

// WaitForFileSystemDeleted blocks until the file system no longer exists, the context is
// cancelled, or the bounded deadline elapses.
func (e EFS) WaitForFileSystemDeleted(ctx context.Context, fileSystemID string) error {
	ctx, cancel := context.WithTimeout(ctx, efsWaitTimeout)
	defer cancel()
	action := fmt.Sprintf("EFS file system %q to delete", fileSystemID)
	for {
		if err := ctx.Err(); err != nil {
			return efsWaitErr(err, action)
		}
		out, err := e.DescribeFileSystems(ctx, &efs.DescribeFileSystemsInput{
			FileSystemId: aws.String(fileSystemID),
		})
		if err != nil {
			var nf *types.FileSystemNotFound
			if errors.As(err, &nf) {
				return nil
			}
			// A deadline that fires mid-describe surfaces as a context error; report it as a timeout.
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				return efsWaitErr(err, action)
			}
			return errors.Wrapf(err, "failed to describe file system %q", fileSystemID)
		}
		if len(out.FileSystems) == 0 {
			return nil
		}
		if err := util.SleepWithContext(ctx, efsWaitInterval); err != nil {
			return efsWaitErr(err, action)
		}
	}
}

// EFSTagSlice converts a tag map into the []types.Tag shape EFS APIs expect.
func EFSTagSlice(tags map[string]string) []types.Tag {
	if len(tags) == 0 {
		return nil
	}
	awsTags := make([]types.Tag, 0, len(tags))
	for k, v := range tags {
		awsTags = append(awsTags, types.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}
	return awsTags
}

// EFSTagMap converts a []types.Tag into a tag map.
func EFSTagMap(tags []types.Tag) map[string]string {
	m := make(map[string]string, len(tags))
	for _, t := range tags {
		m[aws.ToString(t.Key)] = aws.ToString(t.Value)
	}
	return m
}
