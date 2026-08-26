package resource

import (
	"context"
	"testing"

	"github.com/TouchBistro/buildit/util"
)

func TestApplySimpleFail(t *testing.T) {
	svc := SDService{
		Name:          "foo-sd",
		DiscoveryName: "foo4",
		Description:   "test food service",
		Namespace:     "svc.local",
	}
	ctx := context.Background()
	ctx = context.WithValue(ctx, util.RETRY, 3)
	ctx = context.WithValue(ctx, util.BACKOFF, 20)
	err := svc.Apply(ctx)
	if err != nil {
		t.Error("want non-null, got nil")
	}

}
