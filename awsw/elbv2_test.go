package awsw_test

import (
	"context"
	"testing"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/stretchr/testify/assert"
)

func TestELBLoadBalancerArnForIdentifier(t *testing.T) {
	t.Run("Resolves direct ARN", func(t *testing.T) {
		e := awsw.ELB{}
		ctx := context.Background()
		identifier := "arn:aws:elasticloadbalancing:us-east-1:123456789012:loadbalancer/app/my-load-balancer/50dc6c495c0c9188"
		arn, err := e.LoadBalancerArnForIdentifier(ctx, identifier)
		assert.NoError(t, err)
		assert.Equal(t, identifier, *arn)
	})

	t.Run("Returns error for invalid provider", func(t *testing.T) {
		e := awsw.ELB{}
		ctx := context.Background()
		identifier := "invalid::my-lb"
		_, err := e.LoadBalancerArnForIdentifier(ctx, identifier)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "provider \"invalid\" not found")
	})
}

func TestELBTargetGroupArnForIdentifier(t *testing.T) {
	t.Run("Resolves direct ARN", func(t *testing.T) {
		e := awsw.ELB{}
		ctx := context.Background()
		identifier := "arn:aws:elasticloadbalancing:us-east-1:123456789012:targetgroup/my-targets/73e2d6bc24d8a067"
		arn, err := e.TargetGroupArnForIdentifier(ctx, identifier)
		assert.NoError(t, err)
		assert.Equal(t, identifier, *arn)
	})
}
