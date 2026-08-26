package resource

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/TouchBistro/buildit/util"
	"github.com/TouchBistro/goutils/color"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/efs"
	"github.com/aws/aws-sdk-go-v2/service/efs/types"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

var efsPermissionsRe = regexp.MustCompile(`^[0-7]{3,4}$`)

// EFSAccessPoint is an application-specific entry point into an EFS file system. It is defined as
// a nested item under an efs-filesystem (see efs_filesystem.go) and reconciled by the parent,
// which supplies the file system id.
//
// `name` is the access point's ClientToken (its EFS creation token) and the only identity buildit
// matches on — mirroring how the file system is identified by its CreationToken. The `Name` tag is
// never derived from it; set a `Name` under `tags` if you want one. To adopt an access point created
// outside buildit (e.g. in the console, which assigns a `console-<uuid>` token), set `name` to that
// ClientToken.
type EFSAccessPoint struct {
	Name          string            `yaml:"name"`
	PosixUser     *EFSPosixUser     `yaml:"posixUser,omitempty"`
	RootDirectory *EFSRootDirectory `yaml:"rootDirectory,omitempty"`
	Tags          map[string]string `yaml:"tags,omitempty"`
}

// EFSPosixUser is the POSIX identity applied to all NFS requests through an access point.
// Uid and Gid are pointers so an omitted value is distinguishable from a deliberate 0 (root);
// validate rejects a posixUser with either unset.
type EFSPosixUser struct {
	Uid           *int64  `yaml:"uid"`
	Gid           *int64  `yaml:"gid"`
	SecondaryGids []int64 `yaml:"secondaryGids,omitempty"`
}

// EFSRootDirectory is the directory an access point exposes as its root.
type EFSRootDirectory struct {
	Path         string           `yaml:"path,omitempty"`
	CreationInfo *EFSCreationInfo `yaml:"creationInfo,omitempty"`
}

// EFSCreationInfo specifies POSIX ownership/permissions used when EFS creates the root directory.
type EFSCreationInfo struct {
	OwnerUid    int64  `yaml:"ownerUid"`
	OwnerGid    int64  `yaml:"ownerGid"`
	Permissions string `yaml:"permissions"`
}

// Normalize sets defaults and sanitizes fields. globalTags are merged in from the parent. The
// `Name` tag is not synthesized: `name` is the ClientToken (identity), not a display tag, so a Name
// tag is applied only when the user sets one under `tags`.
func (r *EFSAccessPoint) Normalize(ctx context.Context, globalTags map[string]string) {
	r.Name = strings.TrimSpace(r.Name)
	if r.Tags == nil {
		r.Tags = make(map[string]string)
	}
	ResourceTags(r.Tags).Merge(globalTags)
}

// validate returns validation messages for this access point, prefixed for the parent's report.
func (r EFSAccessPoint) validate() []string {
	var msgs []string

	// required: access point name (its ClientToken / creation token, used to match the live set)
	if r.Name == "" {
		msgs = append(msgs, "accessPoints: name is required")
	}

	// required-when-present: posixUser uid/gid must be set explicitly so an access point never
	// silently runs as uid/gid 0 (root) because of an omitted field.
	if r.PosixUser != nil {
		if r.PosixUser.Uid == nil {
			msgs = append(msgs, fmt.Sprintf("accessPoints[%s]: posixUser.uid is required when posixUser is set", r.Name))
		}
		if r.PosixUser.Gid == nil {
			msgs = append(msgs, fmt.Sprintf("accessPoints[%s]: posixUser.gid is required when posixUser is set", r.Name))
		}
	}

	// format: root directory creation permissions must be an octal mode string
	if r.RootDirectory != nil && r.RootDirectory.CreationInfo != nil {
		if !efsPermissionsRe.MatchString(r.RootDirectory.CreationInfo.Permissions) {
			msgs = append(msgs, fmt.Sprintf("accessPoints[%s]: rootDirectory.creationInfo.permissions must be an octal string (e.g. 0755)", r.Name))
		}
	}

	return msgs
}

// clientToken returns the EFS idempotency key buildit assigns when creating this access point: the
// (length-bounded) access point name.
func (r EFSAccessPoint) clientToken() string {
	return awsw.EFSTruncateToken(r.Name)
}

// EFSAccessPointDiff captures the drift of an access point against its live AWS state. posixUser and
// rootDirectory are immutable (reported as a warning, never mutated); only tags are mutable. It
// mirrors the child-diff pattern used by load balancer listeners.
type EFSAccessPointDiff struct {
	BaseResourceDiff

	immutableDiff bool
	tagsDiff      bool
	tagDiff       util.TagDiffResult
	accessPointId string
}

// equals compares the desired access point against its existing AWS state (matched by ClientToken by
// the parent file system) and returns a non-nil diff when the immutable posixUser/rootDirectory
// drifts or the tags differ. It performs no AWS calls.
func (r EFSAccessPoint) equals(ctx context.Context, existing *types.AccessPointDescription) (ResourceDiff, error) {
	d := &EFSAccessPointDiff{accessPointId: aws.ToString(existing.AccessPointId)}
	d.Resource = existing

	diff := false
	// posixUser and rootDirectory are immutable; surface drift so it is visible, but it can only be
	// resolved by destroying & recreating the access point.
	if !efsPosixEqual(r.toPosixUser(), existing.PosixUser) || !efsRootDirEqual(r.toRootDirectory(), existing.RootDirectory) {
		diff = true
		d.immutableDiff = true
		d.Messages = append(d.Messages, "posixUser/rootDirectory is different (immutable; destroy & recreate the access point to change)")
	}

	apTags := filterReservedTags(awsw.EFSTagMap(existing.Tags))
	if td := TagDiffForContext(ctx, apTags, r.Tags); td.HasChanges() {
		diff = true
		d.tagsDiff = true
		d.tagDiff = td
		d.Messages = append(d.Messages, TagDiffSummary(apTags, td)...)
	}

	if !diff {
		return nil, nil
	}
	return d, nil
}

// applyDiff applies the mutable changes captured in d (tags) to the existing access point and warns
// on immutable drift. posixUser and rootDirectory cannot be changed in place.
func (r EFSAccessPoint) applyDiff(ctx context.Context, efsClient awsw.EFS, d *EFSAccessPointDiff) error {
	if d.immutableDiff {
		log.WithField("Name", r.Name).Warn("efs access point posixUser/rootDirectory is different (immutable; destroy & recreate the access point to change)")
	}
	if d.tagsDiff {
		if err := efsClient.AddResourceTags(ctx, d.accessPointId, d.tagDiff.Upserts()); err != nil {
			return errors.Wrapf(err, "error updating tags for efs access point %q", r.Name)
		}
		if err := efsClient.DeleteResourceTags(ctx, d.accessPointId, d.tagDiff.DeletedKeys()); err != nil {
			return errors.Wrapf(err, "error removing tags for efs access point %q", r.Name)
		}
		log.WithField("Name", r.Name).Infof("%s", color.Yellow("efs access point updated"))
	}
	return nil
}

func (r EFSAccessPoint) create(ctx context.Context, efsClient awsw.EFS, fsID string) error {
	if err := efsClient.WaitForFileSystemAvailable(ctx, fsID); err != nil {
		return errors.Wrapf(err, "error waiting for efs filesystem %q", fsID)
	}

	in := &efs.CreateAccessPointInput{
		ClientToken:   aws.String(r.clientToken()),
		FileSystemId:  aws.String(fsID),
		PosixUser:     r.toPosixUser(),
		RootDirectory: r.toRootDirectory(),
		Tags:          awsw.EFSTagSlice(r.Tags),
	}
	if _, err := efsClient.CreateAccessPoint(ctx, in); err != nil {
		return errors.Wrapf(err, "error creating efs access point %q", r.Name)
	}

	log.WithFields(log.Fields{
		"AccessPoint":  r.Name,
		"FileSystemId": fsID,
	}).Info(color.Green("efs access point created"))
	return nil
}

// deleteAccessPoint removes an access point and waits for it to disappear. Used by the parent to
// prune access points no longer desired. File-system teardown (EFSFileSystem.Destroy) instead
// deletes access points in bulk and polls the set once, so it does not call this per-AP helper.
func deleteAccessPoint(ctx context.Context, efsClient awsw.EFS, fsID string, ap types.AccessPointDescription) error {
	if _, err := efsClient.DeleteAccessPoint(ctx, &efs.DeleteAccessPointInput{
		AccessPointId: ap.AccessPointId,
	}); err != nil {
		return errors.Wrapf(err, "error deleting efs access point %q", aws.ToString(ap.AccessPointId))
	}
	apID := aws.ToString(ap.AccessPointId)
	if err := efsPollUntilDeleted(ctx, fmt.Sprintf("efs access point %q", apID), func(ctx context.Context) (bool, error) {
		aps, err := efsClient.AccessPointsForFileSystem(ctx, fsID)
		if err != nil {
			return false, errors.Wrapf(err, "error checking efs access point %q", apID)
		}
		for i := range aps {
			if aws.ToString(aps[i].AccessPointId) == apID {
				return false, nil
			}
		}
		return true, nil
	}); err != nil {
		return err
	}
	log.WithField("AccessPointId", apID).Infof("%s", color.Red("efs access point destroyed"))
	return nil
}

func (r EFSAccessPoint) toPosixUser() *types.PosixUser {
	if r.PosixUser == nil {
		return nil
	}
	pu := &types.PosixUser{
		Uid: r.PosixUser.Uid,
		Gid: r.PosixUser.Gid,
	}
	if len(r.PosixUser.SecondaryGids) > 0 {
		pu.SecondaryGids = r.PosixUser.SecondaryGids
	}
	return pu
}

func (r EFSAccessPoint) toRootDirectory() *types.RootDirectory {
	if r.RootDirectory == nil {
		return nil
	}
	rd := &types.RootDirectory{}
	if r.RootDirectory.Path != "" {
		rd.Path = aws.String(r.RootDirectory.Path)
	}
	if r.RootDirectory.CreationInfo != nil {
		rd.CreationInfo = &types.CreationInfo{
			OwnerUid:    aws.Int64(r.RootDirectory.CreationInfo.OwnerUid),
			OwnerGid:    aws.Int64(r.RootDirectory.CreationInfo.OwnerGid),
			Permissions: aws.String(r.RootDirectory.CreationInfo.Permissions),
		}
	}
	return rd
}

func efsPosixEqual(desired, current *types.PosixUser) bool {
	if desired == nil && current == nil {
		return true
	}
	if desired == nil || current == nil {
		return false
	}
	if aws.ToInt64(desired.Uid) != aws.ToInt64(current.Uid) {
		return false
	}
	if aws.ToInt64(desired.Gid) != aws.ToInt64(current.Gid) {
		return false
	}
	// AWS does not guarantee SecondaryGids ordering, so compare as a multiset to avoid a
	// spurious "different" warning when only the order changed.
	return int64MultisetEqual(desired.SecondaryGids, current.SecondaryGids)
}

func efsRootDirEqual(desired, current *types.RootDirectory) bool {
	if desired == nil && current == nil {
		return true
	}
	if desired == nil || current == nil {
		return false
	}
	if aws.ToString(desired.Path) != aws.ToString(current.Path) {
		return false
	}
	dci, cci := desired.CreationInfo, current.CreationInfo
	if dci == nil && cci == nil {
		return true
	}
	if dci == nil || cci == nil {
		return false
	}
	return aws.ToInt64(dci.OwnerUid) == aws.ToInt64(cci.OwnerUid) &&
		aws.ToInt64(dci.OwnerGid) == aws.ToInt64(cci.OwnerGid) &&
		aws.ToString(dci.Permissions) == aws.ToString(cci.Permissions)
}

// int64MultisetEqual reports whether two slices contain the same values, ignoring order.
func int64MultisetEqual(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	sa := slices.Clone(a)
	sb := slices.Clone(b)
	slices.Sort(sa)
	slices.Sort(sb)
	return slices.Equal(sa, sb)
}
