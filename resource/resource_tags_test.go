package resource

import (
	"context"
	"reflect"
	"testing"

	"github.com/TouchBistro/buildit/util"
)

func TestTagDiffForContext(t *testing.T) {
	current := map[string]string{
		"env": "prod",
		"bob": "old",
	}
	desired := map[string]string{
		"env": "stage",
		"bob": "new",
	}

	ctx := util.WithIgnoreTags(context.Background(), []string{"bob"})
	diff := TagDiffForContext(ctx, current, desired)

	expected := util.TagDiffResult{
		Added:   map[string]string{},
		Deleted: map[string]string{},
		Changed: map[string]string{"env": "stage"},
	}

	if !reflect.DeepEqual(diff, expected) {
		t.Fatalf("unexpected diff: expected=%v got=%v", expected, diff)
	}
}

func TestTagDiffForContextNilContext(t *testing.T) {
	current := map[string]string{"env": "prod"}
	desired := map[string]string{"env": "stage"}

	//nolint:staticcheck // intentional: test nil-context fallback behavior
	diff := TagDiffForContext(nil, current, desired)
	expected := util.TagDiff(current, desired)

	if !reflect.DeepEqual(diff, expected) {
		t.Fatalf("unexpected diff: expected=%v got=%v", expected, diff)
	}
}

func TestTagDiffSummary(t *testing.T) {
	current := map[string]string{
		"changed": "old",
		"deleted": "gone",
	}
	diff := util.TagDiffResult{
		Added: map[string]string{
			"added": "new",
		},
		Deleted: map[string]string{
			"deleted": "gone",
		},
		Changed: map[string]string{
			"changed": "new",
		},
	}

	summary := TagDiffSummary(current, diff)

	expected := []string{
		"tag changed is different: old -> new",
		"tag deleted:gone is removed",
		"tag added is added: new",
	}

	if !reflect.DeepEqual(summary, expected) {
		t.Fatalf("unexpected summary: expected=%v got=%v", expected, summary)
	}
}

// Ordinary keys keep the long-standing precedence: a resource's own tags beat globals.
func TestMergeKeepsResourceValueOverGlobal(t *testing.T) {
	tags := ResourceTags{"env": "stage"}
	tags.Merge(ResourceTags{"env": "prod", "team": "example-team"})

	expected := ResourceTags{"env": "stage", "team": "example-team"}
	if !reflect.DeepEqual(tags, expected) {
		t.Fatalf("unexpected tags: expected=%v got=%v", expected, tags)
	}
}

// Reserved keys invert that precedence: whatever buildit supplies through the global map
// wins, so a config that slipped a reserved key past validation cannot shadow it.
func TestMergeReservedKeyFromGlobalsWins(t *testing.T) {
	tags := ResourceTags{util.BuilditResourceIDTagKey: "not-the-real-id"}
	tags.Merge(ResourceTags{util.BuilditResourceIDTagKey: "example-bucket"})

	if got := tags[util.BuilditResourceIDTagKey]; got != "example-bucket" {
		t.Fatalf("reserved key = %q, want %q", got, "example-bucket")
	}
}

// A reserved key buildit does not itself write has nothing to overwrite it, so the sweep
// has to remove it outright rather than leaving it to lose a comparison.
func TestMergeDropsReservedKeyAbsentFromGlobals(t *testing.T) {
	tags := ResourceTags{"buildit:owner": "example-team", "env": "prod"}
	tags.Merge(ResourceTags{"team": "example-team"})

	expected := ResourceTags{"env": "prod", "team": "example-team"}
	if !reflect.DeepEqual(tags, expected) {
		t.Fatalf("unexpected tags: expected=%v got=%v", expected, tags)
	}
}

func TestInheritedTags(t *testing.T) {
	parent := map[string]string{
		"env":                          "prod",
		"team":                         "example-team",
		util.BuilditResourceIDTagKey:   "example-lb",
		util.BuilditTagPrefix + "sole": "reserved-but-unwritten",
	}

	inherited := InheritedTags(parent)

	expected := map[string]string{"env": "prod", "team": "example-team"}
	if !reflect.DeepEqual(inherited, expected) {
		t.Fatalf("unexpected child tags: expected=%v got=%v", expected, inherited)
	}

	// The parent still needs its own id — the copy must not be an alias.
	if _, ok := parent[util.BuilditResourceIDTagKey]; !ok {
		t.Fatal("InheritedTags mutated the parent tag map")
	}
}

func TestInheritedTagsNil(t *testing.T) {
	if got := InheritedTags(nil); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}
