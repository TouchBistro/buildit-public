package resource

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func exampleLogGroupFilter() CWLogGroupMetricFilter {
	return CWLogGroupMetricFilter{
		Name:    "example-errors",
		Pattern: "ERROR",
		MetricTransformations: []CWLogGroupMetricFilterTransformation{{
			Name:         "ExampleErrors",
			Namespace:    "example/app",
			Value:        "1",
			DefaultValue: 0,
			Dimensions:   map[string]string{"service": "example-service"},
			Unit:         aws.String("Count"),
		}},
	}
}

// exampleLogGroupPair returns an identical existing/desired pair; tests mutate one side.
func exampleLogGroupPair() (*CWLogGroup, CWLogGroup) {
	existing := &CWLogGroup{
		Name:          "example-log-group",
		Retention:     aws.Int32(30),
		MetricFilters: []CWLogGroupMetricFilter{exampleLogGroupFilter()},
		Arn:           aws.String("arn:aws:logs:us-east-1:123456789012:log-group:example-log-group"),
	}
	desired := CWLogGroup{
		Name:          "example-log-group",
		Retention:     aws.Int32(30),
		MetricFilters: []CWLogGroupMetricFilter{exampleLogGroupFilter()},
		Tags:          map[string]string{"env": "example"},
	}
	return existing, desired
}

func TestCWLogGroup_compareLogGroup_Identical(t *testing.T) {
	existing, desired := exampleLogGroupPair()

	diffs := desired.compareLogGroup(context.Background(), existing, map[string]string{"env": "example"})
	assert.Nil(t, diffs)
}

func TestCWLogGroup_compareLogGroup_IsDetailed(t *testing.T) {
	existing, desired := exampleLogGroupPair()
	desired.Retention = aws.Int32(90)

	var diff ResourceDiff = desired.compareLogGroup(context.Background(), existing, map[string]string{"env": "example"})
	_, ok := diff.(DetailedResourceDiff)
	assert.True(t, ok)
}

func TestCWLogGroup_compareLogGroup_Retention(t *testing.T) {
	existing, desired := exampleLogGroupPair()
	desired.Retention = aws.Int32(90)

	diffs := desired.compareLogGroup(context.Background(), existing, map[string]string{"env": "example"})
	require.NotNil(t, diffs)
	assert.True(t, diffs.diffRetention)
	assert.False(t, diffs.metricfiltersDiff)
	assert.False(t, diffs.tagsDiff)
	assert.Equal(t, []string{"retention is not the same"}, diffs.Differences())
	assert.Equal(t, []Change{{Field: "retention", From: "30", To: "90"}}, diffs.Changes())
}

func TestCWLogGroup_compareLogGroup_RetentionNeverExpire(t *testing.T) {
	existing, desired := exampleLogGroupPair()
	existing.Retention = nil // AWS: never expire

	diffs := desired.compareLogGroup(context.Background(), existing, map[string]string{"env": "example"})
	require.NotNil(t, diffs)
	assert.Equal(t, []Change{{Field: "retention", From: "", To: "30"}}, diffs.Changes())
}

func TestCWLogGroup_compareLogGroup_MetricFilterField(t *testing.T) {
	existing, desired := exampleLogGroupPair()
	desired.MetricFilters[0].Pattern = "ERROR -health"

	diffs := desired.compareLogGroup(context.Background(), existing, map[string]string{"env": "example"})
	require.NotNil(t, diffs)
	assert.True(t, diffs.metricfiltersDiff)
	assert.Equal(t, []string{"metric filter at index 0 is not the same"}, diffs.Differences())
	assert.Equal(t, []Change{
		{Field: "metricFilters[0].pattern", From: "ERROR", To: "ERROR -health"},
	}, diffs.Changes())
}

func TestCWLogGroup_compareLogGroup_MetricFilterTransformation(t *testing.T) {
	existing, desired := exampleLogGroupPair()
	existing.MetricFilters[0].MetricTransformations[0].Unit = nil // treated as None
	desired.MetricFilters[0].MetricTransformations[0].Namespace = "example/new"
	desired.MetricFilters[0].MetricTransformations[0].Dimensions = map[string]string{"service": "new-service", "region": "example"}

	diffs := desired.compareLogGroup(context.Background(), existing, map[string]string{"env": "example"})
	require.NotNil(t, diffs)
	assert.Equal(t, []string{"metric filter at index 0 is not the same"}, diffs.Differences())
	assert.Equal(t, []Change{
		{Field: "metricFilters[0].metricTransformations[0].namespace", From: "example/app", To: "example/new"},
		{Field: "metricFilters[0].metricTransformations[0].unit", From: "None", To: "Count"},
		{Field: "metricFilters[0].metricTransformations[0].dimensions.region", From: "", To: "example"},
		{Field: "metricFilters[0].metricTransformations[0].dimensions.service", From: "example-service", To: "new-service"},
	}, diffs.Changes())
}

func TestCWLogGroup_compareLogGroup_MetricFilterTransformationCount(t *testing.T) {
	existing, desired := exampleLogGroupPair()
	desired.MetricFilters[0].MetricTransformations = nil

	diffs := desired.compareLogGroup(context.Background(), existing, map[string]string{"env": "example"})
	require.NotNil(t, diffs)
	assert.Equal(t, []string{"metric filter at index 0 is not the same"}, diffs.Differences())
	assert.Equal(t, []Change{
		{Field: "metricFilters[0].metricTransformations", From: "1", To: "0"},
	}, diffs.Changes())
}

func TestCWLogGroup_compareLogGroup_MetricFilterCount(t *testing.T) {
	t.Run("more defined", func(t *testing.T) {
		existing, desired := exampleLogGroupPair()
		existing.MetricFilters = nil

		diffs := desired.compareLogGroup(context.Background(), existing, map[string]string{"env": "example"})
		require.NotNil(t, diffs)
		assert.Equal(t, []string{"additional metric filters defined than what exist"}, diffs.Differences())
		assert.Equal(t, []Change{{Field: "metricFilters", From: "0", To: "1"}}, diffs.Changes())
	})

	t.Run("fewer defined", func(t *testing.T) {
		existing, desired := exampleLogGroupPair()
		desired.MetricFilters = nil

		diffs := desired.compareLogGroup(context.Background(), existing, map[string]string{"env": "example"})
		require.NotNil(t, diffs)
		assert.Equal(t, []string{"less metric filters defined than what exist"}, diffs.Differences())
		assert.Equal(t, []Change{{Field: "metricFilters", From: "1", To: "0"}}, diffs.Changes())
	})
}

func TestCWLogGroup_compareLogGroup_Tags(t *testing.T) {
	existing, desired := exampleLogGroupPair()
	desired.Tags = map[string]string{"env": "new-env", "team": "example-team"}

	diffs := desired.compareLogGroup(context.Background(), existing, map[string]string{"env": "old-env", "owner": "old-owner"})
	require.NotNil(t, diffs)
	assert.True(t, diffs.tagsDiff)
	assert.Equal(t, []string{
		"tag env is different: old-env -> new-env",
		"tag owner:old-owner is removed",
		"tag team is added: example-team",
	}, diffs.Differences())
	assert.Equal(t, []Change{
		{Field: "tags.env", From: "old-env", To: "new-env"},
		{Field: "tags.owner", From: "old-owner"},
		{Field: "tags.team", To: "example-team"},
	}, diffs.Changes())
}

func TestCWLogGroup_compareLogGroup_MessageOrder(t *testing.T) {
	existing, desired := exampleLogGroupPair()
	desired.Retention = aws.Int32(7)
	desired.MetricFilters[0].Name = "new-errors"
	desired.Tags = map[string]string{"env": "new-env"}

	diffs := desired.compareLogGroup(context.Background(), existing, map[string]string{"env": "old-env"})
	require.NotNil(t, diffs)
	assert.Same(t, existing, diffs.AWSResource())
	assert.Equal(t, []string{
		"retention is not the same",
		"metric filter at index 0 is not the same",
		"tag env is different: old-env -> new-env",
	}, diffs.Differences())
	assert.Equal(t, []Change{
		{Field: "retention", From: "30", To: "7"},
		{Field: "metricFilters[0].name", From: "example-errors", To: "new-errors"},
		{Field: "tags.env", From: "old-env", To: "new-env"},
	}, diffs.Changes())
}
