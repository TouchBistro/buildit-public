package lambda

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
)

func TestFunction_resolveFileSystemArn(t *testing.T) {
	ctx := context.Background()

	t.Run("no-op when no file system is configured", func(t *testing.T) {
		r := Function{Name: "fn"}
		assert.NoError(t, r.resolveFileSystemArn(ctx))
		assert.Nil(t, r.FileSystem)
	})

	t.Run("no-op when the arn is already resolved", func(t *testing.T) {
		arn := "arn:aws:elasticfilesystem:us-east-1:123456789012:access-point/fsap-02e6b009fcdea54d0"
		r := Function{Name: "fn", FileSystem: &FileSystem{Name: "example_access_point", Arn: aws.String(arn)}}
		assert.NoError(t, r.resolveFileSystemArn(ctx))
		assert.Equal(t, arn, *r.FileSystem.Arn)
	})

	t.Run("resolved arn persists through the value receiver", func(t *testing.T) {
		// FileSystem is a pointer field, so a resolution performed by one lifecycle phase
		// (e.g. Compare) must be visible to later phases (e.g. applyDiffs) on copies of r.
		fs := &FileSystem{Name: "example_access_point"}
		r := Function{Name: "fn", FileSystem: fs}
		cp := r
		cp.FileSystem.Arn = aws.String("arn:aws:elasticfilesystem:us-east-1:123456789012:access-point/fsap-1")
		assert.NotNil(t, r.FileSystem.Arn)
	})
}
