package resource

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

type EqualsResult int32

const (
	LeftZero  EqualsResult = iota - 1 // -1
	Equal                             // 0
	RightZero                         // 1
	NotEqual                          // 2
)

// ComparableResource extends the `Resource` & defines behavior
// for this type to retrieve & compare the AWS representation to
// its definition in buildit
type ComparableResource interface {
	Resource

	// Compare must implement the resource-specific comparison logic
	// for the buildit definition & the AWS representation of this
	// resource. The implemenation of Compare() must first
	// fetch the AWS resource & then make a comparison.
	//
	// This method must return a non-nil diff of type `ResourceDiff`
	// if the the definition & AWS represetnation are different;
	//
	// If the definition & the resource are identical, it must
	// return a nil as diff & a nil error must be returned
	//
	// A non-nil error value returned would indicate that the
	// equality could not be determined due to an error
	Compare(context.Context) (ResourceDiff, error)
}

// ResourceDiff represents the diffs between the buildt Resource
// and its representation in AWS.
type ResourceDiff interface {
	// description of differences as string
	Differences() []string
	// the aws resource of any type that was used for comparison
	AWSResource() any
}

// BaseResourceDiff implements the basic behavior for a ResourceDiff
// by providing a String()
type BaseResourceDiff struct {
	// publically available diff messages
	Messages []string
	// aws resources
	Resource any
}

// Differences returns a slice of strings with the differences found
func (r *BaseResourceDiff) Differences() []string {
	return r.Messages
}

// AWSResource returns the aws resource used for this diff
func (r *BaseResourceDiff) AWSResource() any {
	return r.Resource
}

// Change is one field-level difference between a resource's definition and
// its AWS representation: what the user would edit, in the definition's own
// vocabulary, with the value AWS has now and the value the definition asks
// for. Values are strings so a consumer can print or serialize them without
// knowing the resource type.
type Change struct {
	// Field is the dotted path of the setting in the YAML definition, for
	// example "port", "healthcheck.path", "tags.env" or "listeners[443].certificate".
	Field string
	// From is the current AWS value; empty when the setting is being added.
	From string
	// To is the desired value; empty when the setting is being removed.
	To string
}

// DetailedResourceDiff is a ResourceDiff that can also report its differences
// as structured Changes. Adoption is optional and per resource: a diff type
// embeds ChangeSet and records a Change beside each message it appends.
// Differences() stays the human rendering and is never derived from Changes.
type DetailedResourceDiff interface {
	ResourceDiff
	Changes() []Change
}

// ChangeSet collects Changes. Embed it in a diff struct alongside
// BaseResourceDiff to make that struct a DetailedResourceDiff.
type ChangeSet struct {
	changes []Change
}

// AddChange records one field-level change.
func (c *ChangeSet) AddChange(field, from, to string) {
	c.changes = append(c.changes, Change{Field: field, From: from, To: to})
}

// AddChanges records several changes, typically from a helper such as TagChanges.
func (c *ChangeSet) AddChanges(changes ...Change) {
	c.changes = append(c.changes, changes...)
}

// Changes returns the recorded changes in the order they were added.
func (c *ChangeSet) Changes() []Change {
	return c.changes
}

// prefixChanges returns changes with prefix prepended to every Field. A parent
// resource uses it to fold a nested sub-resource's changes under its own path,
// so a listener's "sslPolicy" becomes the load balancer's "listeners[443].sslPolicy".
func prefixChanges(prefix string, changes []Change) []Change {
	if len(changes) == 0 {
		return nil
	}
	out := make([]Change, len(changes))
	for i, c := range changes {
		out[i] = Change{Field: prefix + c.Field, From: c.From, To: c.To}
	}
	return out
}

// ChangeValue renders a value for Change.From or Change.To. Pointers are
// dereferenced and a nil pointer or nil value is the empty string, so callers
// can hand SDK fields over without unwrapping them first.
func ChangeValue(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case *string:
		if x == nil {
			return ""
		}
		return *x
	case bool:
		return strconv.FormatBool(x)
	case *bool:
		if x == nil {
			return ""
		}
		return strconv.FormatBool(*x)
	case *int32:
		if x == nil {
			return ""
		}
		return strconv.FormatInt(int64(*x), 10)
	case *int64:
		if x == nil {
			return ""
		}
		return strconv.FormatInt(*x, 10)
	case []string:
		return strings.Join(x, ",")
	case fmt.Stringer:
		return x.String()
	default:
		return fmt.Sprint(x)
	}
}
