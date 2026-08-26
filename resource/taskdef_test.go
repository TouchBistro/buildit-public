package resource

import (
	"context"
	"testing"

	"github.com/TouchBistro/buildit/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Containers default to projecting the task definition's tags as docker labels, so a new
// built-in tag would otherwise register as a container change: a new task definition
// revision, and a rolling redeploy of every service using it. buildit's own tags are
// internal metadata and must stay out of the label set.
//
// The skip has to sit above the ':' -> '-' rewrite in Normalize. A check placed after it
// sees "buildit-resource-id", matches nothing, and silently re-arms that redeploy — which
// is what this test exists to catch.
func TestTaskDefNormalizeKeepsBuilditTagsOutOfLabels(t *testing.T) {
	// buildit's tags reach a resource through GlobalTags, the way config wires them up.
	// A reserved key set on Tags is the case config load rejects, and ResourceTags.Merge
	// strips it anyway, so putting one there would not model anything real.
	td := TaskDef{
		Name: "example-taskdef",
		GlobalTags: map[string]string{
			util.BuilditResourceIDTagKey: "example-taskdef",
			"buildit:future-builtin":     "whatever",
			"iac:created-with":           "buildit",
			"audit":                      "example-hash",
		},
		Tags:       map[string]string{"team": "example-team"},
		Containers: []*Container{{Name: "example-container"}},
	}

	td.Normalize(context.Background())

	require.Len(t, td.Containers, 1)
	labels := td.Containers[0].Labels

	for key := range labels {
		assert.False(t, util.IsBuilditTagKey(key),
			"reserved tag %q was projected as a label", key)
		assert.NotContains(t, key, "buildit-",
			"reserved tag %q was projected as a label after the ':' rewrite", key)
	}

	// Ordinary tags still project, including the unprefixed legacy `audit` built-in.
	assert.Equal(t, map[string]string{
		"team":             "example-team",
		"iac-created-with": "buildit",
		"audit":            "example-hash",
	}, labels)

	// The tag itself is untouched — it still needs to reach AWS as a tag.
	assert.Equal(t, "example-taskdef", td.Tags[util.BuilditResourceIDTagKey])
}

// An explicit label is still the author's to set, reserved-looking or not: the skip must
// only stop the automatic projection, not rewrite what someone wrote by hand.
func TestTaskDefNormalizeKeepsExplicitLabels(t *testing.T) {
	td := TaskDef{
		Name:       "example-taskdef",
		GlobalTags: map[string]string{util.BuilditResourceIDTagKey: "example-taskdef"},
		Containers: []*Container{{Name: "example-container", Labels: map[string]string{"own": "label"}}},
	}

	td.Normalize(context.Background())

	assert.Equal(t, map[string]string{"own": "label"}, td.Containers[0].Labels)
}
