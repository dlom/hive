package resource

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/hive/internal/clientutil"
)

// Patch patches the given resource using the specified patch type.
// This replaces the kubectl-based patch which had the os.Args mutation bug.
//
// NO os.Args MUTATION! Field manager is passed via PatchOptions.
//
// Supported patch types:
//   - types.StrategicMergePatchType (default) - Understands Kubernetes API semantics
//   - types.MergePatchType (JSON Merge Patch - RFC 7386) - Simple merge
//   - types.JSONPatchType (JSON Patch - RFC 6902) - Array of operations
//   - types.ApplyPatchType (Server-Side Apply) - Recommended
func (h *helperImpl) Patch(ctx context.Context, obj interface{}, patch []byte, opts ...PatchOption) (PatchResult, error) {
	startTime := time.Now()

	// Parse options
	options := patchOptions{
		fieldManager: clientutil.FieldManagerName(h.controllerName),
		patchType:    types.StrategicMergePatchType, // Default
	}
	for _, opt := range opts {
		opt(&options)
	}

	// Convert input to unstructured
	unstructuredObj, gvk, err := h.toUnstructured(obj)
	if err != nil {
		return PatchResult{}, h.wrapError(err, "parse-object", gvk, "", "")
	}

	objectKey := client.ObjectKeyFromObject(unstructuredObj)

	// Prepare patch options
	patchOpts := []client.PatchOption{
		&client.PatchOptions{
			FieldManager: options.fieldManager,
		},
	}

	// Perform patch
	patchObj := client.RawPatch(options.patchType, patch)
	if err := h.client.Patch(ctx, unstructuredObj, patchObj, patchOpts...); err != nil {
		// Record operation metrics
		h.recordOperation("patch", gvk, "failure", time.Since(startTime).Seconds())

		return PatchResult{}, h.wrapError(err, "patch", gvk, objectKey.Namespace, objectKey.Name)
	}

	// Record success metrics
	h.recordOperation("patch", gvk, "success", time.Since(startTime).Seconds())

	// For now, we assume Patched if no error
	// A more sophisticated implementation could compare before/after
	return PatchResult{
		State:  Patched,
		Object: unstructuredObj,
	}, nil
}

// PatchWithObject patches a resource by name and kind using the patch data.
// This is useful when you have the resource identity but not the full object.
func (h *helperImpl) PatchWithObject(
	ctx context.Context,
	gvk schema.GroupVersionKind,
	namespace, name string,
	patch []byte,
	opts ...PatchOption,
) (PatchResult, error) {
	// Create minimal unstructured object with identity
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	obj.SetNamespace(namespace)
	obj.SetName(name)

	return h.Patch(ctx, obj, patch, opts...)
}
