package resource

import (
	"context"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
//
// With WithWait() option, polls until fully deleted or context timeout.
func (h *helperImpl) Delete(ctx context.Context, gvk schema.GroupVersionKind, namespace, name string, opts ...DeleteOption) (DeleteResult, error) {
	startTime := time.Now()

	// Parse options
	options := deleteOptions{}
	for _, opt := range opts {
		opt(&options)
	}

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
		if options.wait {
			// Wait for deletion to complete
			return h.waitForDeletion(ctx, gvk, objectKey, currentObj.GetDeletionTimestamp())
		}

		// Already deleting, return DeletionInProgress
		h.recordOperation("delete", gvk, "deletion-in-progress", time.Since(startTime).Seconds())
		return DeleteResult{
			State:             DeletionInProgress,
			DeletionTimestamp: currentObj.GetDeletionTimestamp(),
			Object:            currentObj,
		}, nil
	}

	// Prepare delete options
	deleteOpts := []client.DeleteOption{}

	if options.gracePeriodSeconds != nil {
		deleteOpts = append(deleteOpts, &client.DeleteOptions{
			GracePeriodSeconds: options.gracePeriodSeconds,
		})
	}

	if options.propagationPolicy != nil {
		deleteOpts = append(deleteOpts, &client.DeleteOptions{
			PropagationPolicy: options.propagationPolicy,
		})
	}

	// Delete the resource
	if err := h.client.Delete(ctx, obj, deleteOpts...); err != nil {
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

	// Wait for deletion if requested
	if options.wait {
		return h.waitForDeletion(ctx, gvk, objectKey, nil)
	}

	// Delete request succeeded
	h.recordOperation("delete", gvk, "success", time.Since(startTime).Seconds())
	return DeleteResult{
		State: Deleted,
	}, nil
}

// waitForDeletion polls until the resource is fully deleted or context times out.
func (h *helperImpl) waitForDeletion(
	ctx context.Context,
	gvk schema.GroupVersionKind,
	objectKey client.ObjectKey,
	deletionTimestamp *metav1.Time,
) (DeleteResult, error) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Context timeout or cancellation
			return DeleteResult{
				State:             DeletionInProgress,
				DeletionTimestamp: deletionTimestamp,
			}, ctx.Err()

		case <-ticker.C:
			// Check if resource still exists
			obj := &unstructured.Unstructured{}
			obj.SetGroupVersionKind(gvk)

			if err := h.client.Get(ctx, objectKey, obj); err != nil {
				if apierrors.IsNotFound(err) {
					// Successfully deleted
					return DeleteResult{
						State: Deleted,
					}, nil
				}
				// Unexpected error
				return DeleteResult{}, h.wrapError(err, "poll-deletion", gvk, objectKey.Namespace, objectKey.Name)
			}

			// Still exists, update deletion timestamp if we see it
			if obj.GetDeletionTimestamp() != nil {
				deletionTimestamp = obj.GetDeletionTimestamp()
			}

			// Continue polling
		}
	}
}
