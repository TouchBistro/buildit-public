package awsw_test

import (
	"context"
	"testing"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/stretchr/testify/assert"
)

func TestCloudWatchAlarmArnForIdentifier(t *testing.T) {
	t.Run("Resolves direct ARN", func(t *testing.T) {
		c := awsw.CloudWatch{}
		ctx := context.Background()
		identifier := "arn:aws:cloudwatch:us-east-1:123456789012:alarm:my-alarm"
		arn, err := c.AlarmArnForIdentifier(ctx, identifier)
		assert.NoError(t, err)
		assert.Equal(t, identifier, *arn)
	})

	t.Run("Returns error for invalid provider", func(t *testing.T) {
		c := awsw.CloudWatch{}
		ctx := context.Background()
		identifier := "invalid::my-alarm"
		_, err := c.AlarmArnForIdentifier(ctx, identifier)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "provider \"invalid\" not found")
	})
}
