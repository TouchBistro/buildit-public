package resource

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/efs/types"
	"github.com/stretchr/testify/assert"
)

func TestEFSAccessPoint_Normalize(t *testing.T) {
	ctx := context.Background()

	t.Run("merges global tags and does not synthesize a Name tag", func(t *testing.T) {
		r := &EFSAccessPoint{Name: "app"}
		r.Normalize(ctx, map[string]string{"owner": "team-a"})
		assert.Equal(t, "team-a", r.Tags["owner"])
		// name is the ClientToken (identity), not a display tag — no Name tag is auto-set.
		_, hasName := r.Tags["Name"]
		assert.False(t, hasName, "Name tag must not be auto-set")
	})

	t.Run("keeps a user-supplied Name tag as an ordinary tag", func(t *testing.T) {
		r := &EFSAccessPoint{Name: "app", Tags: map[string]string{"Name": "custom"}}
		r.Normalize(ctx, nil)
		assert.Equal(t, "custom", r.Tags["Name"])
	})
}

func TestEFSAccessPoint_clientToken(t *testing.T) {
	r := EFSAccessPoint{Name: "data"}
	// ClientToken is the access point name.
	assert.Equal(t, "data", r.clientToken())
}

func TestEFSAccessPoint_validate(t *testing.T) {
	tests := []struct {
		name     string
		ap       EFSAccessPoint
		wantMsgs []string
	}{
		{
			name: "valid",
			ap:   EFSAccessPoint{Name: "app"},
		},
		{
			name:     "empty name",
			ap:       EFSAccessPoint{},
			wantMsgs: []string{"name is required"},
		},
		{
			name: "bad permissions",
			ap: EFSAccessPoint{Name: "app", RootDirectory: &EFSRootDirectory{
				Path:         "/app",
				CreationInfo: &EFSCreationInfo{Permissions: "999"},
			}},
			wantMsgs: []string{"must be an octal string"},
		},
		{
			name: "good permissions",
			ap: EFSAccessPoint{Name: "app", RootDirectory: &EFSRootDirectory{
				Path:         "/app",
				CreationInfo: &EFSCreationInfo{Permissions: "0755"},
			}},
		},
		{
			name:     "posixUser missing uid",
			ap:       EFSAccessPoint{Name: "app", PosixUser: &EFSPosixUser{Gid: aws.Int64(1000)}},
			wantMsgs: []string{"posixUser.uid is required"},
		},
		{
			name:     "posixUser missing gid",
			ap:       EFSAccessPoint{Name: "app", PosixUser: &EFSPosixUser{Uid: aws.Int64(1000)}},
			wantMsgs: []string{"posixUser.gid is required"},
		},
		{
			name: "posixUser with uid and gid",
			ap:   EFSAccessPoint{Name: "app", PosixUser: &EFSPosixUser{Uid: aws.Int64(0), Gid: aws.Int64(0)}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msgs := tc.ap.validate()
			if len(tc.wantMsgs) == 0 {
				assert.Empty(t, msgs)
				return
			}
			for _, want := range tc.wantMsgs {
				assert.Contains(t, joinMsgs(msgs), want)
			}
		})
	}
}

func TestEFSAccessPoint_PosixEqual(t *testing.T) {
	desired := (&EFSAccessPoint{PosixUser: &EFSPosixUser{Uid: aws.Int64(1000), Gid: aws.Int64(1000), SecondaryGids: []int64{2000, 3000}}}).toPosixUser()

	t.Run("reordered secondaryGids are equal", func(t *testing.T) {
		current := &types.PosixUser{Uid: aws.Int64(1000), Gid: aws.Int64(1000), SecondaryGids: []int64{3000, 2000}}
		assert.True(t, efsPosixEqual(desired, current))
	})

	t.Run("different uid is not equal", func(t *testing.T) {
		current := &types.PosixUser{Uid: aws.Int64(2000), Gid: aws.Int64(1000), SecondaryGids: []int64{2000, 3000}}
		assert.False(t, efsPosixEqual(desired, current))
	})

	t.Run("nil vs set is not equal", func(t *testing.T) {
		assert.False(t, efsPosixEqual(desired, nil))
		assert.True(t, efsPosixEqual(nil, nil))
	})
}

func TestEFSAccessPoint_RootDirEqual(t *testing.T) {
	desired := (&EFSAccessPoint{RootDirectory: &EFSRootDirectory{
		Path:         "/app",
		CreationInfo: &EFSCreationInfo{OwnerUid: 1000, OwnerGid: 1000, Permissions: "0755"},
	}}).toRootDirectory()

	t.Run("identical root directories are equal", func(t *testing.T) {
		current := &types.RootDirectory{
			Path: aws.String("/app"),
			CreationInfo: &types.CreationInfo{
				OwnerUid:    aws.Int64(1000),
				OwnerGid:    aws.Int64(1000),
				Permissions: aws.String("0755"),
			},
		}
		assert.True(t, efsRootDirEqual(desired, current))
	})

	t.Run("different path is not equal", func(t *testing.T) {
		current := &types.RootDirectory{Path: aws.String("/other")}
		assert.False(t, efsRootDirEqual(desired, current))
	})
}

func TestEFSAccessPoint_equals(t *testing.T) {
	ctx := context.Background()

	existing := func() *types.AccessPointDescription {
		return &types.AccessPointDescription{
			AccessPointId: aws.String("fsap-123"),
			PosixUser:     &types.PosixUser{Uid: aws.Int64(1000), Gid: aws.Int64(1000)},
			RootDirectory: &types.RootDirectory{
				Path: aws.String("/data"),
				CreationInfo: &types.CreationInfo{
					OwnerUid:    aws.Int64(1000),
					OwnerGid:    aws.Int64(1000),
					Permissions: aws.String("0755"),
				},
			},
			Tags: []types.Tag{{Key: aws.String("env"), Value: aws.String("prod")}},
		}
	}

	desired := func() EFSAccessPoint {
		return EFSAccessPoint{
			Name:      "data",
			PosixUser: &EFSPosixUser{Uid: aws.Int64(1000), Gid: aws.Int64(1000)},
			RootDirectory: &EFSRootDirectory{
				Path:         "/data",
				CreationInfo: &EFSCreationInfo{OwnerUid: 1000, OwnerGid: 1000, Permissions: "0755"},
			},
			Tags: map[string]string{"env": "prod"},
		}
	}

	t.Run("identical returns no diff", func(t *testing.T) {
		d, err := desired().equals(ctx, existing())
		assert.NoError(t, err)
		assert.Nil(t, d)
	})

	t.Run("immutable rootDirectory drift is reported", func(t *testing.T) {
		ap := desired()
		ap.RootDirectory.Path = "/elsewhere"
		d, err := ap.equals(ctx, existing())
		assert.NoError(t, err)
		apd, ok := d.(*EFSAccessPointDiff)
		assert.True(t, ok)
		assert.True(t, apd.immutableDiff)
		assert.False(t, apd.tagsDiff)
	})

	t.Run("immutable posixUser drift is reported", func(t *testing.T) {
		ap := desired()
		ap.PosixUser.Uid = aws.Int64(2000)
		d, err := ap.equals(ctx, existing())
		assert.NoError(t, err)
		apd, ok := d.(*EFSAccessPointDiff)
		assert.True(t, ok)
		assert.True(t, apd.immutableDiff)
	})

	t.Run("tag drift is reported as a mutable change", func(t *testing.T) {
		ap := desired()
		ap.Tags = map[string]string{"env": "staging"}
		d, err := ap.equals(ctx, existing())
		assert.NoError(t, err)
		apd, ok := d.(*EFSAccessPointDiff)
		assert.True(t, ok)
		assert.True(t, apd.tagsDiff)
		assert.False(t, apd.immutableDiff)
		assert.Equal(t, "fsap-123", apd.accessPointId)
	})
}

func TestFormatChildDiffMessages(t *testing.T) {
	assert.Equal(t,
		[]string{"mount target subnet-a: security groups will be updated"},
		formatMountTargetDiffMessages("subnet-a", []string{"security groups will be updated"}),
	)
	assert.Equal(t,
		[]string{"access point ap-1: msg1", "access point ap-1: msg2"},
		formatAccessPointDiffMessages("ap-1", []string{"msg1", "msg2"}),
	)
	assert.Empty(t, formatMountTargetDiffMessages("x", nil))
}
