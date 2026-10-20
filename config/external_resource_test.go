package config_test

import (
	"context"
	"strings"
	"testing"

	"github.com/TouchBistro/buildit/config"
	"github.com/TouchBistro/buildit/resource"
	log "github.com/sirupsen/logrus"
	logtest "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// externalResource is a resource type that lives outside the resource package,
// the way wafer's CloudFront Function and WAFv2 types will. It follows the
// receiver rule: Normalize on the pointer, everything else on the value.
type externalResource struct {
	resource.BaseResource
	Name       string
	Enabled    *bool
	normalized bool
	compared   *bool
}

func (e externalResource) Key() resource.Key  { return resource.NewKey(e.Context.ProviderName, e.Name) }
func (e externalResource) Identifier() string { return e.Name }

func (e *externalResource) Normalize(context.Context) {
	if e.Enabled == nil {
		enabled := true
		e.Enabled = &enabled
	}
	e.normalized = true
}

func (e externalResource) Validate(context.Context) error {
	if e.Name == "" {
		return &resource.ValidationError{ResourceType: "external", Messages: []string{"name is required"}}
	}
	return nil
}

func (e externalResource) Apply(context.Context) error   { return nil }
func (e externalResource) Destroy(context.Context) error { return nil }

func (e externalResource) Compare(context.Context) (resource.ResourceDiff, error) {
	if e.compared != nil {
		*e.compared = true
	}
	return &resource.BaseResourceDiff{Messages: []string{"does not exist"}}, nil
}

// The pointer form satisfies every lifecycle interface a generic builder needs.
var (
	_ resource.NormalizableResource = (*externalResource)(nil)
	_ resource.ValidatableResource  = (*externalResource)(nil)
	_ resource.ComparableResource   = (*externalResource)(nil)
)

// TestExternalResourceJoinsGraph drives a resource defined in another package
// through the lifecycle entirely via interfaces, as a generic builder would,
// then plans it as a graph vertex.
func TestExternalResourceJoinsGraph(t *testing.T) {
	ctx := context.Background()
	compared := false
	ext := &externalResource{
		BaseResource: resource.BaseResource{Context: resource.Context{ProviderName: "main"}},
		Name:         "example-external",
		compared:     &compared,
	}

	var n resource.NormalizableResource = ext
	n.Normalize(ctx)
	assert.True(t, ext.normalized, "Normalize must reach the original value through the pointer")
	require.NotNil(t, ext.Enabled)

	var v resource.ValidatableResource = ext
	require.NoError(t, v.Validate(ctx))

	g := &config.Graph{}
	require.NoError(t, g.AddVertex(ext, nil))

	hook := logtest.NewGlobal()
	defer hook.Reset()
	prev := log.GetLevel()
	log.SetLevel(log.InfoLevel)
	defer log.SetLevel(prev)

	require.NoError(t, g.Plan(ctx))
	assert.True(t, compared, "Plan must call Compare on the external resource")

	// Messages carry colour escapes, so match on the text.
	var planned bool
	for _, e := range hook.AllEntries() {
		planned = planned || strings.Contains(e.Message, "new resource will be provisioned")
	}
	assert.True(t, planned, "Plan must report the external resource as a create")
}

func TestExternalResourceValidateReportsThroughInterface(t *testing.T) {
	var v resource.ValidatableResource = &externalResource{}
	err := v.Validate(context.Background())
	var verr *resource.ValidationError
	require.ErrorAs(t, err, &verr)
	assert.Equal(t, []string{"name is required"}, verr.Messages)
}
