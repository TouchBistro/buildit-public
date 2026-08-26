package awsw_test

import (
	"context"
	"testing"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/stretchr/testify/assert"
)

func TestSQSQueueArnForIdentifier(t *testing.T) {
	t.Run("Resolves direct ARN", func(t *testing.T) {
		s := awsw.SQS{}
		ctx := context.Background()
		identifier := "arn:aws:sqs:us-east-1:123456789012:MyQueue"
		arn, err := s.QueueArnForIdentifier(ctx, identifier)
		assert.NoError(t, err)
		assert.Equal(t, identifier, *arn)
	})

	t.Run("Returns error for invalid provider", func(t *testing.T) {
		s := awsw.SQS{}
		ctx := context.Background()
		identifier := "invalid::MyQueue"
		_, err := s.QueueArnForIdentifier(ctx, identifier)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "provider \"invalid\" not found")
	})
}
