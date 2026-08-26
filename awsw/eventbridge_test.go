package awsw_test

import (
	"context"
	"testing"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/stretchr/testify/assert"
)

func TestEventBridgeRuleArnForIdentifier(t *testing.T) {
	t.Run("Resolves direct ARN", func(t *testing.T) {
		e := awsw.EventBridge{}
		ctx := context.Background()
		identifier := "arn:aws:events:us-east-1:123456789012:rule/my-rule"
		arn, err := e.RuleArnForIdentifier(ctx, identifier)
		assert.NoError(t, err)
		assert.Equal(t, identifier, *arn)
	})

	t.Run("Returns error for invalid provider", func(t *testing.T) {
		e := awsw.EventBridge{}
		ctx := context.Background()
		identifier := "invalid::my-rule"
		_, err := e.RuleArnForIdentifier(ctx, identifier)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "provider \"invalid\" not found")
	})
}

func TestEventBridgeBusArnForIdentifier(t *testing.T) {
	t.Run("Resolves direct ARN", func(t *testing.T) {
		e := awsw.EventBridge{}
		ctx := context.Background()
		identifier := "arn:aws:events:us-east-1:123456789012:event-bus/my-bus"
		arn, err := e.BusArnForIdentifier(ctx, identifier)
		assert.NoError(t, err)
		assert.Equal(t, identifier, *arn)
	})
}
