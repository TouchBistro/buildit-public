package buildit_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/TouchBistro/awesome/providers"
	"github.com/TouchBistro/buildit"
	"github.com/TouchBistro/buildit/client"
	"github.com/TouchBistro/buildit/resource"
	"github.com/TouchBistro/buildit/util"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// offlineProvider builds an awesome provider that is registered by name but never
// validated, so it loads no credentials and makes no AWS call.
func offlineProvider(t *testing.T, name string) providers.CredsProvider {
	t.Helper()
	p, err := providers.NewDefaultCredsProvider(context.Background(), name)
	require.NoError(t, err)
	return p
}

// offlineDefault registers an offline `default` unless one is registered already, so
// NewConfig does not build one from the environment. The registry is process-wide, so
// the tests in this package share it.
func offlineDefault(t *testing.T) providers.CredsProvider {
	t.Helper()
	if p, err := providers.Default(); err == nil {
		return p
	}
	return offlineProvider(t, providers.DefaultCredsProviderName)
}

// The acceptance criterion for DEVOPS-9105: resources added in Go with awesome
// providers come out of NewConfig as the wired graph `buildit plan` would build, with
// the providers found in the awesome registry.
func TestNewConfig(t *testing.T) {
	ctx := context.Background()
	def := offlineDefault(t)
	other := offlineProvider(t, "example-other")

	var rs buildit.ResourceSet
	require.NoError(t, rs.AddCWLogGroup("example-logs", resource.CWLogGroup{}))
	require.NoError(t, rs.AddCWLogGroupWithProvider(other, "example-logs", resource.CWLogGroup{}))
	require.NoError(t, rs.AddCWLogGroup("example-other::example-more-logs", resource.CWLogGroup{}))

	cfg, err := buildit.NewConfig(ctx, rs,
		buildit.WithGlobalTag("team", "example-team"),
		buildit.WithGlobalTags(map[string]string{"env": "example-env"}),
		buildit.WithoutAuditTags(),
	)
	require.NoError(t, err)

	g := cfg.Graph()
	require.Equal(t, 3, g.Size())

	got, err := g.GetVertex(resource.NewKey(client.MainProvider, "example-logs"))
	require.NoError(t, err)
	lg := got.(resource.CWLogGroup)
	assert.Equal(t, client.MainProvider, lg.Context.ProviderName)
	assert.Equal(t, "example-team", lg.Tags["team"])
	assert.Equal(t, "example-env", lg.Tags["env"])
	assert.Equal(t, "example-logs", lg.Tags[util.BuilditResourceIDTagKey])

	got, err = g.GetVertex(resource.NewKey("example-other", "example-logs"))
	require.NoError(t, err)
	assert.Equal(t, "example-other", got.(resource.CWLogGroup).Context.ProviderName)

	assert.Equal(t, map[string]string{"team": "example-team", "env": "example-env"}, cfg.Tags())

	// The registered default is untouched, and nothing was written as main.
	d, err := providers.Default()
	require.NoError(t, err)
	assert.Same(t, def, d)
	_, err = providers.Get(client.MainProvider)
	assert.ErrorIs(t, err, providers.ErrUnknownProvider)
}

// `main` is resolved when a client is built, not registered: with no `main` it is the
// `default` of the moment, so re-registering `default` between two configs moves them
// both; a registered `main` wins. The subtests run in order because the registry is
// process-wide and a registered name cannot be removed.
func TestNewConfigResolvesMain(t *testing.T) {
	ctx := context.Background()
	var rs buildit.ResourceSet
	require.NoError(t, rs.AddCWLogGroup("example-logs", resource.CWLogGroup{}))

	t.Run("default stands in for main", func(t *testing.T) {
		def := offlineDefault(t)
		_, err := buildit.NewConfig(ctx, rs, buildit.WithoutAuditTags())
		require.NoError(t, err)
		m, err := client.Provider(client.MainProvider)
		require.NoError(t, err)
		assert.Same(t, def, m)
	})

	t.Run("a new default moves main", func(t *testing.T) {
		def := offlineProvider(t, providers.DefaultCredsProviderName)
		_, err := buildit.NewConfig(ctx, rs, buildit.WithoutAuditTags())
		require.NoError(t, err)
		m, err := client.Provider(client.MainProvider)
		require.NoError(t, err)
		assert.Same(t, def, m)
	})

	t.Run("a registered main wins", func(t *testing.T) {
		main := offlineProvider(t, client.MainProvider)
		_, err := buildit.NewConfig(ctx, rs, buildit.WithoutAuditTags())
		require.NoError(t, err)
		m, err := client.Provider(client.MainProvider)
		require.NoError(t, err)
		assert.Same(t, main, m)
	})
}

// AddXWithProvider takes the provider the registry holds under its name; an older one
// that a later build has replaced is refused, since the resource's clients would not use it.
func TestResourceSetRejectsReplacedProvider(t *testing.T) {
	stale := offlineProvider(t, "example-shared")
	offlineProvider(t, "example-shared")

	var rs buildit.ResourceSet
	err := rs.AddCWLogGroupWithProvider(stale, "example-logs", resource.CWLogGroup{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `provider "example-shared" is not the one registered under that name`)

	err = rs.AddCWLogGroupWithProvider(nil, "example-logs", resource.CWLogGroup{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil provider")
}

// Validation runs inside the constructor: a bad resource is an error from NewConfig.
func TestNewConfigReportsValidationErrors(t *testing.T) {
	offlineDefault(t)
	var rs buildit.ResourceSet
	require.NoError(t, rs.AddCWLogGroup("example-logs", resource.CWLogGroup{Retention: aws.Int32(2)}))

	_, err := buildit.NewConfig(context.Background(), rs, buildit.WithoutAuditTags())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed validation")
}

// Unregistered providers and reserved tag keys are rejected before any AWS call is made
// on a resource's behalf.
func TestNewConfigRejectsBadInput(t *testing.T) {
	ctx := context.Background()
	offlineDefault(t)

	var rs buildit.ResourceSet
	require.NoError(t, rs.AddCWLogGroup("example-unknown::example-logs", resource.CWLogGroup{}))
	_, err := buildit.NewConfig(ctx, rs, buildit.WithoutAuditTags())
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown provider name "example-unknown"`)

	_, err = buildit.NewConfig(ctx, buildit.ResourceSet{}, buildit.WithGlobalTag(util.BuilditTagPrefix+"owner", "example-team"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reserved")
}

// File-only options are meaningless without a config path and are reported as such.
func TestNewConfigOptionValidation(t *testing.T) {
	cases := map[string]buildit.Option{
		"WithVariable":      buildit.WithVariable("example", "value"),
		"WithVariablesFrom": buildit.WithVariablesFrom("example.vars"),
		"WithTemplates":     buildit.WithTemplates("example.tmpl"),
		"WithOverridePath":  buildit.WithOverridePath("example-override.yml"),
		"WithEnvVariables":  buildit.WithEnvVariables(),
	}
	for name, opt := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := buildit.NewConfig(context.Background(), buildit.ResourceSet{}, opt)
			require.Error(t, err)
			assert.Contains(t, err.Error(), name+" requires WithConfigPath")
		})
	}
}

// WithOverridePath accumulates like the other path options, so a second call is caught
// by the single-file rule instead of silently replacing the first.
func TestNewConfigRejectsSecondOverridePath(t *testing.T) {
	offlineDefault(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "buildit.yml")
	require.NoError(t, os.WriteFile(file, []byte("resources: {}\n"), 0o600))

	_, err := buildit.NewConfig(context.Background(), buildit.ResourceSet{},
		buildit.WithConfigPath(file),
		buildit.WithOverridePath(filepath.Join(dir, "example-override-a.yml")),
		buildit.WithOverridePath(filepath.Join(dir, "example-override-b.yml")),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "merging override config is not supported")
}

// Config files are a further source: they are rendered with the variables and merged
// with the resources built in Go, tags included.
func TestNewConfigMergesConfigFiles(t *testing.T) {
	offlineDefault(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "buildit.yml")
	require.NoError(t, os.WriteFile(file, []byte(`
globalTags:
  source: example-file
resources:
  cloudwatch-loggroup:
    example-file-logs:
      retention: ${retention}
`), 0o600))

	var rs buildit.ResourceSet
	require.NoError(t, rs.AddCWLogGroup("example-go-logs", resource.CWLogGroup{}))

	cfg, err := buildit.NewConfig(context.Background(), rs,
		buildit.WithConfigPath(file),
		buildit.WithVariable("retention", "30"),
		buildit.WithGlobalTag("team", "example-team"),
	)
	require.NoError(t, err)

	g := cfg.Graph()
	require.Equal(t, 2, g.Size())

	got, err := g.GetVertex(resource.NewKey(client.MainProvider, "example-file-logs"))
	require.NoError(t, err)
	lg := got.(resource.CWLogGroup)
	require.NotNil(t, lg.Retention)
	assert.Equal(t, int32(30), *lg.Retention)
	assert.Equal(t, "example-team", lg.Tags["team"])
	assert.Equal(t, "example-file", lg.Tags["source"])

	_, err = g.GetVertex(resource.NewKey(client.MainProvider, "example-go-logs"))
	require.NoError(t, err)

	// With file content to hash, the audit tag is derived as the CLI does.
	assert.NotEmpty(t, cfg.Tags()["audit"])
}

func TestContextWithRetryPolicy(t *testing.T) {
	ctx := buildit.ContextWithRetryPolicy(context.Background(), 5, 30)
	assert.Equal(t, 5, ctx.Value(util.RETRY))
	assert.Equal(t, 30, ctx.Value(util.BACKOFF))
}
