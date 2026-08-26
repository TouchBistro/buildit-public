package util

import (
	"reflect"
	"testing"
)

func TestTagDiff(t *testing.T) {
	tests := []struct {
		name            string
		current         map[string]string
		desired         map[string]string
		expectedAdded   map[string]string
		expectedDeleted map[string]string
		expectedChanged map[string]string
	}{
		{
			name:            "equal maps",
			current:         map[string]string{"env": "prod", "team": "api"},
			desired:         map[string]string{"team": "api", "env": "prod"},
			expectedAdded:   map[string]string{},
			expectedDeleted: map[string]string{},
			expectedChanged: map[string]string{},
		},
		{
			name:            "add only",
			current:         map[string]string{"env": "prod"},
			desired:         map[string]string{"env": "prod", "team": "api"},
			expectedAdded:   map[string]string{"team": "api"},
			expectedDeleted: map[string]string{},
			expectedChanged: map[string]string{},
		},
		{
			name:            "delete only",
			current:         map[string]string{"env": "prod", "team": "api"},
			desired:         map[string]string{"env": "prod"},
			expectedAdded:   map[string]string{},
			expectedDeleted: map[string]string{"team": "api"},
			expectedChanged: map[string]string{},
		},
		{
			name:            "change only",
			current:         map[string]string{"env": "prod"},
			desired:         map[string]string{"env": "stage"},
			expectedAdded:   map[string]string{},
			expectedDeleted: map[string]string{},
			expectedChanged: map[string]string{"env": "stage"},
		},
		{
			name:            "mixed add delete change",
			current:         map[string]string{"env": "prod", "team": "api", "tier": "backend"},
			desired:         map[string]string{"env": "stage", "team": "api", "owner": "platform"},
			expectedAdded:   map[string]string{"owner": "platform"},
			expectedDeleted: map[string]string{"tier": "backend"},
			expectedChanged: map[string]string{"env": "stage"},
		},
		{
			name:            "nil maps",
			current:         nil,
			desired:         nil,
			expectedAdded:   map[string]string{},
			expectedDeleted: map[string]string{},
			expectedChanged: map[string]string{},
		},
		{
			name:            "nil current map",
			current:         nil,
			desired:         map[string]string{"team": "api"},
			expectedAdded:   map[string]string{"team": "api"},
			expectedDeleted: map[string]string{},
			expectedChanged: map[string]string{},
		},
		{
			name:            "nil desired map deletes all current tags",
			current:         map[string]string{"env": "prod", "team": "api"},
			desired:         nil,
			expectedAdded:   map[string]string{},
			expectedDeleted: map[string]string{"env": "prod", "team": "api"},
			expectedChanged: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diff := TagDiff(tt.current, tt.desired)

			if !reflect.DeepEqual(diff.Added, tt.expectedAdded) {
				t.Fatalf("unexpected Added: expected=%v got=%v", tt.expectedAdded, diff.Added)
			}
			if !reflect.DeepEqual(diff.Deleted, tt.expectedDeleted) {
				t.Fatalf("unexpected Deleted: expected=%v got=%v", tt.expectedDeleted, diff.Deleted)
			}
			if !reflect.DeepEqual(diff.Changed, tt.expectedChanged) {
				t.Fatalf("unexpected Changed: expected=%v got=%v", tt.expectedChanged, diff.Changed)
			}
		})
	}
}

func TestTagDiffHelpers(t *testing.T) {
	diff := TagDiff(
		map[string]string{
			"owner": "core",
			"team":  "api",
		},
		map[string]string{
			"owner": "platform",
			"env":   "prod",
		},
	)

	if !diff.HasChanges() {
		t.Fatal("expected HasChanges to be true")
	}

	expectedUpserts := map[string]string{
		"owner": "platform",
		"env":   "prod",
	}
	if !reflect.DeepEqual(diff.Upserts(), expectedUpserts) {
		t.Fatalf("unexpected Upserts: expected=%v got=%v", expectedUpserts, diff.Upserts())
	}

	expectedDeletedKeys := []string{"team"}
	if !reflect.DeepEqual(diff.DeletedKeys(), expectedDeletedKeys) {
		t.Fatalf("unexpected DeletedKeys: expected=%v got=%v", expectedDeletedKeys, diff.DeletedKeys())
	}
}

func TestTagDiffHelpersNoChanges(t *testing.T) {
	diff := TagDiff(
		map[string]string{"team": "api"},
		map[string]string{"team": "api"},
	)

	if diff.HasChanges() {
		t.Fatal("expected HasChanges to be false")
	}

	if len(diff.Upserts()) != 0 {
		t.Fatalf("expected empty Upserts, got %v", diff.Upserts())
	}

	if len(diff.DeletedKeys()) != 0 {
		t.Fatalf("expected empty DeletedKeys, got %v", diff.DeletedKeys())
	}
}

func TestTagDiffWithIgnoredKeys(t *testing.T) {
	diff := TagDiffWithIgnoredKeys(
		map[string]string{
			"env":   "prod",
			"bob":   "old",
			"vance": "x",
		},
		map[string]string{
			"env":  "stage",
			"bob":  "new",
			"fish": "y",
		},
		[]string{" bob ", "vance", "fish", ""},
	)

	expected := TagDiffResult{
		Added:   map[string]string{},
		Deleted: map[string]string{},
		Changed: map[string]string{"env": "stage"},
	}

	if !reflect.DeepEqual(diff, expected) {
		t.Fatalf("unexpected diff: expected=%v got=%v", expected, diff)
	}
}

func TestTagDiffWithIgnoredKeysNoIgnore(t *testing.T) {
	current := map[string]string{"env": "prod", "team": "api"}
	desired := map[string]string{"env": "stage", "owner": "platform"}

	diff := TagDiffWithIgnoredKeys(current, desired, nil)
	expected := TagDiff(current, desired)

	if !reflect.DeepEqual(diff, expected) {
		t.Fatalf("expected diff to match TagDiff, expected=%v got=%v", expected, diff)
	}
}
