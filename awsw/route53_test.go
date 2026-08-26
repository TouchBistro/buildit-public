package awsw_test

import (
	"context"
	"testing"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/stretchr/testify/assert"
)

func TestRoute53HostedZoneArnForIdentifier(t *testing.T) {
	t.Run("Resolves direct ARN", func(t *testing.T) {
		r := awsw.Route53{}
		ctx := context.Background()
		identifier := "arn:aws:route53:::hostedzone/Z1234567890"
		arn, err := r.HostedZoneArnForIdentifier(ctx, identifier)
		assert.NoError(t, err)
		assert.Equal(t, identifier, *arn)
	})

	t.Run("Returns error for invalid provider", func(t *testing.T) {
		r := awsw.Route53{}
		ctx := context.Background()
		identifier := "invalid::Z1234567890"
		_, err := r.HostedZoneArnForIdentifier(ctx, identifier)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "provider \"invalid\" not found")
	})
}
