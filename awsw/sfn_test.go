package awsw_test

import (
	"context"
	"testing"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/stretchr/testify/assert"
)

func TestSFNStateMachineArnForIdentifier(t *testing.T) {
	t.Run("Resolves direct ARN", func(t *testing.T) {
		s := awsw.SFN{}
		ctx := context.Background()
		identifier := "arn:aws:states:us-east-1:123456789012:stateMachine:my-sm"
		arn, err := s.StateMachineArnForIdentifier(ctx, identifier)
		assert.NoError(t, err)
		assert.Equal(t, identifier, *arn)
	})

	t.Run("Returns error for invalid provider", func(t *testing.T) {
		s := awsw.SFN{}
		ctx := context.Background()
		identifier := "invalid::my-sm"
		_, err := s.StateMachineArnForIdentifier(ctx, identifier)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "provider \"invalid\" not found")
	})
}
