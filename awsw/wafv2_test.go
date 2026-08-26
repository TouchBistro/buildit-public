package awsw_test

import (
	"context"
	"testing"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/stretchr/testify/assert"
)

func TestWAFV2WebACLArnForIdentifier(t *testing.T) {
	t.Run("Resolves direct ARN", func(t *testing.T) {
		// The ARN tier resolves without any AWS call, so the zero-value wrapper suffices.
		w := awsw.WAFV2{}
		ctx := context.Background()
		identifier := "arn:aws:wafv2:us-east-1:123456789012:global/webacl/my-acl/abc-123"
		arn, err := w.WebACLArnForIdentifier(ctx, identifier)
		assert.NoError(t, err)
		assert.Equal(t, identifier, *arn)
	})
}
