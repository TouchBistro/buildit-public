package lambda

import (
	"context"
	"strings"
	"testing"

	"github.com/TouchBistro/buildit/resource"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// AWS does not support tagging Lambda layers, so a config that sets tags must fail
// Validate with the standard unsupported-tags message — see resource/AGENTS.md.
func TestLayer_Validate_TagsRejected(t *testing.T) {
	// Code bucket/key are set because Validate formats them into its checksum message
	// unconditionally; a nil there panics before the tags rejection is reachable.
	layer := Layer{
		LayerRef: LayerRef{Name: "example-layer"},
		Code:     Code{S3Bucket: aws.String("example-bucket"), Key: aws.String("example-key.zip")},
		Tags:     map[string]string{"team": "example-team"},
	}

	err := layer.Validate(context.Background())
	require.Error(t, err)

	vErr, ok := err.(*resource.ValidationError)
	require.True(t, ok, "expected a ValidationError, got %T", err)
	assert.True(t, strings.Contains(strings.Join(vErr.Messages, "\n"), resource.UnsupportedTagsMessage("lambda-layer")),
		"validation messages should carry the standard unsupported-tags message: %v", vErr.Messages)
}
