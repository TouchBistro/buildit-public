package buildit_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/TouchBistro/awesome/providers"
	"github.com/TouchBistro/buildit"
	"github.com/TouchBistro/buildit/config"
	"github.com/TouchBistro/buildit/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubLogs is a local server that plays CloudWatch Logs with no log groups in it, and
// the provider whose clients talk to it. Both live for the whole test binary: a provider
// name cannot be unregistered and buildit caches the clients it builds for one, so a
// server per test run would leave later runs calling a closed one.
var stubLogs struct {
	once     sync.Once
	provider providers.CredsProvider
	calls    atomic.Int64
}

// stubLogsProvider returns that provider, so a log group can be planned without
// credentials or network, and the counter of requests the server has served.
func stubLogsProvider(t *testing.T) (providers.CredsProvider, *atomic.Int64) {
	t.Helper()
	stubLogs.once.Do(func() {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			stubLogs.calls.Add(1)
			w.Header().Set("Content-Type", "application/x-amz-json-1.1")
			_, _ = w.Write([]byte(`{"logGroups":[]}`))
		}))

		// The provider reads these when it is built.
		t.Setenv("AWS_ENDPOINT_URL", srv.URL)
		t.Setenv("AWS_REGION", "us-east-1")
		t.Setenv("AWS_ACCESS_KEY_ID", "AKIAIOSFODNN7EXAMPLE")
		t.Setenv("AWS_SECRET_ACCESS_KEY", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")
		t.Setenv("AWS_SESSION_TOKEN", "")
		t.Setenv("AWS_PROFILE", "")
		t.Setenv("AWS_CONFIG_FILE", t.TempDir()+"/config")
		t.Setenv("AWS_SHARED_CREDENTIALS_FILE", t.TempDir()+"/credentials")

		stubLogs.provider = offlineProvider(t, "example-plan")
	})
	return stubLogs.provider, &stubLogs.calls
}

// The acceptance criterion for DEVOPS-8930: two resources and a dependency between them,
// added in Go with no file on disk, come out of NewConfig as a wired graph that plans.
// The plan runs each log group's real Compare against the stub.
func TestNewConfigGraphPlans(t *testing.T) {
	ctx := context.Background()
	offlineDefault(t)
	p, calls := stubLogsProvider(t)
	before := calls.Load()

	first := resource.NewKey("example-plan", "example-first")
	second := resource.NewKey("example-plan", "example-second")

	var rs buildit.ResourceSet
	// Added out of order: the dependency, not the insertion order, decides the plan.
	require.NoError(t, rs.AddCWLogGroupWithProvider(p, "example-second", resource.CWLogGroup{DependsOn: []resource.Key{first}}))
	require.NoError(t, rs.AddCWLogGroupWithProvider(p, "example-first", resource.CWLogGroup{}))

	cfg, err := buildit.NewConfig(ctx, rs, buildit.WithoutAuditTags())
	require.NoError(t, err)

	entries, err := cfg.Graph().PlanResult(ctx, nil)
	require.NoError(t, err)
	require.Len(t, entries, 2)
	assert.Equal(t, first, entries[0].Key, "the dependency is planned first")
	assert.Equal(t, second, entries[1].Key)
	for _, e := range entries {
		assert.Equal(t, config.PlanCreate, e.Action, e.Key)
	}
	assert.Equal(t, int64(2), calls.Load()-before, "each log group was looked up once")
}
