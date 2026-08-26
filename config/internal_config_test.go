package config

import (
	"testing"

	"github.com/TouchBistro/buildit/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTagsForAddsResourceID(t *testing.T) {
	i := InternalConfig{globalTags: map[string]string{"team": "example-team"}}

	assert.Equal(t, map[string]string{
		"team":                       "example-team",
		util.BuilditResourceIDTagKey: "example-bucket",
	}, i.tagsFor("example-bucket"))
}

// Every resource gets its own map. Sharing one would mean the last resource processed
// decided everybody's id.
func TestTagsForReturnsIndependentMaps(t *testing.T) {
	i := InternalConfig{globalTags: map[string]string{"team": "example-team"}}

	first := i.tagsFor("example-bucket")
	second := i.tagsFor("other-example-bucket")

	assert.Equal(t, "example-bucket", first[util.BuilditResourceIDTagKey])
	assert.Equal(t, "other-example-bucket", second[util.BuilditResourceIDTagKey])

	first["team"] = "changed"
	assert.Equal(t, "example-team", second["team"], "resource tag maps share backing storage")
}

// `buildit audit` turns every key of globalTags into a tag filter and ANDs them, so a
// per-resource value leaking into that map would filter on one resource's id and match
// nothing, in every account.
func TestTagsForLeavesGlobalTagsUntouched(t *testing.T) {
	i := InternalConfig{globalTags: map[string]string{"team": "example-team"}}

	i.tagsFor("example-bucket")

	assert.Equal(t, map[string]string{"team": "example-team"}, i.globalTags)
	assert.NotContains(t, i.Tags(), util.BuilditResourceIDTagKey)
}

// globalTags is only allocated when audit tags are on, so --no-audit against a config with
// no globalTags leaves it nil and a bare map write would panic.
func TestTagsForNilGlobalTags(t *testing.T) {
	i := InternalConfig{}

	require.NotPanics(t, func() {
		assert.Equal(t, map[string]string{
			util.BuilditResourceIDTagKey: "example-bucket",
		}, i.tagsFor("example-bucket"))
	})
}

// A wildcard certificate's identifier is its domain name, and '*' is not a legal character
// in an AWS tag value — unsanitized it would fail RequestCertificate outright.
func TestTagsForSanitizesValue(t *testing.T) {
	i := InternalConfig{}

	assert.Equal(t, "_.example.com", i.tagsFor("*.example.com")[util.BuilditResourceIDTagKey])
}
