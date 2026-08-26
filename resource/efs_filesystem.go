package resource

import (
	"context"
	stderrors "errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/TouchBistro/buildit/util"
	"github.com/TouchBistro/goutils/color"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/efs"
	"github.com/aws/aws-sdk-go-v2/service/efs/types"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

// efsDeletePollInterval is the delay between polls while waiting for a nested EFS object
// (access point, mount target) to finish deleting before the file system can be removed.
const efsDeletePollInterval = 5 * time.Second

// efsDeleteTimeout bounds how long we wait for a nested EFS object to finish deleting. It is
// shorter than efsWaitTimeout (file-system transitions) because access point and mount target
// deletions are quick; the bound only guards against a stuck delete.
const efsDeleteTimeout = 2 * time.Minute

// efsPollUntilDeleted polls check at efsDeletePollInterval until it reports the object is gone,
// bounded by efsDeleteTimeout. resource is used only to phrase the timeout error.
func efsPollUntilDeleted(ctx context.Context, resource string, check func(context.Context) (bool, error)) error {
	attempts := int(efsDeleteTimeout / efsDeletePollInterval)
	for range attempts {
		gone, err := check(ctx)
		if err != nil {
			return err
		}
		if gone {
			return nil
		}
		if err := util.SleepWithContext(ctx, efsDeletePollInterval); err != nil {
			return errors.Wrapf(err, "context cancelled while waiting for %s to delete", resource)
		}
	}
	return errors.Errorf("timed out waiting for %s to delete", resource)
}

// EFSFileSystem represents an Amazon EFS file system. The buildit resource name is used as the
// file system's CreationToken (the EFS idempotency key), which is immutable and unique per region
// and therefore the stable identity buildit looks the file system up by — both for reconciliation
// and for importing an existing file system (set the resource name to its CreationToken).
//
// Mount targets and access points cannot exist without a file system and are linked to it by
// FileSystemId, a creation-time value, so they are managed as nested lists here rather than as
// standalone resources. The file system resolves its id once and reconciles each child against it.
type EFSFileSystem struct {
	BaseResource `yaml:",inline"`
	Name                       string                `yaml:"-"`
	Encrypted                  *bool                 `yaml:"encrypted"`
	KmsKeyId                   *string               `yaml:"kmsKeyId,omitempty"`
	PerformanceMode            string                `yaml:"performanceMode,omitempty"`
	ThroughputMode             string                `yaml:"throughputMode,omitempty"`
	ProvisionedThroughputMibps *float64              `yaml:"provisionedThroughputMibps,omitempty"`
	BackupPolicy               *string               `yaml:"backupPolicy,omitempty"`
	LifecyclePolicies          *EFSLifecyclePolicies `yaml:"lifecyclePolicies,omitempty"`
	MountTargets               []*EFSMountTarget     `yaml:"mountTargets,omitempty"`
	AccessPoints               []*EFSAccessPoint     `yaml:"accessPoints,omitempty"`
	Tags                       map[string]string     `yaml:"tags,omitempty"`
	GlobalTags                 map[string]string     `yaml:"-"`
	DependsOn                  []Key                 `yaml:"dependsOn,omitempty"`
}

// EFSLifecyclePolicies describes the lifecycle management transitions for a file system.
// Each transition maps to a single EFS LifecyclePolicy object.
type EFSLifecyclePolicies struct {
	TransitionToIA                  string `yaml:"transitionToIA,omitempty"`
	TransitionToArchive             string `yaml:"transitionToArchive,omitempty"`
	TransitionToPrimaryStorageClass string `yaml:"transitionToPrimaryStorageClass,omitempty"`
}

// Key returns the unique key for the resource for this buildit context.
func (r EFSFileSystem) Key() Key {
	return NewKey(r.Context.ProviderName, r.Identifier())
}

// Identifier returns the file system name.
func (r EFSFileSystem) Identifier() string {
	return r.Name
}

// Normalize sets defaults and sanitizes fields.
func (r *EFSFileSystem) Normalize(ctx context.Context) {
	if r.Encrypted == nil {
		// Secure by default: EFS encryption is immutable, so default it on at create time.
		r.Encrypted = aws.Bool(true)
	}

	if r.PerformanceMode == "" {
		r.PerformanceMode = string(types.PerformanceModeGeneralPurpose)
	} else {
		switch strings.ToLower(r.PerformanceMode) {
		case "generalpurpose":
			r.PerformanceMode = string(types.PerformanceModeGeneralPurpose)
		case "maxio":
			r.PerformanceMode = string(types.PerformanceModeMaxIo)
		}
	}

	if r.ThroughputMode == "" {
		r.ThroughputMode = string(types.ThroughputModeBursting)
	} else {
		r.ThroughputMode = strings.ToLower(strings.TrimSpace(r.ThroughputMode))
	}

	if r.BackupPolicy != nil {
		normalized := strings.ToUpper(strings.TrimSpace(*r.BackupPolicy))
		r.BackupPolicy = &normalized
	}

	if r.LifecyclePolicies != nil {
		r.LifecyclePolicies.TransitionToIA = strings.ToUpper(strings.TrimSpace(r.LifecyclePolicies.TransitionToIA))
		r.LifecyclePolicies.TransitionToArchive = strings.ToUpper(strings.TrimSpace(r.LifecyclePolicies.TransitionToArchive))
		r.LifecyclePolicies.TransitionToPrimaryStorageClass = strings.ToUpper(strings.TrimSpace(r.LifecyclePolicies.TransitionToPrimaryStorageClass))
	}

	if r.Tags == nil {
		r.Tags = make(map[string]string)
	}
	ResourceTags(r.Tags).Merge(r.GlobalTags)
	// The Name tag is not synthesized: the file system is identified by its CreationToken (the
	// buildit resource name), so the Name tag carries no identity meaning and is applied only when
	// the user sets one under `tags`.

	for _, mt := range r.MountTargets {
		mt.Normalize(ctx)
	}
	for _, ap := range r.AccessPoints {
		ap.Normalize(ctx, InheritedTags(r.GlobalTags))
	}
}

// Validate checks the file system input.
func (r EFSFileSystem) Validate(ctx context.Context) error {
	var msgs []string

	// required: resource name
	if r.Identifier() == "" {
		msgs = append(msgs, "efs filesystem name cannot be empty")
	}

	// enum: performance mode
	if !validEnumString(r.PerformanceMode, types.PerformanceMode("").Values()) {
		msgs = append(msgs, fmt.Sprintf("performanceMode must be one of %v", types.PerformanceMode("").Values()))
	}

	// enum + conditional-required: throughput mode and its provisioned coupling
	if !validEnumString(r.ThroughputMode, types.ThroughputMode("").Values()) {
		msgs = append(msgs, fmt.Sprintf("throughputMode must be one of %v", types.ThroughputMode("").Values()))
	} else if r.ThroughputMode == string(types.ThroughputModeProvisioned) {
		if r.ProvisionedThroughputMibps == nil || *r.ProvisionedThroughputMibps <= 0 {
			msgs = append(msgs, "provisionedThroughputMibps is required and must be > 0 when throughputMode is provisioned")
		}
	} else if r.ProvisionedThroughputMibps != nil {
		msgs = append(msgs, "provisionedThroughputMibps is only valid when throughputMode is provisioned")
	}

	// field dependency: kmsKeyId requires encryption
	if r.KmsKeyId != nil && (r.Encrypted == nil || !*r.Encrypted) {
		msgs = append(msgs, "kmsKeyId is only valid when encrypted is true")
	}

	// enum: backup policy
	if r.BackupPolicy != nil && *r.BackupPolicy != string(types.StatusEnabled) && *r.BackupPolicy != string(types.StatusDisabled) {
		msgs = append(msgs, "backupPolicy must be ENABLED or DISABLED")
	}

	if r.LifecyclePolicies != nil {
		lp := r.LifecyclePolicies
		// enum: lifecycle IA transition
		if lp.TransitionToIA != "" && !validEnumString(lp.TransitionToIA, types.TransitionToIARules("").Values()) {
			msgs = append(msgs, fmt.Sprintf("lifecyclePolicies.transitionToIA must be one of %v", types.TransitionToIARules("").Values()))
		}
		// enum: lifecycle Archive transition
		if lp.TransitionToArchive != "" && !validEnumString(lp.TransitionToArchive, types.TransitionToArchiveRules("").Values()) {
			msgs = append(msgs, fmt.Sprintf("lifecyclePolicies.transitionToArchive must be one of %v", types.TransitionToArchiveRules("").Values()))
		}
		// field dependency: Archive storage class requires Elastic throughput + General Purpose performance mode
		if lp.TransitionToArchive != "" {
			if r.ThroughputMode != string(types.ThroughputModeElastic) {
				msgs = append(msgs, "lifecyclePolicies.transitionToArchive requires throughputMode: elastic")
			}
			if r.PerformanceMode != string(types.PerformanceModeGeneralPurpose) {
				msgs = append(msgs, "lifecyclePolicies.transitionToArchive requires performanceMode: generalPurpose")
			}
		}
		// enum: lifecycle primary-storage-class transition
		if lp.TransitionToPrimaryStorageClass != "" && !validEnumString(lp.TransitionToPrimaryStorageClass, types.TransitionToPrimaryStorageClassRules("").Values()) {
			msgs = append(msgs, fmt.Sprintf("lifecyclePolicies.transitionToPrimaryStorageClass must be one of %v", types.TransitionToPrimaryStorageClassRules("").Values()))
		}
	}

	// nested mount targets: validate each and reject duplicate subnets (one mount target per AZ)
	seenSubnets := make(map[string]bool, len(r.MountTargets))
	for _, mt := range r.MountTargets {
		msgs = append(msgs, mt.validate()...)
		if mt.SubnetName != "" {
			if seenSubnets[mt.SubnetName] {
				msgs = append(msgs, fmt.Sprintf("mountTargets: duplicate subnetName %q", mt.SubnetName))
			}
			seenSubnets[mt.SubnetName] = true
		}
	}

	// nested access points: validate each and reject duplicate names (name is the ClientToken / match key)
	seenAPNames := make(map[string]bool, len(r.AccessPoints))
	for _, ap := range r.AccessPoints {
		msgs = append(msgs, ap.validate()...)
		if ap.Name != "" {
			if seenAPNames[ap.Name] {
				msgs = append(msgs, fmt.Sprintf("accessPoints: duplicate name %q", ap.Name))
			}
			seenAPNames[ap.Name] = true
		}
	}

	if msgs == nil {
		return nil
	}
	return &ValidationError{
		ResourceIdentifier: r.Identifier(),
		ResourceType:       "EFSFileSystem",
		Messages:           msgs,
	}
}

// Apply creates or updates the file system, then reconciles its nested mount targets and access
// points against it.
func (r EFSFileSystem) Apply(ctx context.Context) error {
	log.Debugf("applying efs filesystem %v", r.Identifier())

	efsClient := awsw.NewEFS(ctx, r.Context.ProviderName)

	existing, err := r.fetchExisting(ctx)
	if err != nil {
		return errors.Wrapf(err, "error fetching aws resource for %v", r.Identifier())
	}
	if existing == nil {
		// apply creates the file system and its children once it is available.
		return r.apply(ctx)
	}

	// The file system exists: compute one diff covering the file system and its nested children,
	// then apply it. applyDiffs creates/updates/prunes mount targets and access points from the
	// diff, so there is no separate reconcile step.
	diffs, err := r.compareExisting(ctx, efsClient, existing)
	if err != nil {
		return err
	}
	if diffs == nil {
		log.WithFields(log.Fields{"Name": r.Identifier()}).Info("no updates required")
		return nil
	}

	d, ok := diffs.(*EFSFileSystemDiff)
	if !ok {
		return errors.New("invalid diff type supplied")
	}
	for _, msg := range diffs.Differences() {
		log.Debug(msg)
	}
	if d.hasMutableChanges() {
		log.WithField("Name", r.Identifier()).Info("efs filesystem already exists, updating")
	}
	if err := r.applyDiffs(ctx, diffs); err != nil {
		return errors.Wrapf(err, "failed to update efs filesystem %v", r.Identifier())
	}
	return nil
}

func (r EFSFileSystem) apply(ctx context.Context) error {
	efsClient := awsw.NewEFS(ctx, r.Context.ProviderName)

	in := &efs.CreateFileSystemInput{
		CreationToken:   aws.String(r.Name),
		Encrypted:       r.Encrypted,
		PerformanceMode: types.PerformanceMode(r.PerformanceMode),
		ThroughputMode:  types.ThroughputMode(r.ThroughputMode),
		Tags:            awsw.EFSTagSlice(r.Tags),
	}
	if r.KmsKeyId != nil {
		in.KmsKeyId = r.KmsKeyId
	}
	if r.ThroughputMode == string(types.ThroughputModeProvisioned) {
		in.ProvisionedThroughputInMibps = r.ProvisionedThroughputMibps
	}

	out, err := efsClient.CreateFileSystem(ctx, in)
	if err != nil {
		return errors.Wrapf(err, "error creating efs filesystem %v", r.Identifier())
	}
	fsID := aws.ToString(out.FileSystemId)

	if err := efsClient.WaitForFileSystemAvailable(ctx, fsID); err != nil {
		return errors.Wrapf(err, "error waiting for efs filesystem %v", r.Identifier())
	}

	if r.BackupPolicy != nil {
		if err := r.putBackupPolicy(ctx, efsClient, fsID); err != nil {
			return err
		}
	}

	if r.LifecyclePolicies != nil {
		if err := r.putLifecyclePolicies(ctx, efsClient, fsID); err != nil {
			return err
		}
	}

	log.WithFields(log.Fields{
		"Name":         r.Identifier(),
		"FileSystemId": fsID,
	}).Infof("%s", color.Green("efs filesystem created"))

	return r.createChildren(ctx, efsClient, fsID)
}

// createChildren provisions all desired mount targets and access points for a freshly created file
// system. There is nothing to update or prune, so each child is created directly.
func (r EFSFileSystem) createChildren(ctx context.Context, efsClient awsw.EFS, fsID string) error {
	ec2Client := awsw.NewEC2(ctx, r.Context.ProviderName)
	for _, mt := range r.MountTargets {
		if err := mt.create(ctx, efsClient, ec2Client, fsID); err != nil {
			return errors.Wrapf(err, "error creating mount target for efs filesystem %v", r.Identifier())
		}
	}
	for _, ap := range r.AccessPoints {
		if err := ap.create(ctx, efsClient, fsID); err != nil {
			return errors.Wrapf(err, "error creating access point %q for efs filesystem %v", ap.Name, r.Identifier())
		}
	}
	return nil
}

// applyMountTargetDiffs deletes, updates, then adds mount targets per the classified diff sets.
func (r EFSFileSystem) applyMountTargetDiffs(ctx context.Context, efsClient awsw.EFS, fsID string, d *EFSFileSystemDiff) error {
	if !d.mountTargetDiffs {
		return nil
	}
	for _, mt := range d.mountTargetsToDelete {
		if err := deleteMountTarget(ctx, efsClient, mt); err != nil {
			return errors.Wrapf(err, "error pruning mount target for efs filesystem %v", r.Identifier())
		}
	}
	for mt, mtDiff := range d.mountTargetsToUpdate {
		if err := mt.applyDiff(ctx, efsClient, mtDiff); err != nil {
			return errors.Wrapf(err, "error updating mount target for efs filesystem %v", r.Identifier())
		}
	}
	ec2Client := awsw.NewEC2(ctx, r.Context.ProviderName)
	for _, mt := range d.mountTargetsToAdd {
		if err := mt.create(ctx, efsClient, ec2Client, fsID); err != nil {
			return errors.Wrapf(err, "error adding mount target for efs filesystem %v", r.Identifier())
		}
	}
	return nil
}

// applyAccessPointDiffs deletes, updates, then adds access points per the classified diff sets.
func (r EFSFileSystem) applyAccessPointDiffs(ctx context.Context, efsClient awsw.EFS, fsID string, d *EFSFileSystemDiff) error {
	if !d.accessPointDiffs {
		return nil
	}
	for _, ap := range d.accessPointsToDelete {
		if err := deleteAccessPoint(ctx, efsClient, fsID, ap); err != nil {
			return errors.Wrapf(err, "error pruning access point for efs filesystem %v", r.Identifier())
		}
	}
	for ap, apDiff := range d.accessPointsToUpdate {
		if err := ap.applyDiff(ctx, efsClient, apDiff); err != nil {
			return errors.Wrapf(err, "error updating access point %q for efs filesystem %v", ap.Name, r.Identifier())
		}
	}
	for _, ap := range d.accessPointsToAdd {
		if err := ap.create(ctx, efsClient, fsID); err != nil {
			return errors.Wrapf(err, "error adding access point %q for efs filesystem %v", ap.Name, r.Identifier())
		}
	}
	return nil
}

func (r EFSFileSystem) applyDiffs(ctx context.Context, diffs ResourceDiff) error {
	if diffs == nil {
		log.WithField("Name", r.Identifier()).Info("no updates required for efs filesystem")
		return nil
	}

	d, ok := diffs.(*EFSFileSystemDiff)
	if !ok {
		return errors.New("invalid diff type supplied")
	}

	existing, ok := d.Resource.(*types.FileSystemDescription)
	if !ok {
		return errors.Errorf("cannot retrieve existing efs filesystem")
	}
	fsID := aws.ToString(existing.FileSystemId)
	efsClient := awsw.NewEFS(ctx, r.Context.ProviderName)

	if d.encryptedDiff {
		log.WithField("Name", r.Identifier()).Warn("efs filesystem encrypted is immutable; destroy & recreate to change")
	}
	if d.kmsKeyDiff {
		log.WithField("Name", r.Identifier()).Warn("efs filesystem kmsKeyId is immutable; destroy & recreate to change")
	}
	if d.performanceDiff {
		log.WithField("Name", r.Identifier()).Warn("efs filesystem performanceMode is immutable; destroy & recreate to change")
	}

	if d.throughputDiff {
		updateIn := &efs.UpdateFileSystemInput{
			FileSystemId:   existing.FileSystemId,
			ThroughputMode: types.ThroughputMode(r.ThroughputMode),
		}
		if r.ThroughputMode == string(types.ThroughputModeProvisioned) {
			updateIn.ProvisionedThroughputInMibps = r.ProvisionedThroughputMibps
		}
		if _, err := efsClient.UpdateFileSystem(ctx, updateIn); err != nil {
			return errors.Wrapf(err, "error updating throughput for efs filesystem %v", r.Identifier())
		}
		// A throughput change moves the file system to updating; let it settle before
		// any further mutations (backup/lifecycle/access points).
		if err := efsClient.WaitForFileSystemAvailable(ctx, fsID); err != nil {
			return errors.Wrapf(err, "error waiting for efs filesystem %v", r.Identifier())
		}
	}

	if d.backupDiff {
		if err := r.putBackupPolicy(ctx, efsClient, fsID); err != nil {
			return err
		}
	}

	if d.lifecycleDiff {
		if err := r.putLifecyclePolicies(ctx, efsClient, fsID); err != nil {
			return err
		}
	}

	if d.tagsDiff {
		if err := efsClient.AddResourceTags(ctx, fsID, d.tagDiff.Upserts()); err != nil {
			return errors.Wrapf(err, "error updating tags for efs filesystem %v", r.Identifier())
		}
		if err := efsClient.DeleteResourceTags(ctx, fsID, d.tagDiff.DeletedKeys()); err != nil {
			return errors.Wrapf(err, "error removing tags for efs filesystem %v", r.Identifier())
		}
	}

	// Children: delete, then update, then add (the same order the load balancer applies its
	// listeners). The diff already holds the resolved state, so no re-fetch is needed here.
	if err := r.applyMountTargetDiffs(ctx, efsClient, fsID, d); err != nil {
		return err
	}
	if err := r.applyAccessPointDiffs(ctx, efsClient, fsID, d); err != nil {
		return err
	}

	// Only immutable diffs (encrypted/kmsKeyId/performanceMode) were present: warnings were
	// logged above and no AWS mutation happened, so don't claim the file system was updated.
	if !d.hasMutableChanges() {
		return nil
	}

	log.WithFields(log.Fields{
		"Name":         r.Identifier(),
		"FileSystemId": fsID,
	}).Infof("%s", color.Yellow("efs filesystem updated"))

	return nil
}

// Destroy deletes the file system along with its mount targets and access points.
func (r EFSFileSystem) Destroy(ctx context.Context) error {
	log.Debugf("destroying efs filesystem %v", r.Identifier())

	existing, err := r.fetchExisting(ctx)
	if err != nil {
		return errors.Wrapf(err, "error finding efs filesystem %v", r.Identifier())
	}
	if existing == nil {
		log.WithField("Name", r.Identifier()).Info("efs filesystem does not exist, nothing to destroy, skipping")
		return nil
	}

	fsID := aws.ToString(existing.FileSystemId)
	efsClient := awsw.NewEFS(ctx, r.Context.ProviderName)

	// A file system cannot be deleted while it still has mount targets or access points. Both sets
	// are torn down best-effort: every child is issued a delete before we return, so one transient
	// failure doesn't strand its siblings until the next reconciliation. Errors are aggregated and
	// returned so the run still fails (and retries) — but with all deletes attempted.
	mountTargets, err := efsClient.MountTargetsForFileSystem(ctx, fsID)
	if err != nil {
		return errors.Wrapf(err, "error listing mount targets for efs filesystem %v", r.Identifier())
	}
	var mtErrs []error
	for _, mt := range mountTargets {
		if err := deleteMountTarget(ctx, efsClient, mt); err != nil {
			mtErrs = append(mtErrs, err)
		}
	}
	if len(mtErrs) > 0 {
		return errors.Wrapf(stderrors.Join(mtErrs...), "error deleting mount targets for efs filesystem %v", r.Identifier())
	}

	accessPoints, err := efsClient.AccessPointsForFileSystem(ctx, fsID)
	if err != nil {
		return errors.Wrapf(err, "error listing access points for efs filesystem %v", r.Identifier())
	}
	// Access points are deleted in bulk and then waited on once (waitForAccessPointsGone), rather
	// than via the per-access-point deleteAccessPoint helper used by the prune path: issuing every
	// delete up front and polling the set once is faster during teardown than serializing
	// delete+poll for each access point.
	var apErrs []error
	for _, ap := range accessPoints {
		if _, err := efsClient.DeleteAccessPoint(ctx, &efs.DeleteAccessPointInput{
			AccessPointId: ap.AccessPointId,
		}); err != nil {
			apErrs = append(apErrs, errors.Wrapf(err, "access point %v", aws.ToString(ap.Name)))
		}
	}
	if len(apErrs) > 0 {
		return errors.Wrapf(stderrors.Join(apErrs...), "error deleting access points for efs filesystem %v", r.Identifier())
	}
	if len(accessPoints) > 0 {
		if err := r.waitForAccessPointsGone(ctx, efsClient, fsID); err != nil {
			return err
		}
	}

	if _, err := efsClient.DeleteFileSystem(ctx, &efs.DeleteFileSystemInput{
		FileSystemId: existing.FileSystemId,
	}); err != nil {
		return errors.Wrapf(err, "error deleting efs filesystem %v", r.Identifier())
	}

	if err := efsClient.WaitForFileSystemDeleted(ctx, fsID); err != nil {
		return errors.Wrapf(err, "error waiting for efs filesystem %v deletion", r.Identifier())
	}

	log.WithFields(log.Fields{
		"Name":         r.Identifier(),
		"FileSystemId": fsID,
	}).Infof("%s", color.Red("efs filesystem destroyed"))

	return nil
}

func (r EFSFileSystem) waitForAccessPointsGone(ctx context.Context, efsClient awsw.EFS, fsID string) error {
	return efsPollUntilDeleted(ctx, fmt.Sprintf("access points of efs filesystem %v", r.Identifier()), func(ctx context.Context) (bool, error) {
		aps, err := efsClient.AccessPointsForFileSystem(ctx, fsID)
		if err != nil {
			return false, errors.Wrapf(err, "error listing access points for efs filesystem %v", r.Identifier())
		}
		return len(aps) == 0, nil
	})
}

// EFSFileSystemDiff captures the differences between desired and actual file system state.
type EFSFileSystemDiff struct {
	BaseResourceDiff

	encryptedDiff   bool
	kmsKeyDiff      bool
	performanceDiff bool
	throughputDiff  bool
	backupDiff      bool
	lifecycleDiff   bool
	tagsDiff        bool
	tagDiff         util.TagDiffResult

	// Nested children, classified by compareMountTargets / compareAccessPoints and consumed by
	// applyDiffs. Mount targets are matched by subnet, access points by name.
	//
	// *Diffs is set whenever there is any child entry to process in applyDiffs (including an
	// access point whose only drift is its immutable posixUser/rootDirectory, which applyDiffs
	// merely warns about). accessPointMutations is the narrower signal — set only when an access
	// point change actually mutates AWS (an add, a delete, or a tag update) — so a warn-only diff
	// is not mistaken for an update. Mount target updates always change security groups, so
	// mountTargetDiffs already implies a real mutation.
	mountTargetDiffs     bool
	mountTargetsToAdd    []*EFSMountTarget
	mountTargetsToUpdate map[*EFSMountTarget]*EFSMountTargetDiff
	mountTargetsToDelete []types.MountTargetDescription

	accessPointDiffs     bool
	accessPointMutations bool
	accessPointsToAdd    []*EFSAccessPoint
	accessPointsToUpdate map[*EFSAccessPoint]*EFSAccessPointDiff
	accessPointsToDelete []types.AccessPointDescription
}

// hasMutableChanges reports whether the diff contains changes that actually mutate the existing
// file system. Encryption, KMS key, and performance mode are immutable (warn-only), as is an access
// point's posixUser/rootDirectory — none of those count here, so a run with only immutable drift is
// not reported as an update.
func (d *EFSFileSystemDiff) hasMutableChanges() bool {
	return d.throughputDiff || d.backupDiff || d.lifecycleDiff || d.tagsDiff ||
		d.mountTargetDiffs || d.accessPointMutations
}

// Compare fetches the existing file system and computes a diff against the desired state.
func (r EFSFileSystem) Compare(ctx context.Context) (ResourceDiff, error) {
	existing, err := r.fetchExisting(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "error fetching aws resource for %v", r.Identifier())
	}

	if existing == nil {
		diffs := &EFSFileSystemDiff{}
		diffs.Messages = append(diffs.Messages, "efs filesystem does not exist")
		return diffs, nil
	}

	efsClient := awsw.NewEFS(ctx, r.Context.ProviderName)
	return r.compareExisting(ctx, efsClient, existing)
}

// compareExisting computes a diff against an already-fetched live file system. It returns nil when
// there is no difference. The KMS key is resolved to its canonical ARN so that an alias, key id,
// or ARN supplied in config all compare equal to the ARN AWS reports.
func (r EFSFileSystem) compareExisting(ctx context.Context, efsClient awsw.EFS, existing *types.FileSystemDescription) (ResourceDiff, error) {
	fsID := aws.ToString(existing.FileSystemId)

	awsTags, err := efsClient.ResourceTags(ctx, fsID)
	if err != nil {
		return nil, err
	}

	// backupPolicy and lifecyclePolicies follow a Terraform-style ownership model: the config is
	// the full desired state, so live state is always fetched even when the field is omitted (an
	// omitted field reverts live to the AWS default rather than leaving it unmanaged).
	backupStatus, err := efsClient.BackupPolicyStatus(ctx, fsID)
	if err != nil {
		return nil, err
	}

	lifecycle, err := efsClient.LifecyclePolicies(ctx, fsID)
	if err != nil {
		return nil, err
	}

	desiredKmsArn, err := r.resolveKmsArn(ctx)
	if err != nil {
		return nil, err
	}

	diffs := r.computeDiff(ctx, existing, awsTags, backupStatus, lifecycle, desiredKmsArn)
	if diffs == nil {
		// The file system attributes match; start an empty diff so child drift can still be recorded.
		diffs = &EFSFileSystemDiff{}
		diffs.Resource = existing
	}

	// Mount targets and access points are nested children. Classify their drift into add/update/delete
	// sets on the diff so compare/plan reports them and applyDiffs can act on them, mirroring how the
	// load balancer resource handles its listeners.
	if err := r.compareMountTargets(ctx, efsClient, fsID, diffs); err != nil {
		return nil, err
	}
	if err := r.compareAccessPoints(ctx, efsClient, fsID, diffs); err != nil {
		return nil, err
	}

	// Any change — file system attribute or child — appends at least one message, so an empty
	// message list means there is no difference at all.
	if len(diffs.Messages) == 0 {
		return nil, nil
	}
	return diffs, nil
}

// compareMountTargets classifies the desired mount targets against the live set (matched by subnet)
// into add / update / delete, recording the result on diffs. The subnet and IP address are
// immutable, so an existing mount target can only differ in its security groups.
func (r EFSFileSystem) compareMountTargets(ctx context.Context, efsClient awsw.EFS, fsID string, diffs *EFSFileSystemDiff) error {
	ec2Client := awsw.NewEC2(ctx, r.Context.ProviderName)

	existing, err := efsClient.MountTargetsForFileSystem(ctx, fsID)
	if err != nil {
		return errors.Wrapf(err, "error listing mount targets for efs filesystem %v", r.Identifier())
	}
	existingBySubnet := make(map[string]types.MountTargetDescription, len(existing))
	for _, mt := range existing {
		existingBySubnet[aws.ToString(mt.SubnetId)] = mt
	}

	var add []*EFSMountTarget
	var addSubnets, updSubnets []string
	upd := make(map[*EFSMountTarget]*EFSMountTargetDiff)
	desiredSubnetIDs := make(map[string]bool, len(r.MountTargets))
	for _, mt := range r.MountTargets {
		subnetID, vpcID, err := mt.resolveSubnet(ctx, ec2Client)
		if err != nil {
			return err
		}
		desiredSubnetIDs[subnetID] = true

		cur, ok := existingBySubnet[subnetID]
		if !ok {
			add = append(add, mt)
			addSubnets = append(addSubnets, mt.SubnetName)
			continue
		}
		diff, err := mt.equals(ctx, ec2Client, efsClient, vpcID, &cur)
		if err != nil {
			return err
		}
		if diff != nil {
			mtDiff, ok := diff.(*EFSMountTargetDiff)
			if !ok {
				return errors.Errorf("unexpected mount target diff type %T", diff)
			}
			upd[mt] = mtDiff
			updSubnets = append(updSubnets, mt.SubnetName)
			diffs.Messages = append(diffs.Messages, formatMountTargetDiffMessages(mt.SubnetName, mtDiff.Differences())...)
		}
	}

	var del []types.MountTargetDescription
	var delSubnets []string
	for _, mt := range existing {
		if !desiredSubnetIDs[aws.ToString(mt.SubnetId)] {
			del = append(del, mt)
			delSubnets = append(delSubnets, aws.ToString(mt.SubnetId))
		}
	}

	if len(add) == 0 && len(upd) == 0 && len(del) == 0 {
		return nil
	}
	diffs.mountTargetDiffs = true
	diffs.mountTargetsToAdd = add
	diffs.mountTargetsToUpdate = upd
	diffs.mountTargetsToDelete = del
	if len(add) > 0 {
		slices.Sort(addSubnets)
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("%d mount target(s) to be added to efs filesystem %v: %v", len(add), r.Identifier(), strings.Join(addSubnets, ", ")))
	}
	if len(upd) > 0 {
		slices.Sort(updSubnets)
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("%d mount target(s) to be updated on efs filesystem %v: %v", len(upd), r.Identifier(), strings.Join(updSubnets, ", ")))
	}
	if len(del) > 0 {
		slices.Sort(delSubnets)
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("%d mount target(s) to be removed from efs filesystem %v: %v", len(del), r.Identifier(), strings.Join(delSubnets, ", ")))
	}
	return nil
}

// compareAccessPoints classifies the desired access points against the live set into
// add / update / delete, recording the result on diffs. An access point is identified by its
// ClientToken (its `name`); see classifyAccessPoints.
//
// posixUser/rootDirectory are immutable: a matched access point that differs only there is recorded
// (and warned about at apply) but does not count as a mutation, so the run is not reported as an
// update. The "updated" summary therefore lists only access points with a real (tag) change.
func (r EFSFileSystem) compareAccessPoints(ctx context.Context, efsClient awsw.EFS, fsID string, diffs *EFSFileSystemDiff) error {
	existing, err := efsClient.AccessPointsForFileSystem(ctx, fsID)
	if err != nil {
		return errors.Wrapf(err, "error listing access points for efs filesystem %v", r.Identifier())
	}
	add, matched, del := r.classifyAccessPoints(existing)

	var addNames, updNames, delNames []string
	upd := make(map[*EFSAccessPoint]*EFSAccessPointDiff)
	for _, ap := range add {
		addNames = append(addNames, ap.Name)
	}
	for _, m := range matched {
		cur := m.existing
		diff, err := m.desired.equals(ctx, &cur)
		if err != nil {
			return err
		}
		if diff == nil {
			continue
		}
		apDiff, ok := diff.(*EFSAccessPointDiff)
		if !ok {
			return errors.Errorf("unexpected access point diff type %T", diff)
		}
		upd[m.desired] = apDiff
		diffs.Messages = append(diffs.Messages, formatAccessPointDiffMessages(m.desired.Name, apDiff.Differences())...)
		// Only a tag change actually mutates the access point; immutable posixUser/rootDirectory
		// drift is warn-only and must not be counted as an update.
		if apDiff.tagsDiff {
			updNames = append(updNames, m.desired.Name)
		}
	}
	for _, ap := range del {
		name := aws.ToString(ap.Name)
		if name == "" {
			name = aws.ToString(ap.AccessPointId)
		}
		delNames = append(delNames, name)
	}

	if len(add) == 0 && len(upd) == 0 && len(del) == 0 {
		return nil
	}
	diffs.accessPointDiffs = true
	diffs.accessPointMutations = len(add) > 0 || len(del) > 0 || len(updNames) > 0
	diffs.accessPointsToAdd = add
	diffs.accessPointsToUpdate = upd
	diffs.accessPointsToDelete = del
	if len(add) > 0 {
		slices.Sort(addNames)
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("%d access point(s) to be added to efs filesystem %v: %v", len(add), r.Identifier(), strings.Join(addNames, ", ")))
	}
	if len(updNames) > 0 {
		slices.Sort(updNames)
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("%d access point(s) to be updated on efs filesystem %v: %v", len(updNames), r.Identifier(), strings.Join(updNames, ", ")))
	}
	if len(del) > 0 {
		slices.Sort(delNames)
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("%d access point(s) to be removed from efs filesystem %v: %v", len(del), r.Identifier(), strings.Join(delNames, ", ")))
	}
	return nil
}

// apMatch pairs a desired access point with the live access point it was matched to.
type apMatch struct {
	desired  *EFSAccessPoint
	existing types.AccessPointDescription
}

// classifyAccessPoints partitions the desired access points against the live set into adds, matched
// pairs, and deletes. It is pure (no AWS calls) so the matching can be unit tested.
//
// An access point is identified by its ClientToken (its `name`, the EFS creation token), mirroring
// how the file system is matched by its CreationToken. A live access point matches only when its
// ClientToken equals the desired `name`; the Name tag is never used for matching. Any unclaimed live
// access point is a delete. To adopt one created outside buildit, set `name` to its ClientToken.
func (r EFSFileSystem) classifyAccessPoints(existing []types.AccessPointDescription) (add []*EFSAccessPoint, matched []apMatch, del []types.AccessPointDescription) {
	existingByToken := make(map[string]*types.AccessPointDescription, len(existing))
	for i := range existing {
		existingByToken[aws.ToString(existing[i].ClientToken)] = &existing[i]
	}

	claimed := make(map[string]bool, len(existing)) // ClientTokens of live APs claimed by a desired one
	for _, ap := range r.AccessPoints {
		cur, ok := existingByToken[ap.clientToken()]
		if !ok {
			add = append(add, ap)
			continue
		}
		claimed[aws.ToString(cur.ClientToken)] = true
		matched = append(matched, apMatch{desired: ap, existing: *cur})
	}

	for _, ap := range existing {
		if !claimed[aws.ToString(ap.ClientToken)] {
			del = append(del, ap)
		}
	}
	return add, matched, del
}

// formatMountTargetDiffMessages prefixes each mount target child diff message with its subnet,
// mirroring formatListenerDiffMessages in the load balancer resource.
func formatMountTargetDiffMessages(subnetName string, messages []string) []string {
	formatted := make([]string, 0, len(messages))
	for _, msg := range messages {
		formatted = append(formatted, fmt.Sprintf("mount target %v: %v", subnetName, msg))
	}
	return formatted
}

// formatAccessPointDiffMessages prefixes each access point child diff message with its name.
func formatAccessPointDiffMessages(name string, messages []string) []string {
	formatted := make([]string, 0, len(messages))
	for _, msg := range messages {
		formatted = append(formatted, fmt.Sprintf("access point %v: %v", name, msg))
	}
	return formatted
}

// resolveKmsArn resolves the configured KMS key (alias, key id, or ARN) to its canonical key ARN,
// or "" when no key is configured.
func (r EFSFileSystem) resolveKmsArn(ctx context.Context) (string, error) {
	if r.KmsKeyId == nil {
		return "", nil
	}
	kmsClient := awsw.NewKMS(ctx, r.Context.ProviderName)
	arn, err := kmsClient.ResolveKeyArn(ctx, aws.ToString(r.KmsKeyId))
	if err != nil {
		return "", errors.Wrapf(err, "error resolving kmsKeyId for efs filesystem %v", r.Identifier())
	}
	return arn, nil
}

// computeDiff is the pure diff logic, independent of AWS calls so it can be unit tested.
// desiredKmsArn is the configured KMS key resolved to its canonical ARN (or "" when unset).
func (r EFSFileSystem) computeDiff(
	ctx context.Context,
	existing *types.FileSystemDescription,
	awsTags map[string]string,
	backupStatus types.Status,
	lifecycle []types.LifecyclePolicy,
	desiredKmsArn string,
) *EFSFileSystemDiff {
	diffs := &EFSFileSystemDiff{}

	if existing == nil {
		diffs.Messages = append(diffs.Messages, "efs filesystem does not exist")
		return diffs
	}
	diffs.Resource = existing

	diff := false

	if aws.ToBool(r.Encrypted) != aws.ToBool(existing.Encrypted) {
		diff = true
		diffs.encryptedDiff = true
		diffs.Messages = append(diffs.Messages, "encrypted is different (immutable; destroy & recreate to change)")
	}

	// existing.KmsKeyId is the canonical key ARN; desiredKmsArn is the configured key resolved to
	// the same form, so an alias/key-id/ARN supplied in config does not show as a spurious change.
	if r.KmsKeyId != nil && desiredKmsArn != aws.ToString(existing.KmsKeyId) {
		diff = true
		diffs.kmsKeyDiff = true
		diffs.Messages = append(diffs.Messages, "kmsKeyId is different (immutable; destroy & recreate to change)")
	}

	if r.PerformanceMode != string(existing.PerformanceMode) {
		diff = true
		diffs.performanceDiff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("performanceMode is different: %s -> %s (immutable; destroy & recreate to change)", existing.PerformanceMode, r.PerformanceMode))
	}

	if r.ThroughputMode != string(existing.ThroughputMode) ||
		(r.ThroughputMode == string(types.ThroughputModeProvisioned) &&
			aws.ToFloat64(r.ProvisionedThroughputMibps) != aws.ToFloat64(existing.ProvisionedThroughputInMibps)) {
		diff = true
		diffs.throughputDiff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("throughput will be updated: %s -> %s", existing.ThroughputMode, r.ThroughputMode))
	}

	// An omitted backupPolicy reverts to the AWS CreateFileSystem default of DISABLED.
	desiredBackup := string(types.StatusDisabled)
	if r.BackupPolicy != nil {
		desiredBackup = *r.BackupPolicy
	}
	if desiredBackup != string(normalizeBackupStatus(backupStatus)) {
		diff = true
		diffs.backupDiff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("backupPolicy will be updated: %s -> %s", normalizeBackupStatus(backupStatus), desiredBackup))
	}

	// An omitted lifecyclePolicies reverts to the AWS default of no policies. lifecycleView()
	// returns the zero value for a nil pointer, so this diffs against any non-empty live config.
	if r.lifecycleView() != lifecycleViewFromPolicies(lifecycle) {
		diff = true
		diffs.lifecycleDiff = true
		diffs.Messages = append(diffs.Messages, "lifecyclePolicies will be updated")
	}

	// AWS-reserved tags (e.g. aws:elasticfilesystem:default-backup) are managed by AWS and
	// cannot be modified, so they must never participate in the diff.
	awsTags = filterReservedTags(awsTags)
	if tagDiff := TagDiffForContext(ctx, awsTags, r.Tags); tagDiff.HasChanges() {
		diff = true
		diffs.tagsDiff = true
		diffs.tagDiff = tagDiff
		diffs.Messages = append(diffs.Messages, TagDiffSummary(awsTags, diffs.tagDiff)...)
	}

	if !diff {
		return nil
	}
	return diffs
}

// fetchExisting returns the file system whose CreationToken matches this resource's name, or nil.
// CreationToken is the buildit resource name (set at create time), so this also adopts a
// pre-existing file system whose resource name is set to its CreationToken.
func (r EFSFileSystem) fetchExisting(ctx context.Context) (*types.FileSystemDescription, error) {
	efsClient := awsw.NewEFS(ctx, r.Context.ProviderName)
	return efsClient.FileSystemByCreationToken(ctx, r.Name)
}

func (r EFSFileSystem) putBackupPolicy(ctx context.Context, efsClient awsw.EFS, fsID string) error {
	// EFS rejects mutations unless the file system is available; a prior create/update
	// may have left it transitioning.
	if err := efsClient.WaitForFileSystemAvailable(ctx, fsID); err != nil {
		return errors.Wrapf(err, "error waiting for efs filesystem %v", r.Identifier())
	}
	// An omitted backupPolicy reverts to the AWS default of DISABLED.
	status := types.StatusDisabled
	if r.BackupPolicy != nil {
		status = types.Status(*r.BackupPolicy)
	}
	_, err := efsClient.PutBackupPolicy(ctx, &efs.PutBackupPolicyInput{
		FileSystemId: aws.String(fsID),
		BackupPolicy: &types.BackupPolicy{Status: status},
	})
	if err != nil {
		return errors.Wrapf(err, "error setting backup policy for efs filesystem %v", r.Identifier())
	}
	return nil
}

func (r EFSFileSystem) putLifecyclePolicies(ctx context.Context, efsClient awsw.EFS, fsID string) error {
	if err := efsClient.WaitForFileSystemAvailable(ctx, fsID); err != nil {
		return errors.Wrapf(err, "error waiting for efs filesystem %v", r.Identifier())
	}
	_, err := efsClient.PutLifecycleConfiguration(ctx, &efs.PutLifecycleConfigurationInput{
		FileSystemId:      aws.String(fsID),
		LifecyclePolicies: r.lifecyclePoliciesToAWS(),
	})
	if err != nil {
		return errors.Wrapf(err, "error setting lifecycle policies for efs filesystem %v", r.Identifier())
	}
	return nil
}

// lifecyclePoliciesToAWS converts the desired lifecycle block into the per-transition
// LifecyclePolicy array EFS expects (one object per transition). It always returns a non-nil
// slice: an empty (but non-nil) slice is how PutLifecycleConfiguration clears all policies, and
// the SDK rejects a nil LifecyclePolicies as a missing required field.
func (r EFSFileSystem) lifecyclePoliciesToAWS() []types.LifecyclePolicy {
	policies := []types.LifecyclePolicy{}
	if r.LifecyclePolicies == nil {
		return policies
	}
	if r.LifecyclePolicies.TransitionToIA != "" {
		policies = append(policies, types.LifecyclePolicy{
			TransitionToIA: types.TransitionToIARules(r.LifecyclePolicies.TransitionToIA),
		})
	}
	if r.LifecyclePolicies.TransitionToArchive != "" {
		policies = append(policies, types.LifecyclePolicy{
			TransitionToArchive: types.TransitionToArchiveRules(r.LifecyclePolicies.TransitionToArchive),
		})
	}
	if r.LifecyclePolicies.TransitionToPrimaryStorageClass != "" {
		policies = append(policies, types.LifecyclePolicy{
			TransitionToPrimaryStorageClass: types.TransitionToPrimaryStorageClassRules(r.LifecyclePolicies.TransitionToPrimaryStorageClass),
		})
	}
	return policies
}

// lifecycleView is a comparable projection of the desired/actual lifecycle transitions.
type lifecycleView struct {
	ia      string
	archive string
	primary string
}

func (r EFSFileSystem) lifecycleView() lifecycleView {
	if r.LifecyclePolicies == nil {
		return lifecycleView{}
	}
	return lifecycleView{
		ia:      r.LifecyclePolicies.TransitionToIA,
		archive: r.LifecyclePolicies.TransitionToArchive,
		primary: r.LifecyclePolicies.TransitionToPrimaryStorageClass,
	}
}

func lifecycleViewFromPolicies(policies []types.LifecyclePolicy) lifecycleView {
	var v lifecycleView
	for _, p := range policies {
		if p.TransitionToIA != "" {
			v.ia = string(p.TransitionToIA)
		}
		if p.TransitionToArchive != "" {
			v.archive = string(p.TransitionToArchive)
		}
		if p.TransitionToPrimaryStorageClass != "" {
			v.primary = string(p.TransitionToPrimaryStorageClass)
		}
	}
	return v
}

// normalizeBackupStatus collapses transient ENABLING/DISABLING states to their target state.
func normalizeBackupStatus(s types.Status) types.Status {
	switch s {
	case types.StatusEnabled, types.StatusEnabling:
		return types.StatusEnabled
	default:
		return types.StatusDisabled
	}
}

// filterReservedTags drops AWS-reserved tag keys (the "aws:" prefix), which AWS manages
// automatically and which cannot be added, changed, or removed by callers.
func filterReservedTags(tags map[string]string) map[string]string {
	if tags == nil {
		return nil
	}
	out := make(map[string]string, len(tags))
	for k, v := range tags {
		if strings.HasPrefix(k, "aws:") {
			continue
		}
		out[k] = v
	}
	return out
}

func validEnumString[T ~string](v string, values []T) bool {
	return slices.ContainsFunc(values, func(x T) bool { return string(x) == v })
}
