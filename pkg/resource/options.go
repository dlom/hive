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

// patchOptions configures Patch operations.
type patchOptions struct {
	fieldManager string
	patchType    types.PatchType
}

// PatchOption is a functional option for configuring Patch operations.
type PatchOption func(*patchOptions)

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
