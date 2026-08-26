package awsw_test

import (
	"context"
	"testing"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/stretchr/testify/assert"
)

func TestLambdaFunctionArnForIdentifier(t *testing.T) {
	t.Run("Resolves direct ARN", func(t *testing.T) {
		l := awsw.Lambda{}
		ctx := context.Background()
		identifier := "arn:aws:lambda:us-east-1:123456789012:function:my-func"
		arn, err := l.FunctionArnForIdentifier(ctx, identifier)
		assert.NoError(t, err)
		assert.Equal(t, identifier, *arn)
	})

	t.Run("Returns error for invalid provider", func(t *testing.T) {
		l := awsw.Lambda{}
		ctx := context.Background()
		identifier := "invalid::my-func"
		_, err := l.FunctionArnForIdentifier(ctx, identifier)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "provider \"invalid\" not found")
	})
}
