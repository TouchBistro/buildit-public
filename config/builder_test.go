package config

import (
	"context"
	"testing"

	"github.com/TouchBistro/buildit/resource"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// brokenResource has a Normalize that always fails, which no real type does offline.
type brokenResource struct {
	resource.BaseResource
	Name       string
	Tags       map[string]string
	GlobalTags map[string]string
	DependsOn  []resource.Key
}

func (r brokenResource) Key() resource.Key  { return resource.NewKey(r.Context.ProviderName, r.Name) }
func (r brokenResource) Identifier() string { return r.Name }
func (r *brokenResource) Normalize(context.Context) error {
	return errors.New("size is past the limit")
}
func (r brokenResource) Validate(context.Context) error { return nil }
func (r brokenResource) Apply(context.Context) error    { return nil }
func (r brokenResource) Destroy(context.Context) error  { return nil }

// A Normalize that fails is collected with the validation failures, named after the
// resource, and keeps the resource out of the graph.
func TestGraphBuilderCollectsNormalizeError(t *testing.T) {
	i := Config{}
	b := &graphBuilder{cfg: &i, graph: &Graph{}}

	r := brokenResource{}
	require.NoError(t, admit(context.Background(), b, "example-big", &r, wiring{
		name: &r.Name, context: &r.Context, tags: &r.Tags, dependsOn: &r.DependsOn, globalTags: &r.GlobalTags,
	}))

	err := b.build()
	var verrs ValidationErrors
	require.ErrorAs(t, err, &verrs)
	require.Len(t, verrs, 1)
	assert.Contains(t, verrs[0].Error(), `"example-big": size is past the limit`)
	assert.Equal(t, 0, b.graph.Size())
}

// Generate rejects a dependency cycle, so a caller of config.FromResourceSet + Generate cannot
// get a graph that has no order.
func TestBuildGraphRejectsDependencyCycle(t *testing.T) {
	first, second := resource.NewKey("main", "example-first"), resource.NewKey("main", "example-second")

	i := FromResourceSet(ResourceSet{CWLogGroup: map[string]resource.CWLogGroup{
		"example-first":  logGroup("", second),
		"example-second": logGroup("", first),
	}}, nil)

	err := generateOffline(t, i, RootOptions{NoAuditTags: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dependency cycle between resources")
	assert.Contains(t, err.Error(), first.String())
	assert.Contains(t, err.Error(), second.String())
}

// One run reports the failures of every resource type, not only the first type's.
func TestBuildGraphCollectsFailuresAcrossTypes(t *testing.T) {
	i := FromResourceSet(ResourceSet{
		CWLogGroup:    map[string]resource.CWLogGroup{"example-logs": {Retention: aws.Int32(2)}},
		CWMetricAlarm: map[string]resource.CWMetricAlarm{"example-alarm": {}},
		DynamoDB:      map[string]resource.DynamoDB{"example-table": {}},
	}, nil)

	err := generateOffline(t, i, RootOptions{NoAuditTags: true})
	var verrs ValidationErrors
	require.ErrorAs(t, err, &verrs)
	require.Len(t, verrs, 3)

	types := make(map[string]bool)
	for _, e := range verrs {
		var verr *resource.ValidationError
		require.ErrorAs(t, e, &verr)
		types[verr.ResourceType] = true
	}
	assert.Len(t, types, 3, "one failure per resource type: %v", types)
}

// sqs-queue was never validated at config load, so a queue its Validate would reject
// still loads. Turning validation on for it is a behaviour change for existing configs.
func TestBuildGraphDoesNotValidateSQSQueue(t *testing.T) {
	queue := resource.SQSQueue{Attributes: resource.SQSQueueAttributes{DelaySeconds: 901}}
	require.NoError(t, queue.Normalize(context.Background()))
	require.Error(t, queue.Validate(context.Background()), "the queue is one Validate rejects")

	i := FromResourceSet(ResourceSet{SQSQueue: map[string]resource.SQSQueue{
		"example-queue": {Attributes: resource.SQSQueueAttributes{DelaySeconds: 901}},
	}}, nil)
	require.NoError(t, generateOffline(t, i, RootOptions{NoAuditTags: true}))
	assert.Equal(t, 1, i.Graph().Size())
}

// providerFrom is how route53-record lets a legacy `provider/name` recordName pick the
// provider: it wins over the provider in the declared name.
func TestAdmitProviderFromWinsOverDeclaredName(t *testing.T) {
	offlineProvider(t, "example-first")
	offlineProvider(t, "example-second")
	i := Config{}
	b := &graphBuilder{cfg: &i, graph: &Graph{}}

	r := brokenResource{}
	require.NoError(t, admit(context.Background(), b, "example-first/example-record", &r, wiring{
		name: &r.Name, context: &r.Context, tags: &r.Tags, dependsOn: &r.DependsOn,
		legacyDelim: true, providerFrom: "example-second/example-record",
	}))

	assert.Equal(t, "example-second", r.Context.ProviderName)
	assert.Equal(t, "example-record", r.Name)
}
