package config

import (
	"context"
	"testing"

	"github.com/TouchBistro/buildit/client"
	"github.com/TouchBistro/buildit/resource"
	"github.com/TouchBistro/buildit/util"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// generateOffline runs Generate's passes around the provider initialization, which
// calls STS and so cannot run in a unit test. Everything a consumer observes on the
// result — providers, tags, the wired graph — comes from these two.
func generateOffline(t *testing.T, i *InternalConfig, opts RootOptions) error {
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
func TestNewInternalConfigBuildsGraph(t *testing.T) {
	first := resource.NewKey("main", "example-logs")
	second := resource.NewKey("main", "example-logs-2")

	i := NewInternalConfig(ResourceSet{
		CWLogGroup: map[string]resource.CWLogGroup{
			"example-logs":   logGroup(""),
			"example-logs-2": logGroup("", first),
		},
	}, nil, map[string]string{"team": "example-team"})

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

// Providers passed in Go are merged the same way the providers: section is, and a
// resource may name one of them with the provider::name key form.
func TestNewInternalConfigWiresProviders(t *testing.T) {
	i := NewInternalConfig(ResourceSet{
		CWLogGroup: map[string]resource.CWLogGroup{
			"example::example-logs": logGroup(""),
		},
	}, map[string]*client.AwsProvider{
		"example": {ProviderType: client.ProviderTypeRole, AccountId: "123456789012", RoleName: "example-role"},
	}, nil)

	require.NoError(t, generateOffline(t, i, RootOptions{NoAuditTags: true}))

	require.Contains(t, i.Providers(), "example")
	got, err := i.Graph().GetVertex(resource.NewKey("example", "example-logs"))
	require.NoError(t, err)
	assert.Equal(t, "example", got.(resource.CWLogGroup).Context.ProviderName)
}

func TestNewInternalConfigRejectsUnknownProvider(t *testing.T) {
	i := NewInternalConfig(ResourceSet{
		CWLogGroup: map[string]resource.CWLogGroup{"example-logs": logGroup("other")},
	}, nil, nil)

	err := generateOffline(t, i, RootOptions{NoAuditTags: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown provider name "other"`)
}

// Validation runs for a Go-built config exactly as for a parsed one.
func TestNewInternalConfigReportsValidationErrors(t *testing.T) {
	bad := logGroup("")
	bad.Retention = aws.Int32(2)
	i := NewInternalConfig(ResourceSet{
		CWLogGroup: map[string]resource.CWLogGroup{"example-logs": bad},
	}, nil, nil)

	err := generateOffline(t, i, RootOptions{NoAuditTags: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed validation")
}

// The reserved-tag check happens before provider initialization, so the public Generate
// path fails fast on a bad globalTags map with no AWS call.
func TestNewInternalConfigRejectsReservedGlobalTags(t *testing.T) {
	i := NewInternalConfig(ResourceSet{}, nil, map[string]string{util.BuilditTagPrefix + "owner": "example-team"})

	err := i.Generate(context.Background(), RootOptions{NoAuditTags: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reserved")
}

// Without file content there is nothing to hash; the audit tag is still attached, and a
// consumer-supplied value wins, matching the behaviour for parsed configs.
func TestNewInternalConfigAuditTag(t *testing.T) {
	t.Run("derived when absent", func(t *testing.T) {
		i := NewInternalConfig(ResourceSet{}, nil, nil)
		require.NoError(t, generateOffline(t, i, RootOptions{}))
		assert.NotEmpty(t, i.Tags()["audit"])
	})

	t.Run("kept when supplied", func(t *testing.T) {
		i := NewInternalConfig(ResourceSet{}, nil, map[string]string{"audit": "example-hash"})
		require.NoError(t, generateOffline(t, i, RootOptions{}))
		assert.Equal(t, "example-hash", i.Tags()["audit"])
	})
}
