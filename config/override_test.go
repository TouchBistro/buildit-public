package config

import (
	"context"
	"testing"

	"github.com/TouchBistro/buildit/resource"
	"github.com/TouchBistro/buildit/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func overrideWithSecurityGroup(name string, sg resource.SecurityGroup) InternalConfig {
	return InternalConfig{
		graph: &Graph{},
		_override: &builditConfig{Resources: resourcesConfig{
			SecurityGroup: map[string]resource.SecurityGroup{name: sg},
		}},
	}
}

func collectErrs() (func(error), *[]error) {
	var errs []error
	return func(err error) {
		if err != nil {
			errs = append(errs, err)
		}
	}, &errs
}

// An override can append a brand-new security group when its name matches no
// existing resource. Before DEVOPS-8901 the method took the error slice by value,
// so a Validate failure on that group was appended to a local copy and lost: the
// invalid resource was accepted and applied.
func TestOverrideSecurityGroupsReportsValidationErrors(t *testing.T) {
	i := overrideWithSecurityGroup("example-invalid-sg", resource.SecurityGroup{
		Description: "missing vpcName on purpose",
	})
	addErr, errs := collectErrs()

	require.NoError(t, i.overrideSecurityGroups(context.Background(), addErr))

	require.Len(t, *errs, 1)
	var ve *resource.ValidationError
	require.ErrorAs(t, (*errs)[0], &ve)
	assert.Equal(t, "example-invalid-sg", ve.ResourceIdentifier)
	assert.Contains(t, ve.Messages, "vpc name must be supplied to create a security group")
	assert.Equal(t, 0, i.graph.Size(), "an invalid override resource must not reach the graph")
}

// The override path skipped the reserved-tag check while its errors went nowhere.
// It now runs the same checks as every other resource in Generate.
func TestOverrideSecurityGroupsReportsReservedTags(t *testing.T) {
	i := overrideWithSecurityGroup("example-sg", resource.SecurityGroup{
		VPCName: "example-vpc",
		Tags:    map[string]string{util.BuilditTagPrefix + "owner": "example-team"},
	})
	addErr, errs := collectErrs()

	require.NoError(t, i.overrideSecurityGroups(context.Background(), addErr))

	require.Len(t, *errs, 1)
	assert.Contains(t, (*errs)[0].Error(), util.BuilditTagPrefix+"owner")
}

func TestOverrideSecurityGroupsAddsValidResource(t *testing.T) {
	i := overrideWithSecurityGroup("example-sg", resource.SecurityGroup{
		Description: "a valid override group",
		VPCName:     "example-vpc",
	})
	addErr, errs := collectErrs()

	require.NoError(t, i.overrideSecurityGroups(context.Background(), addErr))

	assert.Empty(t, *errs)
	require.Equal(t, 1, i.graph.Size())
	got, err := i.graph.GetVertex(resource.NewKey("main", "example-sg"))
	require.NoError(t, err)
	sg, ok := got.(resource.SecurityGroup)
	require.True(t, ok)
	assert.Equal(t, "example-sg", sg.Tags[util.BuilditResourceIDTagKey])
}

func graphWithSecurityGroup(t *testing.T, name string) *Graph {
	t.Helper()
	g := &Graph{}
	sg := resource.SecurityGroup{Name: name, VPCName: "example-vpc", Description: "original"}
	require.NoError(t, g.AddVertex(sg, sg.DependsOn))
	return g
}

func overrideOnGraph(g *Graph, pattern string, sg resource.SecurityGroup) InternalConfig {
	i := overrideWithSecurityGroup(pattern, sg)
	i.graph = g
	return i
}

// The '*' branch type-asserts the vertex resource into a local copy. Merging into
// that copy without writing it back made every wildcard override a silent no-op.
func TestOverrideSecurityGroupsWildcardWritesBack(t *testing.T) {
	g := graphWithSecurityGroup(t, "example-sg")
	i := overrideOnGraph(g, "*", resource.SecurityGroup{Description: "overridden"})
	addErr, errs := collectErrs()

	require.NoError(t, i.overrideSecurityGroups(context.Background(), addErr))

	assert.Empty(t, *errs)
	got, err := g.GetVertex(resource.NewKey("main", "example-sg"))
	require.NoError(t, err)
	assert.Equal(t, "overridden", got.(resource.SecurityGroup).Description)
}

// Generate builds edges from graph.depdendencies, captured at AddVertex time. A
// dependsOn merged in by an override landed on the resource but never became an
// edge, in both the wildcard and the pattern branch.
func TestOverrideSecurityGroupsMergedDependsOnBecomesEdge(t *testing.T) {
	dep := resource.NewKey("main", "example-dep")
	key := resource.NewKey("main", "example-sg")

	for _, pattern := range []string{"*", "example-sg", "^main::example-.*$"} {
		t.Run(pattern, func(t *testing.T) {
			g := graphWithSecurityGroup(t, "example-sg")
			i := overrideOnGraph(g, pattern, resource.SecurityGroup{DependsOn: []resource.Key{dep}})
			addErr, errs := collectErrs()

			require.NoError(t, i.overrideSecurityGroups(context.Background(), addErr))

			assert.Empty(t, *errs)
			got, err := g.GetVertex(key)
			require.NoError(t, err)
			assert.Equal(t, []resource.Key{dep}, got.(resource.SecurityGroup).DependsOn)
			assert.Equal(t, []resource.Key{dep}, g.depdendencies[key], "merged dependsOn must reach the edge snapshot")
		})
	}
}

// A reserved tag on an override that merges into an existing group used to be
// stripped by ResourceTags.Merge without a word. Every other path reports it.
func TestOverrideSecurityGroupsReportsReservedTagsOnMerge(t *testing.T) {
	g := graphWithSecurityGroup(t, "example-sg")
	i := overrideOnGraph(g, "example-sg", resource.SecurityGroup{
		Tags: map[string]string{util.BuilditTagPrefix + "owner": "example-team"},
	})
	addErr, errs := collectErrs()

	require.NoError(t, i.overrideSecurityGroups(context.Background(), addErr))

	require.Len(t, *errs, 1)
	assert.Contains(t, (*errs)[0].Error(), util.BuilditTagPrefix+"owner")
}

// An override that appends a new group under a name another resource already
// holds is a key collision (DEVOPS-8902). It is reported through the accumulator
// like the other validation failures, not returned as a hard error.
func TestOverrideSecurityGroupsReportsKeyCollision(t *testing.T) {
	g := &Graph{}
	require.NoError(t, g.AddVertex(resource.S3Bucket{Bucket: "example-thing"}, nil))
	i := overrideOnGraph(g, "example-thing", resource.SecurityGroup{VPCName: "example-vpc"})
	addErr, errs := collectErrs()

	require.NoError(t, i.overrideSecurityGroups(context.Background(), addErr))

	require.Len(t, *errs, 1)
	assert.Contains(t, (*errs)[0].Error(), `s3-bucket "example-thing" and security-group "example-thing"`)
	got, err := g.GetVertex(resource.NewKey("main", "example-thing"))
	require.NoError(t, err)
	assert.IsType(t, resource.S3Bucket{}, got, "the existing resource must not be overwritten")
}
