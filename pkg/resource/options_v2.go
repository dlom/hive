package resource

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// applyOptions configures Apply operations.
type applyOptions struct {
	fieldManager string
	force        bool
	dryRun       bool
}

// ApplyOption is a functional option for configuring Apply operations.
type ApplyOption func(*applyOptions)

// WithFieldManager sets a custom field manager name for the Apply operation.
// If not specified, the field manager from the helper configuration is used.
func WithFieldManager(name string) ApplyOption {
	return func(opts *applyOptions) {
		opts.fieldManager = name
	}
}

// WithForce enables force apply, which takes ownership of fields managed by other field managers.
// This should be used carefully as it can overwrite changes made by other controllers.
// Default: false (fails on conflicts)
func WithForce() ApplyOption {
	return func(opts *applyOptions) {
		opts.force = true
	}
}

// WithDryRun enables dry-run mode, which validates the apply without persisting changes.
// Useful for validation and preview of changes.
func WithDryRun() ApplyOption {
	return func(opts *applyOptions) {
		opts.dryRun = true
	}
}

// patchOptions configures Patch operations.
type patchOptions struct {
	fieldManager string
	patchType    types.PatchType
}

// PatchOption is a functional option for configuring Patch operations.
type PatchOption func(*patchOptions)

// WithPatchFieldManager sets a custom field manager name for the Patch operation.
// If not specified, the field manager from the helper configuration is used.
func WithPatchFieldManager(name string) PatchOption {
	return func(opts *patchOptions) {
		opts.fieldManager = name
	}
}

// WithPatchType sets the patch type for the Patch operation.
// Supported types:
//   - types.StrategicMergePatchType (default)
//   - types.MergePatchType (JSON Merge Patch - RFC 7386)
//   - types.JSONPatchType (JSON Patch - RFC 6902)
//   - types.ApplyPatchType (Server-Side Apply)
func WithPatchType(pt types.PatchType) PatchOption {
	return func(opts *patchOptions) {
		opts.patchType = pt
	}
}

// deleteOptions configures Delete operations.
type deleteOptions struct {
	gracePeriodSeconds *int64
	propagationPolicy  *metav1.DeletionPropagation
	wait               bool
}

// DeleteOption is a functional option for configuring Delete operations.
type DeleteOption func(*deleteOptions)

// WithGracePeriod sets the grace period for deletion in seconds.
// The grace period is the duration in seconds before the object should be deleted.
// If not specified, the default grace period for the resource type is used.
func WithGracePeriod(seconds int64) DeleteOption {
	return func(opts *deleteOptions) {
		opts.gracePeriodSeconds = &seconds
	}
}

// WithPropagationPolicy sets the propagation policy for deletion.
// Determines how dependent objects are deleted:
//   - Orphan: Dependent objects are not deleted
//   - Background: Dependent objects are deleted in the background
//   - Foreground: Dependent objects are deleted before the parent
func WithPropagationPolicy(policy metav1.DeletionPropagation) DeleteOption {
	return func(opts *deleteOptions) {
		opts.propagationPolicy = &policy
	}
}

// WithWait enables waiting for deletion to complete.
// The delete operation will poll until the resource is fully removed or the context times out.
// Without this option, delete returns immediately after requesting deletion (even if finalizers exist).
func WithWait() DeleteOption {
	return func(opts *deleteOptions) {
		opts.wait = true
	}
}
