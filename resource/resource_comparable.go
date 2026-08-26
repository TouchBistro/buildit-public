package resource

import "context"

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
