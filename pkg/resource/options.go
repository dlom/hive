package resource

import (
	"k8s.io/apimachinery/pkg/types"
)

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
