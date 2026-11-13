package client

import (
	"context"
	"testing"

	"github.com/TouchBistro/awesome/providers"
	"github.com/TouchBistro/buildit/util"
	log "github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A context without retry values, as a library consumer would pass, gets the CLI's
// defaults instead of a panic; values on the context win.
func TestRetryerFor(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		r := retryerFor(context.Background())
		assert.Equal(t, DefaultRetries, r.MaxAttempts())
	})

	t.Run("from context", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), util.RETRY, 5)
		ctx = context.WithValue(ctx, util.BACKOFF, 30)
		r := retryerFor(ctx)
		assert.Equal(t, 5, r.MaxAttempts())
	})

	// A value of the wrong type is a caller bug: fall back to the default, but say so.
	t.Run("mistyped value warns", func(t *testing.T) {
		hook := test.NewGlobal()
		defer hook.Reset()

		ctx := context.WithValue(context.Background(), util.RETRY, "5")
		r := retryerFor(ctx)
		assert.Equal(t, DefaultRetries, r.MaxAttempts())

		require.Len(t, hook.Entries, 1)
		assert.Equal(t, log.WarnLevel, hook.LastEntry().Level)
		assert.Equal(t, "string", hook.LastEntry().Data["type"])
	})
}

// An unknown provider name is an error a library consumer can report, never a process
// exit (DEVOPS-8926). The accessors forward it unchanged.
func TestUnknownProviderIsAnError(t *testing.T) {
	_, err := Provider("example-missing")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `provider "example-missing" not found`)
	assert.ErrorIs(t, err, providers.ErrUnknownProvider)

	ctx := context.Background() // no retry values either: nothing here may panic
	assert.NotPanics(t, func() {
		c, err := ELB(ctx, "example-missing")
		assert.Nil(t, c)
		assert.ErrorIs(t, err, providers.ErrUnknownProvider)
		g, err := ACMGlobal(ctx, "example-missing")
		assert.Nil(t, g)
		assert.ErrorIs(t, err, providers.ErrUnknownProvider)
	})
}

// offlineProvider builds an awesome provider that is registered by name but never
// validated, so it loads no credentials and makes no AWS call.
func offlineProvider(t *testing.T, name string) providers.CredsProvider {
	t.Helper()
	p, err := providers.NewDefaultCredsProvider(context.Background(), name)
	require.NoError(t, err)
	return p
}

// Provider is the one place names are resolved: `main` falls back to `default` until a
// `main` is registered, and any other name must be registered as is. The subtests run in
// order because the registry is process-wide and a registered name cannot be removed.
func TestProvider(t *testing.T) {
	t.Run("unregistered name", func(t *testing.T) {
		_, err := Provider("example-missing")
		require.ErrorIs(t, err, providers.ErrUnknownProvider)
		assert.Contains(t, err.Error(), `AWS provider "example-missing" not found`)
	})

	t.Run("main without default", func(t *testing.T) {
		if _, err := providers.Default(); err == nil {
			t.Skip("a default provider is already registered in this process")
		}
		_, err := Provider(MainProvider)
		require.ErrorIs(t, err, providers.ErrUnknownProvider)
		assert.Contains(t, err.Error(), `AWS provider "main" not found: no "main" is registered and there is no "default" to fall back to`)
	})

	def := offlineProvider(t, DefaultProvider)

	t.Run("registered name", func(t *testing.T) {
		p, err := Provider(DefaultProvider)
		require.NoError(t, err)
		assert.Same(t, def, p)
	})

	t.Run("main resolves to default", func(t *testing.T) {
		p, err := Provider(MainProvider)
		require.NoError(t, err)
		assert.Same(t, def, p)
	})

	t.Run("ensure keeps a registered default", func(t *testing.T) {
		require.NoError(t, EnsureDefaultProvider(context.Background()))
		p, err := providers.Default()
		require.NoError(t, err)
		assert.Same(t, def, p)
	})

	main := offlineProvider(t, MainProvider)

	t.Run("registered main wins", func(t *testing.T) {
		p, err := Provider(MainProvider)
		require.NoError(t, err)
		assert.Same(t, main, p)
	})
}

// CheckProviderRegistered compares identity, not just the name: a provider built later
// under the same name replaces the earlier one in the registry.
func TestCheckProviderRegistered(t *testing.T) {
	require.Error(t, CheckProviderRegistered(nil))

	stale := offlineProvider(t, "example-replaced")
	require.NoError(t, CheckProviderRegistered(stale))

	current := offlineProvider(t, "example-replaced")
	require.NoError(t, CheckProviderRegistered(current))
	err := CheckProviderRegistered(stale)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `provider "example-replaced" is not the one registered under that name`)
}

// A provider declared in a config replaces a registered one of the same name with a
// warning, unless the registered one came from a config too. The config here has no
// credentials to load, so the init fails after the check, which is all the test needs.
func TestInitNamedProvidersWarnsOnReplace(t *testing.T) {
	hook := test.NewGlobal()
	defer hook.Reset()

	offlineProvider(t, "example-declared")
	cfgs := map[string]*AwsProvider{"example-declared": {ProviderType: ProviderTypeRole}}
	err := InitNamedProviders(context.Background(), cfgs)
	require.Error(t, err) // no role parameters: fails before any AWS call

	var warned bool
	for _, e := range hook.AllEntries() {
		warned = warned || (e.Level == log.WarnLevel && e.Data["provider"] == "example-declared")
	}
	assert.True(t, warned, "expected a warning that the config provider replaces the registered one")
}
