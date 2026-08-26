package awsw_test

import (
	"context"
	"testing"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/stretchr/testify/assert"
)

func TestIAMRoleArnForIdentifier(t *testing.T) {
	t.Run("Resolves direct ARN", func(t *testing.T) {
		i := awsw.IAM{}
		ctx := context.Background()
		identifier := "arn:aws:iam::123456789012:role/my-role"
		arn, err := i.RoleArnForIdentifier(ctx, identifier)
		assert.NoError(t, err)
		assert.Equal(t, identifier, *arn)
	})

	t.Run("Returns error for invalid provider", func(t *testing.T) {
		i := awsw.IAM{}
		ctx := context.Background()
		identifier := "invalid::my-role"
		_, err := i.RoleArnForIdentifier(ctx, identifier)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "provider \"invalid\" not found")
	})
}

func TestIAMPolicyArnForIdentifier(t *testing.T) {
	t.Run("Resolves direct ARN", func(t *testing.T) {
		i := awsw.IAM{}
		ctx := context.Background()
		identifier := "arn:aws:iam::123456789012:policy/my-policy"
		arn, err := i.PolicyArnForIdentifier(ctx, identifier)
		assert.NoError(t, err)
		assert.Equal(t, identifier, *arn)
	})
}
