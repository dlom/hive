package resource

import (
	"context"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (h *helperImpl) Delete(ctx context.Context, gvk schema.GroupVersionKind, namespace, name string) (DeleteResult, error) {
	startTime := time.Now()

	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	obj.SetNamespace(namespace)
	obj.SetName(name)

	objectKey := client.ObjectKey{Namespace: namespace, Name: name}

	currentObj := &unstructured.Unstructured{}
	currentObj.SetGroupVersionKind(gvk)

	if err := h.client.Get(ctx, objectKey, currentObj); err != nil {
		if apierrors.IsNotFound(err) {
			h.recordOperation("delete", gvk, "success", time.Since(startTime).Seconds())
			return DeleteResult{
				State: NotFound,
			}, nil
		}
		return DeleteResult{}, h.wrapError(err, "get-for-delete", gvk, namespace, name)
	}

	if currentObj.GetDeletionTimestamp() != nil {
		h.recordOperation("delete", gvk, "deletion-in-progress", time.Since(startTime).Seconds())
		return DeleteResult{
			State: DeletionInProgress,
		}, nil
	}

	if err := h.client.Delete(ctx, obj); err != nil {
		if apierrors.IsNotFound(err) {
			h.recordOperation("delete", gvk, "success", time.Since(startTime).Seconds())
			return DeleteResult{
				State: Deleted,
			}, nil
		}

		h.recordOperation("delete", gvk, "failure", time.Since(startTime).Seconds())
		return DeleteResult{}, h.wrapError(err, "delete", gvk, namespace, name)
	}

	h.recordOperation("delete", gvk, "success", time.Since(startTime).Seconds())
	return DeleteResult{
		State: Deleted,
	}, nil
}
