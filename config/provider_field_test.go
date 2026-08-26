package config

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/TouchBistro/buildit/client"
	"github.com/TouchBistro/buildit/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

// TestResourceProviderFieldIsParsed guards the `provider` field wiring for every
// resource type in resourcesConfig: BaseResource must be embedded with yaml:",inline",
// or yaml.v2 silently drops the key (unlike encoding/json, it does not auto-inline
// embedded structs) and the resource silently falls back to the main provider.
func TestResourceProviderFieldIsParsed(t *testing.T) {
	rcType := reflect.TypeFor[resourcesConfig]()

	for i := range rcType.NumField() {
		field := rcType.Field(i)
		tag := field.Tag.Get("yaml")
		require.NotEmpty(t, tag, "resourcesConfig field %q has no yaml tag", field.Name)

		t.Run(tag, func(t *testing.T) {
			doc := fmt.Sprintf("resources:\n  %s:\n    test-item:\n      provider: custom-provider\n", tag)

			var cfg builditConfig
			require.NoError(t, yaml.Unmarshal([]byte(doc), &cfg))

			item := reflect.ValueOf(cfg.Resources).Field(i).MapIndex(reflect.ValueOf("test-item"))
			require.True(t, item.IsValid(), "resource %q did not unmarshal", tag)

			ctx := item.FieldByName("Context")
			require.True(t, ctx.IsValid(), "resource %q does not embed BaseResource", tag)

			assert.Equal(t, "custom-provider", ctx.FieldByName("ProviderName").String(),
				"provider field was not parsed — is BaseResource embedded with yaml:\",inline\"?")
		})
	}
}

// TestGetEffectiveProviderAndId_FieldValidation covers the provider-field paths of
// getEffectiveProviderAndId, which were unreachable while the field was silently
// dropped: unknown providers are rejected, and a provider supplied both via the
// resource-name prefix and the field must be consistent.
func TestGetEffectiveProviderAndId_FieldValidation(t *testing.T) {
	i := InternalConfig{providers: map[string]*client.AwsProvider{"known": {}}}

	t.Run("field provider used", func(t *testing.T) {
		pr, id, err := i.getEffectiveProviderAndId("my-res", resource.Context{ProviderName: "known"})
		require.NoError(t, err)
		assert.Equal(t, "known", *pr)
		assert.Equal(t, "my-res", *id)
	})

	t.Run("unknown field provider rejected", func(t *testing.T) {
		_, _, err := i.getEffectiveProviderAndId("my-res", resource.Context{ProviderName: "unused"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), `unknown provider name "unused"`)
	})

	t.Run("name prefix and field must agree", func(t *testing.T) {
		_, _, err := i.getEffectiveProviderAndId("other::my-res", resource.Context{ProviderName: "known"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ambiguous provider definition")
	})

	t.Run("empty field falls back to main", func(t *testing.T) {
		pr, _, err := i.getEffectiveProviderAndId("my-res", resource.Context{})
		require.NoError(t, err)
		assert.Equal(t, client.MainProvider, *pr)
	})
}
