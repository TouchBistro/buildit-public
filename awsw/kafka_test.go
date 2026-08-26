package awsw_test

import (
	"context"
	"testing"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/stretchr/testify/assert"
)

func TestKafkaClusterArnForIdentifier(t *testing.T) {
	t.Run("Resolves direct ARN", func(t *testing.T) {
		k := awsw.Kafka{}
		ctx := context.Background()
		identifier := "arn:aws:kafka:us-east-1:123456789012:cluster/my-cluster/12345678-1234-1234-1234-123456789012-1"
		arn, err := k.ClusterArnForIdentifier(ctx, identifier)
		assert.NoError(t, err)
		assert.Equal(t, identifier, *arn)
	})

	t.Run("Returns error for invalid provider", func(t *testing.T) {
		k := awsw.Kafka{}
		ctx := context.Background()
		identifier := "invalid::my-cluster"
		_, err := k.ClusterArnForIdentifier(ctx, identifier)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "provider \"invalid\" not found")
	})
}
