package awsw_test

import (
	"context"
	"testing"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/stretchr/testify/assert"
)

func TestSecretsManagerSecretArnForIdentifier(t *testing.T) {
	t.Run("Resolves direct ARN", func(t *testing.T) {
		s := awsw.SecretsManager{}
		ctx := context.Background()
		identifier := "arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret-123456"
		arn, err := s.SecretArnForIdentifier(ctx, identifier)
		assert.NoError(t, err)
		assert.Equal(t, identifier, *arn)
	})

	t.Run("Returns error for invalid provider", func(t *testing.T) {
		s := awsw.SecretsManager{}
		ctx := context.Background()
		identifier := "invalid::my-secret"
		_, err := s.SecretArnForIdentifier(ctx, identifier)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "provider \"invalid\" not found")
	})
}
