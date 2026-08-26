package awsw_test

import (
	"context"
	"testing"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/stretchr/testify/assert"
)

func TestECSClusterArnForIdentifier(t *testing.T) {
	t.Run("Resolves direct ARN", func(t *testing.T) {
		e := awsw.ECS{}
		ctx := context.Background()
		identifier := "arn:aws:ecs:us-east-1:123456789012:cluster/my-cluster"
		arn, err := e.ClusterArnForIdentifier(ctx, identifier)
		assert.NoError(t, err)
		assert.Equal(t, identifier, *arn)
	})

	t.Run("Returns error for invalid provider", func(t *testing.T) {
		e := awsw.ECS{}
		ctx := context.Background()
		identifier := "invalid::my-cluster"
		_, err := e.ClusterArnForIdentifier(ctx, identifier)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "provider \"invalid\" not found")
	})
}
