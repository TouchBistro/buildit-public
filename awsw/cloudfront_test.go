package awsw_test

import (
	"context"
	"testing"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/stretchr/testify/assert"
)

func TestIsCloudfrontPolicyId(t *testing.T) {
	assert.True(t, awsw.IsCloudfrontPolicyId("658327ea-f89d-4fab-a63d-7e88639e58f6"))
	assert.True(t, awsw.IsCloudfrontPolicyId("4135EA2D-6DF8-44A3-9DF3-4B5A84BE39AD"))
	assert.False(t, awsw.IsCloudfrontPolicyId("CachingOptimized"))
	assert.False(t, awsw.IsCloudfrontPolicyId("Managed-CachingOptimized"))
	assert.False(t, awsw.IsCloudfrontPolicyId("658327ea-f89d-4fab-a63d"))
	assert.False(t, awsw.IsCloudfrontPolicyId(""))
}

func TestCloudfrontPolicyIdForIdentifier(t *testing.T) {
	// Policy ids resolve without any AWS call, so the zero-value wrapper suffices.
	c := awsw.Cloudfront{}
	ctx := context.Background()
	id := "658327ea-f89d-4fab-a63d-7e88639e58f6"

	t.Run("Cache policy id passes through", func(t *testing.T) {
		got, err := c.CachePolicyIdForIdentifier(ctx, id)
		assert.NoError(t, err)
		assert.Equal(t, id, got)
	})

	t.Run("Origin request policy id passes through", func(t *testing.T) {
		got, err := c.OriginRequestPolicyIdForIdentifier(ctx, id)
		assert.NoError(t, err)
		assert.Equal(t, id, got)
	})

	t.Run("Response headers policy id passes through", func(t *testing.T) {
		got, err := c.ResponseHeadersPolicyIdForIdentifier(ctx, id)
		assert.NoError(t, err)
		assert.Equal(t, id, got)
	})
}
