package resource

import (
	"context"
	"testing"

	"github.com/TouchBistro/buildit/util"
)

func TestCreateRole(t *testing.T) {
	role := IAMRole{
		Name:        "buildit-role",
		Description: "test buildit role",
		TrustPolicy: IAMPolicyDocument{
			Version: "2012-10-17",
			Statement: []IAMPolicyStatement{
				{
					Sid:    "",
					Effect: "Allow",
					Principal: map[string]interface{}{
						"Service": "ecs-tasks.amazonaws.com",
					},
					Action: "sts:AssumeRole",
				},
			},
		},
		MaxSessionDuration: 3600,
		Path:               "/buildit/",
	}
	ctx := context.Background()
	ctx = context.WithValue(ctx, util.RETRY, 3)
	ctx = context.WithValue(ctx, util.BACKOFF, 20)
	err := role.Apply(ctx)
	if err != nil {
		t.Error(err.Error())
	}
}
