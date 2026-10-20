package config

import (
	"context"
	"errors"
	"testing"

	"github.com/TouchBistro/buildit/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeResource satisfies resource.Resource but not ComparableResource.
type fakeResource struct {
	key resource.Key
}

func (f fakeResource) Key() resource.Key             { return f.key }
func (f fakeResource) Identifier() string            { return f.key.String() }
func (f fakeResource) Apply(context.Context) error   { return nil }
func (f fakeResource) Destroy(context.Context) error { return nil }

// fakeComparable returns a canned diff/error from Compare.
type fakeComparable struct {
	fakeResource
	diff resource.ResourceDiff
	err  error
}

func (f fakeComparable) Compare(context.Context) (resource.ResourceDiff, error) {
	return f.diff, f.err
}

// newComparable builds a fakeComparable under the main provider. A nil diff must
// be passed as an untyped nil so Compare returns a nil interface, as real
// resources do when nothing differs.
func newComparable(name string, diff *resource.BaseResourceDiff) fakeComparable {
	f := fakeComparable{fakeResource: fakeResource{key: resource.NewKey("main", name)}}
	if diff != nil {
		f.diff = diff
	}
	return f
}

func singleEntry(t *testing.T, res resource.Resource, targets []resource.Key) PlanEntry {
	t.Helper()
	g := &Graph{}
	require.NoError(t, g.AddVertex(res, nil))
	entries, err := g.PlanResult(context.Background(), targets)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	return entries[0]
}

func TestPlanResultClassifiesEachAction(t *testing.T) {
	existing := struct{ Name string }{"old-name"}
	cases := []struct {
		name    string
		res     resource.Resource
		targets []resource.Key
		want    PlanEntry
	}{
		{
			name: "create when diff has no aws resource",
			res:  newComparable("example-create", &resource.BaseResourceDiff{Messages: []string{"does not exist"}}),
			want: PlanEntry{Key: resource.NewKey("main", "example-create"), Type: "config.fakeComparable", Action: PlanCreate},
		},
		{
			name: "update when diff has aws resource and messages",
			res: newComparable("example-update", &resource.BaseResourceDiff{
				Messages: []string{"name: old-name != new-name"},
				Resource: existing,
			}),
			want: PlanEntry{
				Key: resource.NewKey("main", "example-update"), Type: "config.fakeComparable", Action: PlanUpdate,
				Differences: []string{"name: old-name != new-name"}, AWSResource: existing,
			},
		},
		{
			name: "no change when diff is nil",
			res:  newComparable("example-same", nil),
			want: PlanEntry{Key: resource.NewKey("main", "example-same"), Type: "config.fakeComparable", Action: PlanNoChange},
		},
		{
			name: "no change when diff has aws resource but no messages",
			res:  newComparable("example-empty", &resource.BaseResourceDiff{Resource: existing}),
			want: PlanEntry{Key: resource.NewKey("main", "example-empty"), Type: "config.fakeComparable", Action: PlanNoChange},
		},
		{
			name: "unsupported when resource is not comparable",
			res:  fakeResource{key: resource.NewKey("main", "example-plain")},
			want: PlanEntry{Key: resource.NewKey("main", "example-plain"), Type: "config.fakeResource", Action: PlanUnsupported},
		},
		{
			name:    "skipped when excluded by a non-empty targets list",
			res:     newComparable("example-skipped", &resource.BaseResourceDiff{Messages: []string{"never compared"}}),
			targets: []resource.Key{resource.NewKey("main", "example-other")},
			want:    PlanEntry{Key: resource.NewKey("main", "example-skipped"), Type: "config.fakeComparable", Action: PlanSkipped},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, singleEntry(t, tc.res, tc.targets))
		})
	}
}

func TestPlanResultEmptyTargetsPlansEverything(t *testing.T) {
	entry := singleEntry(t, newComparable("example-same", nil), []resource.Key{})
	assert.Equal(t, PlanNoChange, entry.Action)
}

func TestPlanResultFollowsDependencyOrder(t *testing.T) {
	g, targets := planFixtureGraph(t)
	entries, err := g.PlanResult(context.Background(), targets)
	require.NoError(t, err)

	var got []PlanAction
	for _, e := range entries {
		got = append(got, e.Action)
	}
	assert.Equal(t, []PlanAction{PlanCreate, PlanUpdate, PlanNoChange, PlanUnsupported, PlanSkipped}, got)
}

func TestPlanResultWrapsCompareError(t *testing.T) {
	g := &Graph{}
	ok := newComparable("example-ok", nil)
	broken := newComparable("example-broken", nil)
	broken.err = errors.New("access denied")
	require.NoError(t, g.AddVertex(ok, nil))
	require.NoError(t, g.AddVertex(broken, nil))
	require.NoError(t, g.AddEdge(ok.Key(), broken.Key()))

	entries, err := g.PlanResult(context.Background(), nil)
	require.EqualError(t, err, "failed comparing resource main::example-broken: access denied")
	// Entries computed before the failure are still returned.
	require.Len(t, entries, 1)
	assert.Equal(t, ok.Key(), entries[0].Key)
}

func TestPlanResultCycleError(t *testing.T) {
	g := &Graph{}
	a := newComparable("example-a", nil)
	b := newComparable("example-b", nil)
	require.NoError(t, g.AddVertex(a, nil))
	require.NoError(t, g.AddVertex(b, nil))
	require.NoError(t, g.AddEdge(a.Key(), b.Key()))
	require.NoError(t, g.AddEdge(b.Key(), a.Key()))

	_, err := g.PlanResult(context.Background(), nil)
	require.ErrorContains(t, err, "failed to toposort resource graph")
}

func TestPlanActionString(t *testing.T) {
	assert.Equal(t, "create", PlanCreate.String())
	assert.Equal(t, "skipped", PlanSkipped.String())
	assert.Equal(t, "unknown", PlanAction(99).String())
}
