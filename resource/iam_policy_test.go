package resource

import (
	"context"
	"testing"

	"github.com/TouchBistro/buildit/util"
)

func TestCreatePolicy(t *testing.T) {
	policy := IAMPolicy{
		Name:        "buildit/buildit-policy",
		Description: "test buildit policy",
		Policy: IAMPolicyDocument{
			Version: "2012-10-17",
			Statement: []IAMPolicyStatement{
				{
					Effect: "Allow",
					Action: []string{
						"appmesh:StreamAggregatedResources",
					},
					Resource: "*",
				},
			},
		},
	}
	ctx := context.Background()
	ctx = context.WithValue(ctx, util.RETRY, 3)
	ctx = context.WithValue(ctx, util.BACKOFF, 20)
	err := policy.Apply(ctx)
	if err != nil {
		t.Error(err.Error())
	}
}
