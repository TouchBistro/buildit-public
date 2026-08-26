package awsw_test

import (
	"context"
	"testing"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/stretchr/testify/assert"
)

func TestS3BucketArnForIdentifier(t *testing.T) {
	t.Run("Resolves direct ARN", func(t *testing.T) {
		s := awsw.S3{}
		ctx := context.Background()
		identifier := "arn:aws:s3:::my-bucket"
		arn, err := s.BucketArnForIdentifier(ctx, identifier)
		assert.NoError(t, err)
		assert.Equal(t, "arn:aws:s3:::my-bucket", *arn)
	})

	t.Run("Returns error for invalid provider", func(t *testing.T) {
		s := awsw.S3{}
		ctx := context.Background()
		identifier := "invalid::my-bucket"
		_, err := s.BucketArnForIdentifier(ctx, identifier)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "provider \"invalid\" not found")
	})
}
