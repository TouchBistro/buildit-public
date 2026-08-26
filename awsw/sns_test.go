package awsw_test

import (
	"context"
	"testing"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/stretchr/testify/assert"
)

func TestSNSTopicArnForIdentifier(t *testing.T) {
	t.Run("Resolves direct ARN", func(t *testing.T) {
		s := awsw.SNS{}
		ctx := context.Background()
		identifier := "arn:aws:sns:us-east-1:123456789012:MyTopic"
		arn, err := s.TopicArnForIdentifier(ctx, identifier)
		assert.NoError(t, err)
		assert.Equal(t, identifier, *arn)
	})

	t.Run("Returns error for invalid provider", func(t *testing.T) {
		s := awsw.SNS{}
		ctx := context.Background()
		identifier := "invalid::MyTopic"
		_, err := s.TopicArnForIdentifier(ctx, identifier)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "provider \"invalid\" not found")
	})
}
