package resource

import "context"

// WaitableResource is a resource that allows waiting for it to be in a desired state.
//
// Some resources might perform additional operations in the background after Apply which
// need to be waited on. For example, after updating an ECS service it might take some time
// for it to create and fully cutover to new tasks. For this situation Wait could be used to
// wait for the service deployment to be complete.
//
// Calls to Wait should not have dependencies on other resources. It should be safe to call at
// any time in any order, and it should be safe to call concurrently. If an operation needs to
// be completed for dependants than it should not be performed in Wait but rather in Apply.
type WaitableResource interface {
	Resource
	// Wait waits for any pending operations to be completed.
	//
	// The provided context can be used to stop waiting if it
	// becomes done before Wait completes on its own.
	Wait(ctx context.Context) error
}
