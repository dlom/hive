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

func (h *helperImpl) Patch(ctx context.Context, obj interface{}, patch []byte, opts ...PatchOption) (PatchResult, error) {
	startTime := time.Now()

	options := patchOptions{
		fieldManager: clientutil.FieldManagerName(h.controllerName),
		patchType:    types.StrategicMergePatchType,
	}
	for _, opt := range opts {
		opt(&options)
	}

	unstructuredObj, gvk, err := h.toUnstructured(obj)
	if err != nil {
		return PatchResult{}, h.wrapError(err, "parse-object", gvk, "", "")
	}

	objectKey := client.ObjectKeyFromObject(unstructuredObj)

	existingObj := &unstructured.Unstructured{}
	existingObj.SetGroupVersionKind(gvk)
	if err := h.client.Get(ctx, objectKey, existingObj); err != nil {
		h.recordOperation("patch", gvk, "failure", time.Since(startTime).Seconds())
		return PatchResult{}, h.wrapError(err, "get-before-patch", gvk, objectKey.Namespace, objectKey.Name)
	}
	existingResourceVersion := existingObj.GetResourceVersion()

	patchOpts := []client.PatchOption{
		&client.PatchOptions{
			FieldManager: options.fieldManager,
		},
	}

	patchObj := client.RawPatch(options.patchType, patch)
	if err := h.client.Patch(ctx, unstructuredObj, patchObj, patchOpts...); err != nil {
		h.recordOperation("patch", gvk, "failure", time.Since(startTime).Seconds())

		return PatchResult{}, h.wrapError(err, "patch", gvk, objectKey.Namespace, objectKey.Name)
	}

	h.recordOperation("patch", gvk, "success", time.Since(startTime).Seconds())

	state := Patched
	if unstructuredObj.GetResourceVersion() == existingResourceVersion {
		state = PatchUnchanged
	}

	return PatchResult{
		State: state,
	}, nil
}

func (h *helperImpl) PatchWithObject(
	ctx context.Context,
	gvk schema.GroupVersionKind,
	namespace, name string,
	patch []byte,
	opts ...PatchOption,
) (PatchResult, error) {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	obj.SetNamespace(namespace)
	obj.SetName(name)

	return h.Patch(ctx, obj, patch, opts...)
}
