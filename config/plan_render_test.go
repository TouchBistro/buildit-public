package config

import (
	"bytes"
	"context"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TouchBistro/buildit/resource"
	"github.com/TouchBistro/goutils/color"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

// Regenerate the golden files with: go test ./config -run TestPlanTargetsOutput -update
var update = flag.Bool("update", false, "rewrite golden files")

// captureLog routes the global logrus logger into a buffer with a deterministic
// formatter and disables ANSI colour so golden files are stable. State is
// restored when the test ends.
func captureLog(t *testing.T, level log.Level) *bytes.Buffer {
	t.Helper()
	std := log.StandardLogger()
	prevOut, prevFmt, prevLevel := std.Out, std.Formatter, std.Level
	t.Cleanup(func() {
		std.SetOutput(prevOut)
		std.SetFormatter(prevFmt)
		std.SetLevel(prevLevel)
		color.SetEnabled(true)
	})

	var buf bytes.Buffer
	std.SetOutput(&buf)
	std.SetFormatter(&log.TextFormatter{DisableColors: true, DisableTimestamp: true})
	std.SetLevel(level)
	color.SetEnabled(false)
	return &buf
}

// planFixtureGraph builds a chain a -> b -> c -> d -> e so toposort order is
// deterministic, with one resource per plan outcome.
func planFixtureGraph(t *testing.T) (*Graph, []resource.Key) {
	t.Helper()
	g := &Graph{}
	create := newComparable("example-create", &resource.BaseResourceDiff{Messages: []string{"does not exist"}})
	updated := newComparable("example-update", &resource.BaseResourceDiff{
		Messages: []string{"name: old-name != new-name", "port: 80 != 443"},
		Resource: struct{ Name string }{"old-name"},
	})
	same := newComparable("example-same", nil)
	unsupported := fakeResource{key: resource.NewKey("main", "example-unsupported")}
	skipped := newComparable("example-skipped", nil)

	all := []resource.Resource{create, updated, same, unsupported, skipped}
	for _, r := range all {
		require.NoError(t, g.AddVertex(r, nil))
	}
	for i := 1; i < len(all); i++ {
		require.NoError(t, g.AddEdge(all[i-1].Key(), all[i].Key()))
	}
	targets := []resource.Key{create.Key(), updated.Key(), same.Key(), unsupported.Key()}
	return g, targets
}

func TestPlanTargetsOutput(t *testing.T) {
	cases := []struct {
		name   string
		level  log.Level
		golden string
	}{
		{"info level hides skipped and warns", log.InfoLevel, "plan_info.golden"},
		{"debug level lists skipped and does not warn", log.DebugLevel, "plan_debug.golden"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g, targets := planFixtureGraph(t)
			buf := captureLog(t, tc.level)
			require.NoError(t, g.PlanTargets(context.Background(), targets))

			path := filepath.Join("testdata", tc.golden)
			if *update {
				require.NoError(t, os.MkdirAll("testdata", 0o755))
				require.NoError(t, os.WriteFile(path, buf.Bytes(), 0o644))
			}
			want, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, string(want), buf.String())
		})
	}
}

// hookedComparable runs an arbitrary function as Compare.
type hookedComparable struct {
	fakeResource
	compare func(context.Context) (resource.ResourceDiff, error)
}

func (h hookedComparable) Compare(ctx context.Context) (resource.ResourceDiff, error) {
	return h.compare(ctx)
}

// TestPlanTargetsStreamsEntries pins the progressive rendering: a resource's plan
// line is written before the next resource's Compare starts, so a long plan
// shows progress instead of printing everything after the last AWS call.
func TestPlanTargetsStreamsEntries(t *testing.T) {
	buf := captureLog(t, log.InfoLevel)

	first := newComparable("example-first", &resource.BaseResourceDiff{Messages: []string{"does not exist"}})
	var sawFirst bool
	second := hookedComparable{
		fakeResource: fakeResource{key: resource.NewKey("main", "example-second")},
		compare: func(context.Context) (resource.ResourceDiff, error) {
			sawFirst = strings.Contains(buf.String(), "example-first")
			return nil, nil
		},
	}

	g := &Graph{}
	require.NoError(t, g.AddVertex(first, nil))
	require.NoError(t, g.AddVertex(second, nil))
	require.NoError(t, g.AddEdge(first.Key(), second.Key()))

	require.NoError(t, g.PlanTargets(context.Background(), nil))
	require.True(t, sawFirst, "first resource must be rendered before the second Compare runs")
}
