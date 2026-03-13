package resource

import (
	"context"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Delete deletes the specified resource with clear deletion semantics.
// This fixes the legacy deletion timing bug where DeletionInProgress was ambiguous.
//
// Returns clear states:
//   - Deleted: Successfully deleted or already gone (idempotent success)
//   - NotFound: Resource never existed
//   - DeletionInProgress: Has deletionTimestamp but still exists (finalizers)
func (h *helperImpl) Delete(ctx context.Context, gvk schema.GroupVersionKind, namespace, name string) (DeleteResult, error) {
	startTime := time.Now()

	// Create object for deletion
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	obj.SetNamespace(namespace)
	obj.SetName(name)

	objectKey := client.ObjectKey{Namespace: namespace, Name: name}

	// Check current state
	currentObj := &unstructured.Unstructured{}
	currentObj.SetGroupVersionKind(gvk)

	if err := h.client.Get(ctx, objectKey, currentObj); err != nil {
		if apierrors.IsNotFound(err) {
			// Resource never existed or already deleted
			h.recordOperation("delete", gvk, "success", time.Since(startTime).Seconds())
			return DeleteResult{
				State: NotFound,
			}, nil
		}
		return DeleteResult{}, h.wrapError(err, "get-for-delete", gvk, namespace, name)
	}

	// Check if already deleting
	if currentObj.GetDeletionTimestamp() != nil {
		// Already deleting, return DeletionInProgress
		h.recordOperation("delete", gvk, "deletion-in-progress", time.Since(startTime).Seconds())
		return DeleteResult{
			State: DeletionInProgress,
		}, nil
	}

	// Delete the resource
	if err := h.client.Delete(ctx, obj); err != nil {
		if apierrors.IsNotFound(err) {
			// Already deleted (race condition)
			h.recordOperation("delete", gvk, "success", time.Since(startTime).Seconds())
			return DeleteResult{
				State: Deleted,
			}, nil
		}

		h.recordOperation("delete", gvk, "failure", time.Since(startTime).Seconds())
		return DeleteResult{}, h.wrapError(err, "delete", gvk, namespace, name)
	}

	// Delete request succeeded
	h.recordOperation("delete", gvk, "success", time.Since(startTime).Seconds())
	return DeleteResult{
		State: Deleted,
	}, nil
}
