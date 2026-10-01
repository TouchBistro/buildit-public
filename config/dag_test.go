package config

import (
	"testing"

	"github.com/TouchBistro/buildit/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Two resources of different types with the same name produce the same
// provider::name key. Before DEVOPS-8902 the second silently replaced the first
// in the vertex map, so one declared resource vanished from every plan.
func TestAddVertexRejectsSameKeyAcrossTypes(t *testing.T) {
	bucket := resource.S3Bucket{Bucket: "example-thing"}
	queue := resource.SQSQueue{Name: "example-thing"}
	require.Equal(t, bucket.Key(), queue.Key(), "fixture must collide on key")

	g := &Graph{}
	require.NoError(t, g.AddVertex(bucket, nil))

	err := g.AddVertex(queue, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "main::example-thing")
	assert.Contains(t, err.Error(), `s3-bucket "example-thing"`)
	assert.Contains(t, err.Error(), `sqs-queue "example-thing"`)

	// The first resource must survive untouched.
	assert.Equal(t, 1, g.Size())
	got, err := g.GetVertex(bucket.Key())
	require.NoError(t, err)
	assert.IsType(t, resource.S3Bucket{}, got)
}

// Same type, same name is also a collision (two config files can declare it).
func TestAddVertexRejectsSameKeySameType(t *testing.T) {
	g := &Graph{}
	require.NoError(t, g.AddVertex(resource.S3Bucket{Bucket: "example-thing"}, nil))

	err := g.AddVertex(resource.S3Bucket{Bucket: "example-thing"}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `s3-bucket "example-thing" and s3-bucket "example-thing"`)
}

// Same name under different providers is two different keys, and fine.
func TestAddVertexAllowsSameNameDifferentProvider(t *testing.T) {
	a := resource.S3Bucket{Bucket: "example-thing"}
	a.Context.ProviderName = "example-a"
	b := resource.SQSQueue{Name: "example-thing"}
	b.Context.ProviderName = "example-b"

	g := &Graph{}
	require.NoError(t, g.AddVertex(a, nil))
	require.NoError(t, g.AddVertex(b, nil))
	assert.Equal(t, 2, g.Size())
}

func TestResourceTypeName(t *testing.T) {
	tests := []struct {
		name string
		res  resource.Resource
		want string
	}{
		{"value", resource.S3Bucket{}, "s3-bucket"},
		{"pointer", &resource.SQSQueue{}, "sqs-queue"},
		{"certificate", resource.ACMCertificate{}, "certificate"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, resourceTypeName(tt.res))
		})
	}
}
