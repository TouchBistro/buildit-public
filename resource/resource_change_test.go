package resource

import (
	"testing"

	"github.com/TouchBistro/buildit/util"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
)

type detailedDiff struct {
	BaseResourceDiff
	ChangeSet
}

func TestChangeSetMakesDiffDetailed(t *testing.T) {
	d := &detailedDiff{}
	var _ DetailedResourceDiff = d

	d.AddChange("port", "80", "443")
	d.AddChanges(Change{Field: "tags.env", To: "example"})

	assert.Equal(t, []Change{
		{Field: "port", From: "80", To: "443"},
		{Field: "tags.env", To: "example"},
	}, d.Changes())
	assert.Nil(t, (&detailedDiff{}).Changes())
}

func TestBaseResourceDiffIsNotDetailed(t *testing.T) {
	var diff ResourceDiff = &BaseResourceDiff{}
	_, ok := diff.(DetailedResourceDiff)
	assert.False(t, ok)
}

func TestChangeValue(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"nil", nil, ""},
		{"string", "a", "a"},
		{"string ptr", aws.String("a"), "a"},
		{"nil string ptr", (*string)(nil), ""},
		{"bool", false, "false"},
		{"bool ptr", aws.Bool(true), "true"},
		{"int32 ptr", aws.Int32(8080), "8080"},
		{"int64 ptr", aws.Int64(300), "300"},
		{"nil int64 ptr", (*int64)(nil), ""},
		{"slice", []string{"a", "b"}, "a,b"},
		{"int", 7, "7"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ChangeValue(tc.in))
		})
	}
}

func TestTagChangesMirrorsTagDiffSummary(t *testing.T) {
	current := map[string]string{"env": "old-env", "team": "example-team", "stale": "x"}
	desired := map[string]string{"env": "new-env", "team": "example-team", "owner": "example-owner"}
	diff := util.TagDiff(current, desired)

	assert.Equal(t, []Change{
		{Field: "tags.env", From: "old-env", To: "new-env"},
		{Field: "tags.stale", From: "x"},
		{Field: "tags.owner", To: "example-owner"},
	}, TagChanges(current, diff))

	// Same ordering as the human summary, one entry per message.
	assert.Equal(t, []string{
		"tag env is different: old-env -> new-env",
		"tag stale:x is removed",
		"tag owner is added: example-owner",
	}, TagDiffSummary(current, diff))
}

func TestPrefixChanges(t *testing.T) {
	changes := []Change{{Field: "sslPolicy", From: "old", To: "new"}, {Field: "tags.env", To: "example"}}
	assert.Equal(t, []Change{
		{Field: "listeners[443].sslPolicy", From: "old", To: "new"},
		{Field: "listeners[443].tags.env", To: "example"},
	}, prefixChanges("listeners[443].", changes))
	assert.Nil(t, prefixChanges("listeners[443].", nil))
}
