package config

import (
	"context"
	"reflect"
	"testing"

	"github.com/TouchBistro/awesome/providers"
	"github.com/TouchBistro/buildit/resource"
	"github.com/TouchBistro/buildit/util"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// generateOffline runs Generate's passes around the provider initialization, which
// calls STS and so cannot run in a unit test. Everything a consumer observes on the
// result — providers, tags, the wired graph — comes from these two.
func generateOffline(t *testing.T, i *Config, opts RootOptions) error {
	t.Helper()
	if err := i.mergeConfigs(opts); err != nil {
		return err
	}
	return i.buildGraph(context.Background())
}

func logGroup(provider string, dependsOn ...resource.Key) resource.CWLogGroup {
	return resource.CWLogGroup{
		BaseResource: resource.BaseResource{Context: resource.Context{ProviderName: provider}},
		DependsOn:    dependsOn,
	}
}

// The acceptance criterion for DEVOPS-8933: a ResourceSet built in Go, with no file on
// disk, comes out of Generate as the same wired graph a parsed buildit.yml would.
func TestNewConfigBuildsGraph(t *testing.T) {
	first := resource.NewKey("main", "example-logs")
	second := resource.NewKey("main", "example-logs-2")

	i := NewConfig(ResourceSet{
		CWLogGroup: map[string]resource.CWLogGroup{
			"example-logs":   logGroup(""),
			"example-logs-2": logGroup("", first),
		},
	}, map[string]string{"team": "example-team"})

	require.NoError(t, generateOffline(t, i, RootOptions{NoAuditTags: true}))

	g := i.Graph()
	require.NotNil(t, g)
	require.Equal(t, 2, g.Size())

	got, err := g.GetVertex(first)
	require.NoError(t, err)
	lg, ok := got.(resource.CWLogGroup)
	require.True(t, ok, "vertex holds %T", got)

	// Name and provider are filled in from the map key, as for a parsed config.
	assert.Equal(t, "example-logs", lg.Name)
	assert.Equal(t, "main", lg.Context.ProviderName)
	// Normalize ran: the retention default is set and global tags reached the resource.
	require.NotNil(t, lg.Retention)
	assert.Equal(t, "example-team", lg.Tags["team"])
	assert.Equal(t, "example-logs", lg.Tags[util.BuilditResourceIDTagKey])
	// Per-resource ids must not leak into the shared global tags.
	assert.Equal(t, map[string]string{"team": "example-team"}, i.Tags())

	// dependsOn became an edge: first -> second.
	assert.Contains(t, g.Vertices()[first].adjacentVertices, second)
	assert.Equal(t, 1, g.Vertices()[second].inDegree)
}

// offlineProvider builds an awesome provider that is registered by name but never
// validated, so it loads no credentials and makes no AWS call.
func offlineProvider(t *testing.T, name string) providers.CredsProvider {
	t.Helper()
	p, err := providers.NewDefaultCredsProvider(context.Background(), name)
	require.NoError(t, err)
	return p
}

// A provider registered with the awesome package is known to the config the way a
// providers: entry is, and a resource may name it with the provider::name key form; a
// name that is neither is rejected while the graph is built.
func TestNewConfigAcceptsRegisteredProviders(t *testing.T) {
	offlineProvider(t, "example")
	i := NewConfig(ResourceSet{
		CWLogGroup: map[string]resource.CWLogGroup{
			"example::example-logs": logGroup(""),
		},
	}, nil)
	require.NoError(t, generateOffline(t, i, RootOptions{NoAuditTags: true}))

	// Not a providers: entry, so not in the map Generate initializes.
	assert.NotContains(t, i.Providers(), "example")
	got, err := i.Graph().GetVertex(resource.NewKey("example", "example-logs"))
	require.NoError(t, err)
	assert.Equal(t, "example", got.(resource.CWLogGroup).Context.ProviderName)

	i = NewConfig(ResourceSet{
		CWLogGroup: map[string]resource.CWLogGroup{
			"example-unregistered::example-logs": logGroup(""),
		},
	}, nil)
	err = generateOffline(t, i, RootOptions{NoAuditTags: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown provider name "example-unregistered"`)
}

func TestNewConfigRejectsUnknownProvider(t *testing.T) {
	i := NewConfig(ResourceSet{
		CWLogGroup: map[string]resource.CWLogGroup{"example-logs": logGroup("other")},
	}, nil)

	err := generateOffline(t, i, RootOptions{NoAuditTags: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown provider name "other"`)
}

// Validation runs for a Go-built config exactly as for a parsed one.
func TestNewConfigReportsValidationErrors(t *testing.T) {
	bad := logGroup("")
	bad.Retention = aws.Int32(2)
	i := NewConfig(ResourceSet{
		CWLogGroup: map[string]resource.CWLogGroup{"example-logs": bad},
	}, nil)

	err := generateOffline(t, i, RootOptions{NoAuditTags: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed validation")
}

// The reserved-tag check happens before provider initialization, so the public Generate
// path fails fast on a bad globalTags map with no AWS call.
func TestNewConfigRejectsReservedGlobalTags(t *testing.T) {
	i := NewConfig(ResourceSet{}, map[string]string{util.BuilditTagPrefix + "owner": "example-team"})

	err := i.Generate(context.Background(), RootOptions{NoAuditTags: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reserved")
}

// Without file content there is nothing to hash; the audit tag is still attached, and a
// consumer-supplied value wins, matching the behaviour for parsed configs.
func TestNewConfigAuditTag(t *testing.T) {
	t.Run("derived when absent", func(t *testing.T) {
		i := NewConfig(ResourceSet{}, nil)
		require.NoError(t, generateOffline(t, i, RootOptions{}))
		assert.NotEmpty(t, i.Tags()["audit"])
	})

	t.Run("kept when supplied", func(t *testing.T) {
		i := NewConfig(ResourceSet{}, map[string]string{"audit": "example-hash"})
		require.NoError(t, generateOffline(t, i, RootOptions{}))
		assert.Equal(t, "example-hash", i.Tags()["audit"])
	})
}

// The Add methods are how a Go consumer fills a ResourceSet (DEVOPS-9105).
func TestResourceSetAdd(t *testing.T) {
	t.Run("stores under the name and allocates the map", func(t *testing.T) {
		var rs ResourceSet
		require.NoError(t, rs.AddCWLogGroup("example-logs", logGroup("")))
		require.NoError(t, rs.AddCWLogGroup("other::example-logs", logGroup("")))
		assert.Len(t, rs.CWLogGroup, 2)
	})

	t.Run("rejects a duplicate name for the type", func(t *testing.T) {
		var rs ResourceSet
		require.NoError(t, rs.AddCWLogGroup("example-logs", logGroup("")))
		err := rs.AddCWLogGroup("example-logs", logGroup(""))
		require.Error(t, err)
		assert.Contains(t, err.Error(), `cloudwatch-loggroup "example-logs" is already defined`)
		// The same name under another type is a different resource.
		require.NoError(t, rs.AddS3Bucket("example-logs", resource.S3Bucket{}))
	})

	t.Run("with provider keys by provider::name and records the provider", func(t *testing.T) {
		var rs ResourceSet
		p := offlineProvider(t, "example-add")
		require.NoError(t, rs.AddCWLogGroupWithProvider(p, "example-logs", logGroup("")))
		require.Contains(t, rs.CWLogGroup, "example-add::example-logs")
	})

	t.Run("with provider rejects a nil provider and a prefixed name", func(t *testing.T) {
		var rs ResourceSet
		require.Error(t, rs.AddCWLogGroupWithProvider(nil, "example-logs", logGroup("")))
		err := rs.AddCWLogGroupWithProvider(offlineProvider(t, "example-pfx"), "other::example-logs", logGroup(""))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must not contain")
	})

	// The provider must be the one the registry holds under its name: a provider built
	// later with the same name has replaced it there, and the resource's clients would
	// use that one, not the one the caller passed.
	t.Run("with provider rejects a replaced provider", func(t *testing.T) {
		var rs ResourceSet
		stale := offlineProvider(t, "example-replaced")
		current := offlineProvider(t, "example-replaced")
		err := rs.AddCWLogGroupWithProvider(stale, "example-logs", logGroup(""))
		require.Error(t, err)
		assert.Contains(t, err.Error(), `provider "example-replaced" is not the one registered under that name`)
		require.NoError(t, rs.AddCWLogGroupWithProvider(current, "example-logs", logGroup("")))
	})
}

// A resource added with a provider resolves to that provider in the generated graph.
func TestNewConfigUsesResourceSetProviders(t *testing.T) {
	var rs ResourceSet
	p := offlineProvider(t, "example-rs")
	require.NoError(t, rs.AddCWLogGroupWithProvider(p, "example-logs", logGroup("")))

	i := NewConfig(rs, nil)
	require.NoError(t, generateOffline(t, i, RootOptions{NoAuditTags: true}))

	got, err := i.Graph().GetVertex(resource.NewKey("example-rs", "example-logs"))
	require.NoError(t, err)
	assert.Equal(t, "example-rs", got.(resource.CWLogGroup).Context.ProviderName)
}

// Every resource type in ResourceSet must have an AddX(name, r) and an
// AddXWithProvider(p, name, r) method with the right signature, so a type added to the
// struct cannot be left out of the Go API.
func TestResourceSetAddMethodsCoverEveryType(t *testing.T) {
	st := reflect.TypeFor[ResourceSet]()
	ptr := reflect.TypeFor[*ResourceSet]()
	errT := reflect.TypeFor[error]()
	strT := reflect.TypeFor[string]()
	provT := reflect.TypeFor[providers.CredsProvider]()

	for i := range st.NumField() {
		f := st.Field(i)
		if !f.IsExported() || f.Type.Kind() != reflect.Map {
			continue
		}
		elem := f.Type.Elem()

		t.Run(f.Name, func(t *testing.T) {
			m, ok := ptr.MethodByName("Add" + f.Name)
			require.True(t, ok, "missing Add%s", f.Name)
			// receiver, name, resource -> error
			require.Equal(t, 3, m.Type.NumIn())
			assert.Equal(t, strT, m.Type.In(1))
			assert.Equal(t, elem, m.Type.In(2))
			require.Equal(t, 1, m.Type.NumOut())
			assert.Equal(t, errT, m.Type.Out(0))

			m, ok = ptr.MethodByName("Add" + f.Name + "WithProvider")
			require.True(t, ok, "missing Add%sWithProvider", f.Name)
			// receiver, provider, name, resource -> error
			require.Equal(t, 4, m.Type.NumIn())
			assert.Equal(t, provT, m.Type.In(1))
			assert.Equal(t, strT, m.Type.In(2))
			assert.Equal(t, elem, m.Type.In(3))
			require.Equal(t, 1, m.Type.NumOut())
			assert.Equal(t, errT, m.Type.Out(0))
		})
	}
}
