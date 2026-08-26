package resource

import (
	"context"
	"testing"

	"github.com/TouchBistro/buildit/util"
	"github.com/stretchr/testify/assert"
)

func TestS3Bucket_Normalize(t *testing.T) {
	t.Run("strips arn prefix", func(t *testing.T) {
		b := &S3Bucket{
			Bucket: "arn:aws:s3:::my-bucket",
		}
		b.Normalize(context.Background())
		assert.Equal(t, "my-bucket", b.Bucket)
	})

	t.Run("merges tags", func(t *testing.T) {
		b := &S3Bucket{
			Bucket: "my-bucket",
			Tags: map[string]string{
				"Environment": "production",
			},
			GlobalTags: map[string]string{
				"Owner": "team-a",
			},
		}
		b.Normalize(context.Background())
		assert.Equal(t, "production", b.Tags["Environment"])
		assert.Equal(t, "team-a", b.Tags["Owner"])
	})

	// config stamps the resource-id from the config key before Normalize runs, and this is
	// the one resource whose Normalize rewrites its own identifier. Without the re-sync the
	// tag would keep the arn form while Identifier() returned the bare bucket name.
	t.Run("re-points the resource-id at the trimmed bucket name", func(t *testing.T) {
		b := &S3Bucket{
			Bucket:     "arn:aws:s3:::example-bucket",
			GlobalTags: map[string]string{util.BuilditResourceIDTagKey: "arn:aws:s3:::example-bucket"},
		}
		b.Normalize(context.Background())

		assert.Equal(t, "example-bucket", b.Identifier())
		assert.Equal(t, b.Identifier(), b.Tags[util.BuilditResourceIDTagKey])
	})

	// Nothing to re-point when the name was never in arn form.
	t.Run("leaves a plain resource-id alone", func(t *testing.T) {
		b := &S3Bucket{
			Bucket:     "example-bucket",
			GlobalTags: map[string]string{util.BuilditResourceIDTagKey: "example-bucket"},
		}
		b.Normalize(context.Background())

		assert.Equal(t, "example-bucket", b.Tags[util.BuilditResourceIDTagKey])
	})
}

func TestS3Bucket_Validate(t *testing.T) {
	t.Run("valid configuration", func(t *testing.T) {
		b := &S3Bucket{
			Bucket: "my-bucket",
		}
		err := b.Validate(context.Background())
		assert.Nil(t, err)
	})

	t.Run("missing bucket name", func(t *testing.T) {
		b := &S3Bucket{
			Bucket: "",
		}
		err := b.Validate(context.Background())
		assert.NotNil(t, err)
		assert.Contains(t, err.Error(), "bucket name is required")
	})
}
