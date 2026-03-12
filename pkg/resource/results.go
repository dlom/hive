package resource

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ApplyState represents the state of a resource after an Apply operation.
type ApplyState int

const (
	// Created indicates the resource was created by the Apply operation.
	Created ApplyState = iota

	// Configured indicates the resource already existed and was updated by the Apply operation.
	Configured

	// Unchanged indicates the resource already existed in the desired state and no update was needed.
	Unchanged
)

// String returns a string representation of the ApplyState.
func (s ApplyState) String() string {
	switch s {
	case Created:
		return "created"
	case Configured:
		return "configured"
	case Unchanged:
		return "unchanged"
	default:
		return "unknown"
	}
}

// ApplyResult contains the result of an Apply operation.
type ApplyResult struct {
	// State indicates whether the resource was created, configured, or unchanged.
	State ApplyState

	// Object is the resource after the apply operation.
	Object runtime.Object

	// GVK is the GroupVersionKind of the resource.
	GVK schema.GroupVersionKind
}

// PatchState represents the state of a resource after a Patch operation.
type PatchState int

const (
	// Patched indicates the resource was successfully patched.
	Patched PatchState = iota

	// PatchUnchanged indicates the patch resulted in no changes to the resource.
	PatchUnchanged
)

// String returns a string representation of the PatchState.
func (s PatchState) String() string {
	switch s {
	case Patched:
		return "patched"
	case PatchUnchanged:
		return "unchanged"
	default:
		return "unknown"
	}
}

// PatchResult contains the result of a Patch operation.
type PatchResult struct {
	// State indicates whether the resource was patched or unchanged.
	State PatchState

	// Object is the resource after the patch operation.
	Object runtime.Object
}

// DeleteState represents the state of a resource after a Delete operation.
type DeleteState int

const (
	// Deleted indicates the resource was successfully deleted or was already gone.
	// This is the success state for idempotent deletion.
	Deleted DeleteState = iota

	// NotFound indicates the resource never existed.
	// This allows callers to distinguish "deleted what was there" from "nothing to delete".
	NotFound

	// DeletionInProgress indicates the resource has a deletionTimestamp but still exists.
	// This occurs when finalizers are present or graceful deletion is in progress.
	DeletionInProgress
)

// String returns a string representation of the DeleteState.
func (s DeleteState) String() string {
	switch s {
	case Deleted:
		return "deleted"
	case NotFound:
		return "not-found"
	case DeletionInProgress:
		return "deletion-in-progress"
	default:
		return "unknown"
	}
}

// DeleteResult contains the result of a Delete operation.
type DeleteResult struct {
	// State indicates the deletion state.
	State DeleteState

	// DeletionTimestamp is when deletion was requested (for DeletionInProgress state).
	DeletionTimestamp *metav1.Time

	// Object is the final state of the resource if available.
	Object runtime.Object
}

// IsDeleted returns true if the resource is deleted or was not found.
// This is useful for callers who treat both states as "successfully removed".
func (r DeleteResult) IsDeleted() bool {
	return r.State == Deleted || r.State == NotFound
}
