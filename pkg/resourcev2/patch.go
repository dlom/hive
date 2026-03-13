package resourcev2

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (h *helperImpl) Patch(
	ctx context.Context,
	gvk schema.GroupVersionKind,
	namespace, name string,
	patch []byte,
	patchType types.PatchType,
) (PatchState, error) {
	startTime := time.Now()

	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	obj.SetNamespace(namespace)
	obj.SetName(name)

	objectKey := client.ObjectKey{Namespace: namespace, Name: name}

	existingObj := &unstructured.Unstructured{}
	existingObj.SetGroupVersionKind(gvk)
	if err := h.client.Get(ctx, objectKey, existingObj); err != nil {
		h.recordOperation("patch", gvk, "failure", time.Since(startTime).Seconds())
		return 0, h.wrapError(err, "get-before-patch", gvk, namespace, name)
	}
	existingResourceVersion := existingObj.GetResourceVersion()

	patchOpts := []client.PatchOption{
		&client.PatchOptions{
			FieldManager: FieldManagerName(h.controllerName),
		},
	}

	patchObj := client.RawPatch(patchType, patch)
	if err := h.client.Patch(ctx, obj, patchObj, patchOpts...); err != nil {
		h.recordOperation("patch", gvk, "failure", time.Since(startTime).Seconds())
		return 0, h.wrapError(err, "patch", gvk, namespace, name)
	}

	h.recordOperation("patch", gvk, "success", time.Since(startTime).Seconds())

	state := Patched
	if obj.GetResourceVersion() == existingResourceVersion {
		state = PatchUnchanged
	}

	return state, nil
}
