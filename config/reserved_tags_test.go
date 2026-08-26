package config

import (
	"testing"

	"github.com/TouchBistro/buildit/resource"
	"github.com/TouchBistro/buildit/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func parseConfig(t *testing.T, doc string) builditConfig {
	t.Helper()

	var cfg builditConfig
	require.NoError(t, yaml.Unmarshal([]byte(doc), &cfg))
	cfg.Source = "example.yml"

	return cfg
}

// The whole namespace is reserved, not just the keys buildit writes today, so these cases
// deliberately include keys buildit does not set — an implementation that only knew about
// buildit:resource-id would pass a suite built solely around that key.
func TestCheckReservedTags(t *testing.T) {
	tests := []struct {
		name string
		tags map[string]string
		msgs []string
	}{
		{name: "nil tags", tags: nil},
		{name: "ordinary tags", tags: map[string]string{"team": "example-team", "iac:created-with": "buildit"}},
		{
			// Contains the prefix but is not prefixed with it.
			name: "prefix lookalike",
			tags: map[string]string{"x-buildit:owner": "example-team"},
		},
		{
			name: "the key buildit writes",
			tags: map[string]string{util.BuilditResourceIDTagKey: "not-mine"},
			msgs: []string{`tag key "buildit:resource-id" uses the reserved "buildit:" prefix`},
		},
		{
			name: "a key buildit does not write",
			tags: map[string]string{"buildit:owner": "example-team"},
			msgs: []string{`tag key "buildit:owner" uses the reserved "buildit:" prefix`},
		},
		{
			name: "several offenders are reported in a stable order",
			tags: map[string]string{"buildit:owner": "a", "buildit:cost": "b", "team": "c"},
			msgs: []string{
				`tag key "buildit:cost" uses the reserved "buildit:" prefix`,
				`tag key "buildit:owner" uses the reserved "buildit:" prefix`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkReservedTags("s3-bucket", "example-bucket", tt.tags)

			if tt.msgs == nil {
				assert.NoError(t, err)
				return
			}

			var verr *resource.ValidationError
			require.ErrorAs(t, err, &verr)
			assert.Equal(t, "s3-bucket", verr.ResourceType)
			assert.Equal(t, "example-bucket", verr.ResourceIdentifier)
			assert.Equal(t, tt.msgs, verr.Messages)
		})
	}
}

// This is what catches a resource type whose Generate block was never wired to tagsFor.
func TestCheckBuilditTagsApplied(t *testing.T) {
	t.Run("wired", func(t *testing.T) {
		tags := map[string]string{util.BuilditResourceIDTagKey: "example-bucket"}
		assert.NoError(t, checkBuilditTagsApplied("s3-bucket", "example-bucket", tags))
	})

	t.Run("not wired", func(t *testing.T) {
		err := checkBuilditTagsApplied("s3-bucket", "example-bucket", map[string]string{"team": "example-team"})

		var verr *resource.ValidationError
		require.ErrorAs(t, err, &verr)
		assert.Equal(t, "s3-bucket", verr.ResourceType)
		assert.Contains(t, verr.Messages[0], "was not applied")
		assert.Contains(t, verr.Messages[0], "tagsFor")
	})

	t.Run("nil tags", func(t *testing.T) {
		assert.Error(t, checkBuilditTagsApplied("s3-bucket", "example-bucket", nil))
	})
}

func TestCheckReservedGlobalTags(t *testing.T) {
	t.Run("rejects a reserved key and names the source file", func(t *testing.T) {
		cfg := parseConfig(t, `
globalTags:
  buildit:owner: example-team
`)
		i := InternalConfig{_config: []builditConfig{cfg}}

		err := i.checkReservedGlobalTags()
		require.Error(t, err)
		assert.Contains(t, err.Error(), `1 globalTags key(s) use the reserved "buildit:" prefix`)
	})

	t.Run("allows ordinary keys, including the legacy audit tag", func(t *testing.T) {
		cfg := parseConfig(t, `
globalTags:
  iac:created-with: buildit
  audit: example-hash
  x-buildit:owner: example-team
`)
		i := InternalConfig{_config: []builditConfig{cfg}}

		assert.NoError(t, i.checkReservedGlobalTags())
	})

	t.Run("covers the override config", func(t *testing.T) {
		override := parseConfig(t, `
globalTags:
  buildit:owner: example-team
`)
		i := InternalConfig{_override: &override}

		assert.Error(t, i.checkReservedGlobalTags())
	})

	t.Run("counts offenders across every collected config", func(t *testing.T) {
		one := parseConfig(t, "globalTags:\n  buildit:owner: a\n")
		two := parseConfig(t, "globalTags:\n  buildit:cost: b\n  buildit:team: c\n")
		i := InternalConfig{_config: []builditConfig{one, two}}

		err := i.checkReservedGlobalTags()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "3 globalTags key(s)")
	})
}
