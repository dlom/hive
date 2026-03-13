package resource

import (
	"bytes"
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/hive/internal/clientutil"
	"github.com/openshift/hive/pkg/util/scheme"
)

func (h *helperImpl) Apply(ctx context.Context, obj interface{}) (ApplyState, error) {
	startTime := time.Now()

	unstructuredObj, gvk, err := h.toUnstructured(obj)
	if err != nil {
		return 0, h.wrapError(err, "parse-object", gvk, "", "")
	}

	existingObj := &unstructured.Unstructured{}
	existingObj.SetGroupVersionKind(gvk)
	objectKey := client.ObjectKeyFromObject(unstructuredObj)

	exists := false
	var existingResourceVersion string
	if err := h.client.Get(ctx, objectKey, existingObj); err != nil {
		if !apierrors.IsNotFound(err) {
			return 0, h.wrapError(err, "get-existing", gvk, objectKey.Namespace, objectKey.Name)
		}
	} else {
		exists = true
		existingResourceVersion = existingObj.GetResourceVersion()
	}

	patchOpts := []client.PatchOption{
		client.ForceOwnership,
		client.FieldOwner(clientutil.FieldManagerName(h.controllerName)),
	}

	if err := h.client.Patch(ctx, unstructuredObj, client.Apply, patchOpts...); err != nil {
		h.recordOperation("apply", gvk, "failure", time.Since(startTime).Seconds())
		return 0, h.wrapError(err, "apply", gvk, objectKey.Namespace, objectKey.Name)
	}

	var state ApplyState
	if !exists {
		state = Created
	} else if unstructuredObj.GetResourceVersion() == existingResourceVersion {
		state = Unchanged
	} else {
		state = Configured
	}

	h.recordOperation("apply", gvk, "success", time.Since(startTime).Seconds())
	return state, nil
}

func (h *helperImpl) toUnstructured(obj interface{}) (*unstructured.Unstructured, schema.GroupVersionKind, error) {
	switch v := obj.(type) {
	case []byte:
		return h.parseBytes(v)
	case *unstructured.Unstructured:
		return v, v.GroupVersionKind(), nil
	case runtime.Object:
		return h.runtimeObjectToUnstructured(v)

	default:
		return nil, schema.GroupVersionKind{}, fmt.Errorf("unsupported object type: %T", obj)
	}
}

func (h *helperImpl) parseBytes(data []byte) (*unstructured.Unstructured, schema.GroupVersionKind, error) {
	obj := &unstructured.Unstructured{}
	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), 4096)

	if err := decoder.Decode(obj); err != nil {
		return nil, schema.GroupVersionKind{}, fmt.Errorf("failed to decode YAML/JSON: %w", err)
	}

	gvk := obj.GroupVersionKind()
	if gvk.Empty() {
		return nil, schema.GroupVersionKind{}, fmt.Errorf("object missing apiVersion or kind")
	}

	return obj, gvk, nil
}

func (h *helperImpl) runtimeObjectToUnstructured(obj runtime.Object) (*unstructured.Unstructured, schema.GroupVersionKind, error) {
	gvks, _, err := scheme.GetScheme().ObjectKinds(obj)
	if err != nil {
		return nil, schema.GroupVersionKind{}, fmt.Errorf("failed to get object kind: %w", err)
	}
	if len(gvks) == 0 {
		return nil, schema.GroupVersionKind{}, fmt.Errorf("object has no kind")
	}
	gvk := gvks[0]

	unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, gvk, fmt.Errorf("failed to convert to unstructured: %w", err)
	}

	result := &unstructured.Unstructured{Object: unstructuredObj}
	result.SetGroupVersionKind(gvk)

	return result, gvk, nil
}
