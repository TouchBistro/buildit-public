package resource

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/TouchBistro/goutils/color"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/efs"
	"github.com/aws/aws-sdk-go-v2/service/efs/types"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

// EFSMountTarget attaches an EFS file system to a subnet, making the file system reachable from
// that subnet's Availability Zone. It is defined as a nested item under an efs-filesystem (see
// efs_filesystem.go) and reconciled by the parent, which supplies the file system id.
//
// A file system has at most one mount target per subnet (one per Availability Zone), so a mount
// target is identified by its subnet. The subnet and IP address are fixed at creation; the
// attached security groups are mutable.
type EFSMountTarget struct {
	SubnetName string `yaml:"subnetName"`
	// SecurityGroupNames are the security groups attached to the mount target's network interface.
	// Empty means "unmanaged": on create AWS attaches the subnet VPC's default security group, and
	// on update the existing groups are left untouched (never diffed). Supply at least one group to
	// have buildit manage them.
	SecurityGroupNames []string `yaml:"securityGroupNames,omitempty"`
	IpAddress          string   `yaml:"ipAddress,omitempty"`
}

// Normalize sanitizes fields.
func (r *EFSMountTarget) Normalize(ctx context.Context) {
	r.SubnetName = strings.TrimSpace(r.SubnetName)
	r.IpAddress = strings.TrimSpace(r.IpAddress)
}

// validate returns validation messages for this mount target, prefixed for the parent's report.
func (r EFSMountTarget) validate() []string {
	var msgs []string

	// required: subnet the mount target lives in
	if r.SubnetName == "" {
		msgs = append(msgs, "mountTargets: subnetName is required")
	}

	// format: ip address must be a valid address when supplied
	if r.IpAddress != "" && net.ParseIP(r.IpAddress) == nil {
		msgs = append(msgs, fmt.Sprintf("mountTargets: ipAddress %q is not a valid IP address", r.IpAddress))
	}

	return msgs
}

// EFSMountTargetDiff captures the mutable drift of a mount target against its live AWS state.
// The subnet and IP address are immutable, so only the attached security groups can change on an
// existing mount target. It mirrors the child-diff pattern used by load balancer listeners.
type EFSMountTargetDiff struct {
	BaseResourceDiff

	securityGroupsDiff bool
	desiredSGs         []string
	mountTargetId      string
}

// equals compares the desired mount target against its existing AWS state (matched by subnet by
// the parent file system) and returns a non-nil diff only when the mutable security groups differ.
// vpcID is the subnet's owning VPC, used to resolve the configured security group names to IDs.
func (r EFSMountTarget) equals(ctx context.Context, ec2Client awsw.EC2, efsClient awsw.EFS, vpcID *string, existing *types.MountTargetDescription) (ResourceDiff, error) {
	desiredSGs, err := r.resolveSecurityGroupIDs(ctx, ec2Client, vpcID)
	if err != nil {
		return nil, err
	}
	currentSGs, err := efsClient.MountTargetSecurityGroups(ctx, aws.ToString(existing.MountTargetId))
	if err != nil {
		return nil, err
	}
	// Only the security groups are mutable; an empty desired set leaves them unmanaged.
	if !mountTargetSGChanged(currentSGs, desiredSGs) {
		return nil, nil
	}
	d := &EFSMountTargetDiff{
		securityGroupsDiff: true,
		desiredSGs:         desiredSGs,
		mountTargetId:      aws.ToString(existing.MountTargetId),
	}
	d.Resource = existing
	d.Messages = append(d.Messages, "security groups will be updated")
	return d, nil
}

// create provisions the mount target in its subnet. The subnet and security group names are
// resolved first; the file system must be available before a mount target can be attached.
func (r EFSMountTarget) create(ctx context.Context, efsClient awsw.EFS, ec2Client awsw.EC2, fsID string) error {
	subnetID, vpcID, err := r.resolveSubnet(ctx, ec2Client)
	if err != nil {
		return err
	}
	desiredSGs, err := r.resolveSecurityGroupIDs(ctx, ec2Client, vpcID)
	if err != nil {
		return err
	}
	if err := efsClient.WaitForFileSystemAvailable(ctx, fsID); err != nil {
		return errors.Wrapf(err, "error waiting for efs filesystem %q", fsID)
	}
	in := &efs.CreateMountTargetInput{
		FileSystemId:   aws.String(fsID),
		SubnetId:       aws.String(subnetID),
		SecurityGroups: desiredSGs,
	}
	if r.IpAddress != "" {
		in.IpAddress = aws.String(r.IpAddress)
	}
	if _, err := efsClient.CreateMountTarget(ctx, in); err != nil {
		return errors.Wrapf(err, "error creating efs mount target in subnet %q", r.SubnetName)
	}
	log.WithFields(log.Fields{
		"FileSystemId": fsID,
		"SubnetId":     subnetID,
	}).Info(color.Green("efs mount target created"))
	return nil
}

// applyDiff applies the mutable changes captured in d (security groups) to the existing mount target.
func (r EFSMountTarget) applyDiff(ctx context.Context, efsClient awsw.EFS, d *EFSMountTargetDiff) error {
	if !d.securityGroupsDiff {
		return nil
	}
	if _, err := efsClient.ModifyMountTargetSecurityGroups(ctx, &efs.ModifyMountTargetSecurityGroupsInput{
		MountTargetId:  aws.String(d.mountTargetId),
		SecurityGroups: d.desiredSGs,
	}); err != nil {
		return errors.Wrapf(err, "error updating security groups for efs mount target in subnet %q", r.SubnetName)
	}
	log.WithField("SubnetName", r.SubnetName).Infof("%s", color.Yellow("efs mount target security groups updated"))
	return nil
}

// resolveSubnet resolves the subnet name to its subnet ID and owning VPC ID.
func (r EFSMountTarget) resolveSubnet(ctx context.Context, ec2Client awsw.EC2) (subnetID string, vpcID *string, err error) {
	subnets, err := ec2Client.SubnetByNames(ctx, []string{r.SubnetName})
	if err != nil {
		return "", nil, errors.Wrapf(err, "error resolving subnet %q for mount target", r.SubnetName)
	}
	if len(subnets) == 0 {
		return "", nil, errors.Errorf("subnet %q not found for mount target", r.SubnetName)
	}
	return aws.ToString(subnets[0].SubnetId), subnets[0].VpcId, nil
}

// resolveSecurityGroupIDs resolves the configured security group names to IDs within the VPC.
func (r EFSMountTarget) resolveSecurityGroupIDs(ctx context.Context, ec2Client awsw.EC2, vpcID *string) ([]string, error) {
	if len(r.SecurityGroupNames) == 0 {
		return nil, nil
	}
	ids, err := ec2Client.SecurityGroupIdsByNames(ctx, vpcID, r.SecurityGroupNames)
	if err != nil {
		return nil, errors.Wrapf(err, "error resolving security groups for mount target in subnet %q", r.SubnetName)
	}
	return ids, nil
}

// mountTargetSGChanged reports whether the desired security groups differ from the current set.
// An empty desired set leaves the security groups unmanaged (no change reported).
func mountTargetSGChanged(currentSGs, desiredSGs []string) bool {
	return len(desiredSGs) > 0 && !stringSetEqual(currentSGs, desiredSGs)
}

// deleteMountTarget removes a mount target and waits for it to disappear. Used by the parent to
// prune mount targets whose subnet is no longer desired and during file system teardown.
func deleteMountTarget(ctx context.Context, efsClient awsw.EFS, mt types.MountTargetDescription) error {
	if _, err := efsClient.DeleteMountTarget(ctx, &efs.DeleteMountTargetInput{
		MountTargetId: mt.MountTargetId,
	}); err != nil {
		return errors.Wrapf(err, "error deleting efs mount target %q", aws.ToString(mt.MountTargetId))
	}
	subnetID := aws.ToString(mt.SubnetId)
	fsID := aws.ToString(mt.FileSystemId)
	if err := efsPollUntilDeleted(ctx, fmt.Sprintf("efs mount target in subnet %q", subnetID), func(ctx context.Context) (bool, error) {
		cur, err := efsClient.MountTargetForFileSystemBySubnet(ctx, fsID, subnetID)
		if err != nil {
			return false, errors.Wrapf(err, "error checking efs mount target in subnet %q", subnetID)
		}
		return cur == nil, nil
	}); err != nil {
		return err
	}
	log.WithField("SubnetId", subnetID).Infof("%s", color.Red("efs mount target destroyed"))
	return nil
}

// stringSetEqual reports whether two string slices contain the same elements, ignoring order.
func stringSetEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[string]int, len(a))
	for _, s := range a {
		seen[s]++
	}
	for _, s := range b {
		if seen[s] == 0 {
			return false
		}
		seen[s]--
	}
	return true
}
