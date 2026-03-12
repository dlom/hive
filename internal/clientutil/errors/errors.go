package errors

import (
	stderr "errors"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ClusterError wraps errors with cluster and operation context for better debugging
// and error handling in remote cluster operations.
type ClusterError struct {
	ClusterID string                  // namespace/name of the cluster
	Operation string                  // operation being performed (e.g., "apply", "patch", "delete", "build-client")
	GVK       schema.GroupVersionKind // resource GroupVersionKind if applicable
	Namespace string                  // resource namespace if applicable
	Name      string                  // resource name if applicable
	Cause     error                   // underlying error
}

// Error implements the error interface
func (e *ClusterError) Error() string {
	if e.GVK.Empty() {
		// No resource context, just cluster and operation
		return fmt.Sprintf("cluster %s operation %s failed: %v", e.ClusterID, e.Operation, e.Cause)
	}

	if e.Namespace != "" {
		return fmt.Sprintf("cluster %s operation %s on %s %s/%s failed: %v",
			e.ClusterID, e.Operation, e.GVK.String(), e.Namespace, e.Name, e.Cause)
	}

	return fmt.Sprintf("cluster %s operation %s on %s %s failed: %v",
		e.ClusterID, e.Operation, e.GVK.String(), e.Name, e.Cause)
}

// Unwrap returns the underlying error for use with errors.Is and errors.As
func (e *ClusterError) Unwrap() error {
	return e.Cause
}

// WrapClusterError wraps an error with cluster and operation context.
// If err is nil, returns nil.
// If err is already a ClusterError, updates the context and returns it.
func WrapClusterError(err error, clusterID, operation string, gvk schema.GroupVersionKind, namespace, name string) error {
	if err == nil {
		return nil
	}

	// If already a ClusterError, update context if needed
	var ce *ClusterError
	if AsClusterError(err, &ce) {
		// Preserve the most specific error context
		if ce.ClusterID == "" {
			ce.ClusterID = clusterID
		}
		if ce.Operation == "" {
			ce.Operation = operation
		}
		return ce
	}

	return &ClusterError{
		ClusterID: clusterID,
		Operation: operation,
		GVK:       gvk,
		Namespace: namespace,
		Name:      name,
		Cause:     err,
	}
}

// AsClusterError is a convenience wrapper for errors.As for ClusterError
func AsClusterError(err error, target **ClusterError) bool {
	if err == nil {
		return false
	}
	*target = &ClusterError{}
	return stderr.As(err, target)
}
