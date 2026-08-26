package awsw_test

import (
	"context"
	"testing"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/stretchr/testify/assert"
)

func TestEFSFileSystemArnForIdentifier(t *testing.T) {
	t.Run("Resolves direct ARN", func(t *testing.T) {
		e := awsw.EFS{}
		ctx := context.Background()
		identifier := "arn:aws:elasticfilesystem:us-east-1:123456789012:file-system/fs-12345678"
		arn, err := e.FileSystemArnForIdentifier(ctx, identifier)
		assert.NoError(t, err)
		assert.Equal(t, identifier, *arn)
	})

	t.Run("Returns error for invalid provider", func(t *testing.T) {
		e := awsw.EFS{}
		ctx := context.Background()
		identifier := "invalid::fs-12345678"
		_, err := e.FileSystemArnForIdentifier(ctx, identifier)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "provider \"invalid\" not found")
	})
}

func TestEFSFileSystemIDForIdentifier(t *testing.T) {
	tests := []struct {
		name       string
		identifier string
		want       string
		wantErr    string
	}{
		{
			name:       "Passes through physical file system ID",
			identifier: "fs-0763b5806d2a9e5ab",
			want:       "fs-0763b5806d2a9e5ab",
		},
		{
			name:       "Passes through provider-prefixed physical ID without provider lookup",
			identifier: "staging::fs-0763b5806d2a9e5ab",
			want:       "fs-0763b5806d2a9e5ab",
		},
		{
			name:       "Returns error for invalid provider with resource name",
			identifier: "invalid::reviewbot-v2-efs",
			wantErr:    "provider \"invalid\" not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := awsw.EFS{}
			id, err := e.FileSystemIDForIdentifier(context.Background(), tt.identifier)
			if tt.wantErr != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.want, *id)
		})
	}
}

func TestEFSAccessPointArnForIdentifier(t *testing.T) {
	tests := []struct {
		name       string
		identifier string
		want       string
		wantErr    string
	}{
		{
			name:       "Passes through direct ARN",
			identifier: "arn:aws:elasticfilesystem:us-east-1:123456789012:access-point/fsap-02e6b009fcdea54d0",
			want:       "arn:aws:elasticfilesystem:us-east-1:123456789012:access-point/fsap-02e6b009fcdea54d0",
		},
		{
			name:       "Returns error for invalid provider with access point name",
			identifier: "invalid::reviewbot-v2-accesspt",
			wantErr:    "provider \"invalid\" not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := awsw.EFS{}
			arn, err := e.AccessPointArnForIdentifier(context.Background(), tt.identifier)
			if tt.wantErr != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.want, *arn)
		})
	}
}

func TestEFSResolveVolumeIDs(t *testing.T) {
	e := awsw.EFS{}
	ctx := context.Background()

	t.Run("Nil references pass through untouched", func(t *testing.T) {
		fsID, apID, err := e.ResolveVolumeIDs(ctx, nil, nil)
		assert.NoError(t, err)
		assert.Nil(t, fsID)
		assert.Nil(t, apID)
	})

	t.Run("Physical IDs pass through without AWS calls", func(t *testing.T) {
		fs := "fs-0763b5806d2a9e5ab"
		ap := "fsap-02e6b009fcdea54d0"
		fsID, apID, err := e.ResolveVolumeIDs(ctx, &fs, &ap)
		assert.NoError(t, err)
		assert.Equal(t, fs, *fsID)
		assert.Equal(t, ap, *apID)
	})

	t.Run("Wraps resolution errors with the failing reference", func(t *testing.T) {
		fs := "invalid::my-efs"
		_, _, err := e.ResolveVolumeIDs(ctx, &fs, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "error resolving efs file system reference invalid::my-efs")
	})
}

func TestEFSTruncateToken(t *testing.T) {
	// token stays within EFS's 64-char limit, keeping the suffix
	long := awsw.EFSTruncateToken("really-long-access-point-name-that-exceeds-the-sixty-four-character-client-token-limit")
	assert.Len(t, long, 64)
	assert.Equal(t, "sixty-four-character-client-token-limit", long[len(long)-39:])
	// short tokens are returned unchanged
	assert.Equal(t, "data", awsw.EFSTruncateToken("data"))
}

func TestEFSAccessPointIDForIdentifier(t *testing.T) {
	tests := []struct {
		name       string
		identifier string
		want       string
		wantErr    string
	}{
		{
			name:       "Passes through physical access point ID",
			identifier: "fsap-02e6b009fcdea54d0",
			want:       "fsap-02e6b009fcdea54d0",
		},
		{
			name:       "Passes through provider-prefixed physical ID without provider lookup",
			identifier: "staging::fsap-02e6b009fcdea54d0",
			want:       "fsap-02e6b009fcdea54d0",
		},
		{
			name:       "Returns error for invalid provider with access point name",
			identifier: "invalid::reviewbot-v2-accesspt",
			wantErr:    "provider \"invalid\" not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := awsw.EFS{}
			id, err := e.AccessPointIDForIdentifier(context.Background(), tt.identifier)
			if tt.wantErr != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.want, *id)
		})
	}
}
