package resource

import (
	"context"
	"testing"

	"github.com/TouchBistro/buildit/util"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/efs/types"
	"github.com/stretchr/testify/assert"
)

func TestEFSFileSystem_Normalize(t *testing.T) {
	ctx := context.Background()

	t.Run("sets defaults and does not synthesize a Name tag", func(t *testing.T) {
		r := &EFSFileSystem{Name: "shared"}
		r.Normalize(ctx)
		assert.True(t, aws.ToBool(r.Encrypted))
		assert.Equal(t, "generalPurpose", r.PerformanceMode)
		assert.Equal(t, "bursting", r.ThroughputMode)
		// File system is identified by its CreationToken, so the Name tag is not auto-set.
		_, hasName := r.Tags["Name"]
		assert.False(t, hasName, "Name tag must not be auto-set")
	})

	t.Run("canonicalizes enums", func(t *testing.T) {
		r := &EFSFileSystem{
			Name:            "shared",
			PerformanceMode: "MaxIO",
			ThroughputMode:  "Provisioned",
			BackupPolicy:    aws.String("enabled"),
		}
		r.Normalize(ctx)
		assert.Equal(t, "maxIO", r.PerformanceMode)
		assert.Equal(t, "provisioned", r.ThroughputMode)
		assert.Equal(t, "ENABLED", *r.BackupPolicy)
	})

	t.Run("merges global tags and keeps explicit Name tag", func(t *testing.T) {
		r := &EFSFileSystem{
			Name:       "shared",
			Tags:       map[string]string{"Name": "custom", "env": "prod"},
			GlobalTags: map[string]string{"owner": "team-a", "env": "stg"},
		}
		r.Normalize(ctx)
		assert.Equal(t, "custom", r.Tags["Name"]) // explicit wins
		assert.Equal(t, "prod", r.Tags["env"])    // local wins over global
		assert.Equal(t, "team-a", r.Tags["owner"])
	})

	// An access point is not a top-level buildit resource and has no identifier of its
	// own, so it must not inherit the file system's. If it did, a lookup for one
	// resource-id would resolve to the file system plus every access point under it.
	t.Run("access points do not inherit the file system's buildit tags", func(t *testing.T) {
		r := &EFSFileSystem{
			Name: "shared",
			GlobalTags: map[string]string{
				util.BuilditResourceIDTagKey: "shared",
				"owner":                      "team-a",
			},
			AccessPoints: []*EFSAccessPoint{{Name: "ap-1"}},
		}
		r.Normalize(ctx)

		assert.Equal(t, "shared", r.Tags[util.BuilditResourceIDTagKey])

		ap := r.AccessPoints[0]
		assert.NotContains(t, ap.Tags, util.BuilditResourceIDTagKey)
		assert.Equal(t, "team-a", ap.Tags["owner"], "ordinary global tags should still reach the access point")
	})
}

func TestEFSFileSystem_Validate(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name    string
		fs      EFSFileSystem
		wantErr bool
		errText string
	}{
		{
			name: "valid bursting",
			fs:   EFSFileSystem{Name: "shared", PerformanceMode: "generalPurpose", ThroughputMode: "bursting"},
		},
		{
			name:    "empty name",
			fs:      EFSFileSystem{PerformanceMode: "generalPurpose", ThroughputMode: "bursting"},
			wantErr: true,
			errText: "name cannot be empty",
		},
		{
			name:    "bad performance mode",
			fs:      EFSFileSystem{Name: "shared", PerformanceMode: "fast", ThroughputMode: "bursting"},
			wantErr: true,
			errText: "performanceMode must be one of",
		},
		{
			name:    "provisioned without throughput",
			fs:      EFSFileSystem{Name: "shared", PerformanceMode: "generalPurpose", ThroughputMode: "provisioned"},
			wantErr: true,
			errText: "provisionedThroughputMibps is required",
		},
		{
			name:    "throughput set without provisioned mode",
			fs:      EFSFileSystem{Name: "shared", PerformanceMode: "generalPurpose", ThroughputMode: "bursting", ProvisionedThroughputMibps: aws.Float64(10)},
			wantErr: true,
			errText: "only valid when throughputMode is provisioned",
		},
		{
			name:    "kms without encryption",
			fs:      EFSFileSystem{Name: "shared", PerformanceMode: "generalPurpose", ThroughputMode: "bursting", Encrypted: aws.Bool(false), KmsKeyId: aws.String("alias/k")},
			wantErr: true,
			errText: "kmsKeyId is only valid when encrypted is true",
		},
		{
			name:    "bad backup policy",
			fs:      EFSFileSystem{Name: "shared", PerformanceMode: "generalPurpose", ThroughputMode: "bursting", BackupPolicy: aws.String("ON")},
			wantErr: true,
			errText: "backupPolicy must be ENABLED or DISABLED",
		},
		{
			name:    "bad lifecycle transition",
			fs:      EFSFileSystem{Name: "shared", PerformanceMode: "generalPurpose", ThroughputMode: "bursting", LifecyclePolicies: &EFSLifecyclePolicies{TransitionToIA: "AFTER_99_DAYS"}},
			wantErr: true,
			errText: "transitionToIA must be one of",
		},
		{
			name:    "archive without elastic throughput",
			fs:      EFSFileSystem{Name: "shared", PerformanceMode: "generalPurpose", ThroughputMode: "bursting", LifecyclePolicies: &EFSLifecyclePolicies{TransitionToArchive: "AFTER_90_DAYS"}},
			wantErr: true,
			errText: "requires throughputMode: elastic",
		},
		{
			name:    "archive with elastic but maxIO performance",
			fs:      EFSFileSystem{Name: "shared", PerformanceMode: "maxIO", ThroughputMode: "elastic", LifecyclePolicies: &EFSLifecyclePolicies{TransitionToArchive: "AFTER_90_DAYS"}},
			wantErr: true,
			errText: "requires performanceMode: generalPurpose",
		},
		{
			name: "archive with elastic and generalPurpose",
			fs:   EFSFileSystem{Name: "shared", PerformanceMode: "generalPurpose", ThroughputMode: "elastic", LifecyclePolicies: &EFSLifecyclePolicies{TransitionToArchive: "AFTER_90_DAYS"}},
		},
		{
			name: "duplicate mount target subnet",
			fs: EFSFileSystem{Name: "shared", PerformanceMode: "generalPurpose", ThroughputMode: "bursting",
				MountTargets: []*EFSMountTarget{{SubnetName: "a"}, {SubnetName: "a"}}},
			wantErr: true,
			errText: "duplicate subnetName",
		},
		{
			name: "nested mount target missing subnet",
			fs: EFSFileSystem{Name: "shared", PerformanceMode: "generalPurpose", ThroughputMode: "bursting",
				MountTargets: []*EFSMountTarget{{}}},
			wantErr: true,
			errText: "subnetName is required",
		},
		{
			name: "duplicate access point name",
			fs: EFSFileSystem{Name: "shared", PerformanceMode: "generalPurpose", ThroughputMode: "bursting",
				AccessPoints: []*EFSAccessPoint{{Name: "app"}, {Name: "app"}}},
			wantErr: true,
			errText: "duplicate name",
		},
		{
			name: "nested access point missing name",
			fs: EFSFileSystem{Name: "shared", PerformanceMode: "generalPurpose", ThroughputMode: "bursting",
				AccessPoints: []*EFSAccessPoint{{}}},
			wantErr: true,
			errText: "accessPoints: name is required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.fs.Validate(ctx)
			if tc.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.errText)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func existingFS() *types.FileSystemDescription {
	return &types.FileSystemDescription{
		FileSystemId:    aws.String("fs-123"),
		FileSystemArn:   aws.String("arn:aws:elasticfilesystem:us-east-1:1:file-system/fs-123"),
		Encrypted:       aws.Bool(true),
		PerformanceMode: types.PerformanceModeGeneralPurpose,
		ThroughputMode:  types.ThroughputModeBursting,
	}
}

func TestEFSFileSystem_ComputeDiff(t *testing.T) {
	ctx := context.Background()

	base := func() EFSFileSystem {
		return EFSFileSystem{
			Name:            "shared",
			Encrypted:       aws.Bool(true),
			PerformanceMode: "generalPurpose",
			ThroughputMode:  "bursting",
			Tags:            map[string]string{},
		}
	}

	t.Run("does not exist", func(t *testing.T) {
		r := base()
		d := r.computeDiff(ctx, nil, nil, "", nil, "")
		assert.NotNil(t, d)
		assert.Nil(t, d.AWSResource())
		assert.False(t, d.hasMutableChanges())
		assert.Contains(t, d.Differences()[0], "does not exist")
	})

	t.Run("no diff returns nil", func(t *testing.T) {
		r := base()
		d := r.computeDiff(ctx, existingFS(), map[string]string{}, "", nil, "")
		assert.Nil(t, d)
	})

	t.Run("throughput diff is mutable", func(t *testing.T) {
		r := base()
		r.ThroughputMode = "elastic"
		d := r.computeDiff(ctx, existingFS(), map[string]string{}, "", nil, "")
		assert.NotNil(t, d)
		assert.True(t, d.throughputDiff)
		assert.True(t, d.hasMutableChanges())
	})

	t.Run("encryption diff is immutable", func(t *testing.T) {
		r := base()
		r.Encrypted = aws.Bool(false)
		d := r.computeDiff(ctx, existingFS(), map[string]string{}, "", nil, "")
		assert.NotNil(t, d)
		assert.True(t, d.encryptedDiff)
		assert.False(t, d.hasMutableChanges())
	})

	t.Run("performance diff is immutable", func(t *testing.T) {
		r := base()
		r.PerformanceMode = "maxIO"
		d := r.computeDiff(ctx, existingFS(), map[string]string{}, "", nil, "")
		assert.NotNil(t, d)
		assert.True(t, d.performanceDiff)
		assert.False(t, d.hasMutableChanges())
	})

	t.Run("backup policy diff", func(t *testing.T) {
		r := base()
		r.BackupPolicy = aws.String("ENABLED")
		d := r.computeDiff(ctx, existingFS(), map[string]string{}, types.StatusDisabled, nil, "")
		assert.NotNil(t, d)
		assert.True(t, d.backupDiff)
		assert.True(t, d.hasMutableChanges())
	})

	t.Run("backup policy no diff when already enabled", func(t *testing.T) {
		r := base()
		r.BackupPolicy = aws.String("ENABLED")
		d := r.computeDiff(ctx, existingFS(), map[string]string{}, types.StatusEnabling, nil, "")
		assert.Nil(t, d)
	})

	t.Run("lifecycle diff", func(t *testing.T) {
		r := base()
		r.LifecyclePolicies = &EFSLifecyclePolicies{TransitionToIA: "AFTER_30_DAYS"}
		d := r.computeDiff(ctx, existingFS(), map[string]string{}, "", nil, "")
		assert.NotNil(t, d)
		assert.True(t, d.lifecycleDiff)
		assert.True(t, d.hasMutableChanges())
	})

	t.Run("lifecycle no diff", func(t *testing.T) {
		r := base()
		r.LifecyclePolicies = &EFSLifecyclePolicies{TransitionToIA: "AFTER_30_DAYS"}
		current := []types.LifecyclePolicy{{TransitionToIA: types.TransitionToIARulesAfter30Days}}
		d := r.computeDiff(ctx, existingFS(), map[string]string{}, "", current, "")
		assert.Nil(t, d)
	})

	t.Run("empty lifecycle block clears live policies (present == desired state)", func(t *testing.T) {
		r := base()
		r.LifecyclePolicies = &EFSLifecyclePolicies{} // present but empty => no policies desired
		current := []types.LifecyclePolicy{{TransitionToIA: types.TransitionToIARulesAfter30Days}}
		d := r.computeDiff(ctx, existingFS(), map[string]string{}, "", current, "")
		assert.NotNil(t, d)
		assert.True(t, d.lifecycleDiff, "an empty block must diff against live policies (to clear them)")
	})

	t.Run("omitted lifecycle reverts to default (clears live policies)", func(t *testing.T) {
		r := base()
		r.LifecyclePolicies = nil // omitted => revert to AWS default (no policies)
		current := []types.LifecyclePolicy{{TransitionToIA: types.TransitionToIARulesAfter30Days}}
		d := r.computeDiff(ctx, existingFS(), map[string]string{}, "", current, "")
		assert.NotNil(t, d)
		assert.True(t, d.lifecycleDiff, "omitting lifecyclePolicies must diff against live policies (to clear them)")
	})

	t.Run("omitted backup reverts to default DISABLED", func(t *testing.T) {
		r := base()
		r.BackupPolicy = nil // omitted => revert to AWS default (DISABLED)
		d := r.computeDiff(ctx, existingFS(), map[string]string{}, types.StatusEnabled, nil, "")
		assert.NotNil(t, d)
		assert.True(t, d.backupDiff, "omitting backupPolicy must diff against live ENABLED state (to disable it)")
		assert.True(t, d.hasMutableChanges())
	})

	t.Run("tag diff is mutable", func(t *testing.T) {
		r := base()
		r.Tags = map[string]string{"env": "prod"}
		d := r.computeDiff(ctx, existingFS(), map[string]string{"env": "stg"}, "", nil, "")
		assert.NotNil(t, d)
		assert.True(t, d.tagsDiff)
		assert.Equal(t, "prod", d.tagDiff.Upserts()["env"])
	})

	t.Run("ignores aws-reserved tags", func(t *testing.T) {
		r := base()
		r.Tags = map[string]string{"env": "prod"}
		awsTags := map[string]string{
			"env":                                  "prod",
			"aws:elasticfilesystem:default-backup": "enabled",
		}
		d := r.computeDiff(ctx, existingFS(), awsTags, "", nil, "")
		assert.Nil(t, d) // reserved tag must not produce a delete diff
	})

	t.Run("kms no diff when resolved arn equals existing", func(t *testing.T) {
		r := base()
		r.KmsKeyId = aws.String("alias/my-key")
		existing := existingFS()
		existing.KmsKeyId = aws.String("arn:aws:kms:us-east-1:1:key/abc")
		// caller resolved alias -> the same ARN AWS reports, so no spurious diff
		d := r.computeDiff(ctx, existing, map[string]string{}, "", nil, "arn:aws:kms:us-east-1:1:key/abc")
		assert.Nil(t, d)
	})

	t.Run("kms diff when resolved arn differs (immutable)", func(t *testing.T) {
		r := base()
		r.KmsKeyId = aws.String("alias/my-key")
		existing := existingFS()
		existing.KmsKeyId = aws.String("arn:aws:kms:us-east-1:1:key/abc")
		d := r.computeDiff(ctx, existing, map[string]string{}, "", nil, "arn:aws:kms:us-east-1:1:key/different")
		assert.NotNil(t, d)
		assert.True(t, d.kmsKeyDiff)
		assert.False(t, d.hasMutableChanges())
	})
}

func TestEFSFileSystem_classifyAccessPoints(t *testing.T) {
	const fsToken = "quickCreated-ae9f0eb8"

	t.Run("matches only by ClientToken (== name)", func(t *testing.T) {
		ap := &EFSAccessPoint{Name: "data"}
		fs := EFSFileSystem{Name: fsToken, AccessPoints: []*EFSAccessPoint{ap}}
		live := []types.AccessPointDescription{{
			AccessPointId: aws.String("fsap-2"),
			ClientToken:   aws.String(ap.clientToken()), // ClientToken == name
			Name:          aws.String("something-else"), // Name is ignored for matching
		}}

		add, matched, del := fs.classifyAccessPoints(live)
		assert.Empty(t, add)
		assert.Empty(t, del)
		assert.Len(t, matched, 1)
		assert.Equal(t, "fsap-2", aws.ToString(matched[0].existing.AccessPointId))
	})

	t.Run("a matching Name tag is NOT enough — token must match (console AP churns unless name=token)", func(t *testing.T) {
		// name does not equal the live console ClientToken, even though the Name tag matches.
		fs := EFSFileSystem{Name: fsToken, AccessPoints: []*EFSAccessPoint{{Name: "reviewbot-ap"}}}
		live := []types.AccessPointDescription{{
			AccessPointId: aws.String("fsap-1"),
			ClientToken:   aws.String("console-55683b92"),
			Name:          aws.String("reviewbot-ap"),
		}}

		add, matched, del := fs.classifyAccessPoints(live)
		assert.Empty(t, matched, "Name must not be used for matching")
		assert.Len(t, add, 1)
		assert.Equal(t, "reviewbot-ap", add[0].Name)
		assert.Len(t, del, 1)
		assert.Equal(t, "fsap-1", aws.ToString(del[0].AccessPointId))
	})

	t.Run("adopts a console AP when name is set to its ClientToken", func(t *testing.T) {
		fs := EFSFileSystem{Name: fsToken, AccessPoints: []*EFSAccessPoint{{Name: "console-55683b92"}}}
		live := []types.AccessPointDescription{{
			AccessPointId: aws.String("fsap-1"),
			ClientToken:   aws.String("console-55683b92"),
			Name:          aws.String("reviewbot-ap"),
		}}

		add, matched, del := fs.classifyAccessPoints(live)
		assert.Empty(t, add)
		assert.Empty(t, del)
		assert.Len(t, matched, 1)
		assert.Equal(t, "fsap-1", aws.ToString(matched[0].existing.AccessPointId))
	})

	t.Run("adds desired and deletes unmatched", func(t *testing.T) {
		fs := EFSFileSystem{Name: fsToken, AccessPoints: []*EFSAccessPoint{{Name: "wanted"}}}
		live := []types.AccessPointDescription{{
			AccessPointId: aws.String("fsap-old"),
			ClientToken:   aws.String("console-old"),
		}}

		add, matched, del := fs.classifyAccessPoints(live)
		assert.Len(t, add, 1)
		assert.Equal(t, "wanted", add[0].Name)
		assert.Empty(t, matched)
		assert.Len(t, del, 1)
		assert.Equal(t, "fsap-old", aws.ToString(del[0].AccessPointId))
	})
}

func TestEFSFileSystemDiff_hasMutableChanges(t *testing.T) {
	t.Run("immutable-only access point drift is not a mutable change", func(t *testing.T) {
		// accessPointDiffs is set (there is an entry to warn about) but nothing mutates AWS,
		// so the run must not be reported as an update.
		d := &EFSFileSystemDiff{accessPointDiffs: true}
		assert.False(t, d.hasMutableChanges())
	})

	t.Run("access point add/delete/tag change counts as mutable", func(t *testing.T) {
		d := &EFSFileSystemDiff{accessPointDiffs: true, accessPointMutations: true}
		assert.True(t, d.hasMutableChanges())
	})

	t.Run("immutable file-system fields alone are not mutable", func(t *testing.T) {
		d := &EFSFileSystemDiff{encryptedDiff: true, kmsKeyDiff: true, performanceDiff: true}
		assert.False(t, d.hasMutableChanges())
	})

	t.Run("real file-system and mount target changes are mutable", func(t *testing.T) {
		assert.True(t, (&EFSFileSystemDiff{throughputDiff: true}).hasMutableChanges())
		assert.True(t, (&EFSFileSystemDiff{backupDiff: true}).hasMutableChanges())
		assert.True(t, (&EFSFileSystemDiff{lifecycleDiff: true}).hasMutableChanges())
		assert.True(t, (&EFSFileSystemDiff{tagsDiff: true}).hasMutableChanges())
		assert.True(t, (&EFSFileSystemDiff{mountTargetDiffs: true}).hasMutableChanges())
	})
}

func TestEFSFileSystem_lifecyclePoliciesToAWS(t *testing.T) {
	// EFS PutLifecycleConfiguration rejects a nil LifecyclePolicies as a missing required field;
	// clearing policies requires a non-nil empty slice. Both an omitted block and a present-but-
	// empty block must therefore produce a non-nil empty slice.
	t.Run("omitted produces non-nil empty slice", func(t *testing.T) {
		r := EFSFileSystem{}
		got := r.lifecyclePoliciesToAWS()
		assert.NotNil(t, got)
		assert.Empty(t, got)
	})

	t.Run("empty block produces non-nil empty slice", func(t *testing.T) {
		r := EFSFileSystem{LifecyclePolicies: &EFSLifecyclePolicies{}}
		got := r.lifecyclePoliciesToAWS()
		assert.NotNil(t, got)
		assert.Empty(t, got)
	})

	t.Run("transitions map to one policy each", func(t *testing.T) {
		r := EFSFileSystem{LifecyclePolicies: &EFSLifecyclePolicies{
			TransitionToIA:      "AFTER_30_DAYS",
			TransitionToArchive: "AFTER_90_DAYS",
		}}
		got := r.lifecyclePoliciesToAWS()
		assert.Len(t, got, 2)
	})
}
