package awsw_test

import (
	"context"
	"testing"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/stretchr/testify/assert"
)

func TestACMCertificateArnForIdentifier(t *testing.T) {
	t.Run("Resolves direct ARN", func(t *testing.T) {
		a := awsw.ACM{}
		ctx := context.Background()
		identifier := "arn:aws:acm:us-east-1:123456789012:certificate/12345678-1234-1234-1234-123456789012"
		arn, err := a.CertificateArnForIdentifier(ctx, identifier)
		assert.NoError(t, err)
		assert.Equal(t, identifier, *arn)
	})

	t.Run("Returns error for invalid provider", func(t *testing.T) {
		a := awsw.ACM{}
		ctx := context.Background()
		identifier := "invalid::domain.com"
		_, err := a.CertificateArnForIdentifier(ctx, identifier)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "provider \"invalid\" not found")
	})
}
