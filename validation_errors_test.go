package buildit_test

import (
	"context"
	"errors"
	"testing"

	"github.com/TouchBistro/buildit"
	"github.com/TouchBistro/buildit/client"
	"github.com/TouchBistro/buildit/config"
	"github.com/TouchBistro/buildit/resource"
	"github.com/TouchBistro/buildit/util"
	"github.com/aws/aws-sdk-go-v2/aws"
	pkgerrors "github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	logtest "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func logGroupKey(name string) resource.Key {
	return resource.NewKey(client.MainProvider, name)
}

// Every failure comes back in the error itself: a library consumer sees which resources
// failed without reading a log.
func TestNewConfigReturnsEveryValidationError(t *testing.T) {
	offlineDefault(t)
	hook := logtest.NewGlobal()
	defer hook.Reset()

	var rs buildit.ResourceSet
	require.NoError(t, rs.AddCWLogGroup("example-bad", resource.CWLogGroup{Retention: aws.Int32(2)}))
	require.NoError(t, rs.AddCWLogGroup("example-worse", resource.CWLogGroup{Retention: aws.Int32(4)}))
	require.NoError(t, rs.AddCWLogGroup("example-reserved", resource.CWLogGroup{
		Tags: map[string]string{util.BuilditTagPrefix + "owner": "example-team"},
	}))

	_, err := buildit.NewConfig(context.Background(), rs, buildit.WithoutAuditTags())
	require.Error(t, err)

	var verrs buildit.ValidationErrors
	require.ErrorAs(t, err, &verrs)
	failed := make(map[string]bool)
	for _, e := range verrs {
		var verr *resource.ValidationError
		require.ErrorAs(t, e, &verr)
		failed[verr.ResourceIdentifier] = true
	}
	for _, name := range []string{"example-bad", "example-worse", "example-reserved"} {
		assert.True(t, failed[name], name)
		assert.Contains(t, err.Error(), name)
	}
	var verr *resource.ValidationError
	assert.True(t, errors.As(err, &verr), "a single failure is reachable from the returned error")

	for _, e := range hook.AllEntries() {
		assert.NotEqual(t, log.ErrorLevel, e.Level, "failures are returned, not logged: %q", e.Message)
	}
}

func TestNewConfigRejectsDependencyCycle(t *testing.T) {
	offlineDefault(t)
	first, second := logGroupKey("example-first"), logGroupKey("example-second")

	var rs buildit.ResourceSet
	require.NoError(t, rs.AddCWLogGroup("example-first", resource.CWLogGroup{DependsOn: []resource.Key{second}}))
	require.NoError(t, rs.AddCWLogGroup("example-second", resource.CWLogGroup{DependsOn: []resource.Key{first}}))

	_, err := buildit.NewConfig(context.Background(), rs, buildit.WithoutAuditTags())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dependency cycle")
	assert.Contains(t, err.Error(), first.String())
	assert.Contains(t, err.Error(), second.String())
}

// A dependency on a key no resource has is an error, not a dropped edge.
func TestNewConfigRejectsDependencyOnMissingResource(t *testing.T) {
	offlineDefault(t)

	var rs buildit.ResourceSet
	require.NoError(t, rs.AddCWLogGroup("example-first", resource.CWLogGroup{
		DependsOn: []resource.Key{logGroupKey("example-missing")},
	}))

	_, err := buildit.NewConfig(context.Background(), rs, buildit.WithoutAuditTags())
	require.Error(t, err)
	assert.Contains(t, err.Error(), `depends on "main::example-missing"`)
	assert.Contains(t, err.Error(), "does not exist")
}

// The CLI prints a failed run as a heading per resource with its messages below; Lines is
// that rendering, and Error carries the same text.
func TestValidationErrorsLines(t *testing.T) {
	verrs := config.ValidationErrors{
		&resource.ValidationError{ResourceIdentifier: "example-first", ResourceType: "example", Messages: []string{"one", "two"}},
		errors.New("resource key main::example-second is declared twice"),
	}

	assert.Equal(t, []string{
		"example-first (example)",
		"\t one",
		"\t two",
		"\t resource key main::example-second is declared twice",
	}, verrs.Lines())
	assert.Equal(t, "One or more resources failed validation", verrs.Summary())
	assert.Contains(t, verrs.Error(), "One or more resources failed validation:\nexample-first (example)\n\t one")
}

// A ValidationError that was wrapped with context (a failed Normalize is) prints as one
// line carrying that context; the heading form would drop it.
func TestValidationErrorsLinesKeepsWrappedContext(t *testing.T) {
	inner := &resource.ValidationError{ResourceIdentifier: "example-first", ResourceType: "example", Messages: []string{"one"}}
	verrs := config.ValidationErrors{pkgerrors.Wrap(inner, `failed normalizing example "example-first"`)}

	lines := verrs.Lines()
	require.Len(t, lines, 1)
	assert.Contains(t, lines[0], `failed normalizing example "example-first"`)

	var verr *resource.ValidationError
	assert.True(t, errors.As(verrs, &verr), "the wrapped failure is still reachable")
}
