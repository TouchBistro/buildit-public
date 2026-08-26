package resource

import (
	"context"
	"testing"

	"github.com/TouchBistro/buildit/util"
	"github.com/aws/aws-sdk-go-v2/aws"
)

func TestApplyFargate(t *testing.T) {
	svc := ECSService{
		Name:       "venue.sync.service-buildit",
		LaunchType: aws.String(Fargate),
	}
	ctx := context.Background()
	ctx = context.WithValue(ctx, util.RETRY, 3)
	ctx = context.WithValue(ctx, util.BACKOFF, 20)
	err := svc.Apply(ctx)
	if err == nil {
		t.Error("Want non null err, got nil")
	}
}

func TestApplyEc2(t *testing.T) {
	svc := ECSService{
		Name:              "service-buildit",
		ClusterName:       "cluster-buildit",
		TaskDefName:       "taskdef-buildit", //optional include :version
		ServiceType:       aws.String(Replica),
		LaunchType:        aws.String(EC2),
		DesiredCount:      aws.Int32(0),
		MinHealthyPercent: aws.Int32(100),
		MaxPercent:        aws.Int32(200),
		LoadBalancing: ECSServiceLoadBalancing{
			HealthcheckGracePeriod: 5,
			TargetGroupAssignments: []ECSServiceTargetGroupAssignment{
				{
					ContainerName:   "container-buildit",
					ContainerPort:   8080,
					TargetGroupName: "tg-buildit-pub",
				},
				{
					ContainerName:   "container-buildit",
					ContainerPort:   8080,
					TargetGroupName: "tg-buildit-pvt",
				},
			},
		},
	}
	ctx := context.Background()
	ctx = context.WithValue(ctx, util.RETRY, 3)
	ctx = context.WithValue(ctx, util.BACKOFF, 20)
	err := svc.Apply(ctx)
	if err != nil {
		t.Error("want non-null, got nil")
	}
}

func TestUpdate(t *testing.T) {
	svc := ECSService{
		Name:       "some-existing-service",
		LaunchType: aws.String(EC2),
	}
	ctx := context.Background()
	ctx = context.WithValue(ctx, util.RETRY, 3)
	ctx = context.WithValue(ctx, util.BACKOFF, 20)
	err := svc.Apply(ctx)
	if err != nil {
		t.Error("Want nil, got error!")
	}
}
