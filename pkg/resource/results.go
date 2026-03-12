package resource

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ApplyStateV2 represents the state of a resource after an Apply operation.
type ApplyStateV2 int

const (
	// CreatedV2 indicates the resource was created by the Apply operation.
	CreatedV2 ApplyStateV2 = iota

	// ConfiguredV2 indicates the resource already existed and was updated by the Apply operation.
	ConfiguredV2

	// UnchangedV2 indicates the resource already existed in the desired state and no update was needed.
	UnchangedV2
)

// String returns a string representation of the ApplyStateV2.
func (s ApplyStateV2) String() string {
	switch s {
	case CreatedV2:
		return "created"
	case ConfiguredV2:
		return "configured"
	case UnchangedV2:
		return "unchanged"
	default:
		return "unknown"
	}
}

// ApplyResultV2 contains the result of an Apply operation.
type ApplyResultV2 struct {
	// State indicates whether the resource was created, configured, or unchanged.
	State ApplyStateV2

	// Object is the resource after the apply operation.
	Object runtime.Object

	// GVK is the GroupVersionKind of the resource.
	GVK schema.GroupVersionKind
}

// PatchStateV2 represents the state of a resource after a Patch operation.
type PatchStateV2 int

const (
	// PatchedV2 indicates the resource was successfully patched.
	PatchedV2 PatchStateV2 = iota

	// PatchUnchangedV2 indicates the patch resulted in no changes to the resource.
	PatchUnchangedV2
)

// String returns a string representation of the PatchStateV2.
func (s PatchStateV2) String() string {
	switch s {
	case PatchedV2:
		return "patched"
	case PatchUnchangedV2:
		return "unchanged"
	default:
		return "unknown"
	}
}

// PatchResultV2 contains the result of a Patch operation.
type PatchResultV2 struct {
	// State indicates whether the resource was patched or unchanged.
	State PatchStateV2

	// Object is the resource after the patch operation.
	Object runtime.Object
}

// DeleteStateV2 represents the state of a resource after a Delete operation.
type DeleteStateV2 int

const (
	// DeletedV2 indicates the resource was successfully deleted or was already gone.
	// This is the success state for idempotent deletion.
	DeletedV2 DeleteStateV2 = iota

	// NotFoundV2 indicates the resource never existed.
	// This allows callers to distinguish "deleted what was there" from "nothing to delete".
	NotFoundV2

	// DeletionInProgressV2 indicates the resource has a deletionTimestamp but still exists.
	// This occurs when finalizers are present or graceful deletion is in progress.
	DeletionInProgressV2
)

// String returns a string representation of the DeleteStateV2.
func (s DeleteStateV2) String() string {
	switch s {
	case DeletedV2:
		return "deleted"
	case NotFoundV2:
		return "not-found"
	case DeletionInProgressV2:
		return "deletion-in-progress"
	default:
		return "unknown"
	}
}

// DeleteResultV2 contains the result of a Delete operation.
type DeleteResultV2 struct {
	// State indicates the deletion state.
	State DeleteStateV2

	// DeletionTimestamp is when deletion was requested (for DeletionInProgress state).
	DeletionTimestamp *metav1.Time

	// Object is the final state of the resource if available.
	Object runtime.Object
}

// IsDeleted returns true if the resource is deleted or was not found.
// This is useful for callers who treat both states as "successfully removed".
func (r DeleteResultV2) IsDeleted() bool {
	return r.State == DeletedV2 || r.State == NotFoundV2
}
