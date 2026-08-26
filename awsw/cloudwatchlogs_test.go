package awsw_test

import (
	"context"
	"testing"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/stretchr/testify/assert"
)

func TestCloudWatchLogsLogGroupArnForIdentifier(t *testing.T) {
	t.Run("Resolves direct ARN", func(t *testing.T) {
		c := awsw.CloudWatchLogs{}
		ctx := context.Background()
		identifier := "arn:aws:logs:us-east-1:123456789012:log-group:my-log-group:*"
		arn, err := c.LogGroupArnForIdentifier(ctx, identifier)
		assert.NoError(t, err)
		assert.Equal(t, identifier, *arn)
	})

	t.Run("Returns error for invalid provider", func(t *testing.T) {
		c := awsw.CloudWatchLogs{}
		ctx := context.Background()
		identifier := "invalid::my-log-group"
		_, err := c.LogGroupArnForIdentifier(ctx, identifier)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "provider \"invalid\" not found")
	})
}
