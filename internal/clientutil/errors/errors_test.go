package errors

import (
	stderr "errors"
	"fmt"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestClusterError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *ClusterError
		expected string
	}{
		{
			name: "error without resource context",
			err: &ClusterError{
				ClusterID: "test-ns/test-cluster",
				Operation: "build-client",
				Cause:     fmt.Errorf("connection refused"),
			},
			expected: "cluster test-ns/test-cluster operation build-client failed: connection refused",
		},
		{
			name: "error with namespaced resource",
			err: &ClusterError{
				ClusterID: "test-ns/test-cluster",
				Operation: "apply",
				GVK:       schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
				Namespace: "default",
				Name:      "my-deployment",
				Cause:     fmt.Errorf("conflict"),
			},
			expected: "cluster test-ns/test-cluster operation apply on apps/v1, Kind=Deployment default/my-deployment failed: conflict",
		},
		{
			name: "error with cluster-scoped resource",
			err: &ClusterError{
				ClusterID: "test-ns/test-cluster",
				Operation: "delete",
				GVK:       schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Namespace"},
				Name:      "test-namespace",
				Cause:     fmt.Errorf("not found"),
			},
			expected: "cluster test-ns/test-cluster operation delete on /v1, Kind=Namespace test-namespace failed: not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			if got != tt.expected {
				t.Errorf("ClusterError.Error() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestClusterError_Unwrap(t *testing.T) {
	cause := fmt.Errorf("original error")
	err := &ClusterError{
		ClusterID: "test-ns/test-cluster",
		Operation: "apply",
		Cause:     cause,
	}

	unwrapped := err.Unwrap()
	if unwrapped != cause {
		t.Errorf("Unwrap() = %v, want %v", unwrapped, cause)
	}

	// Test with errors.Is
	if !stderr.Is(err, cause) {
		t.Error("errors.Is() should return true for wrapped error")
	}
}

func TestWrapClusterError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		clusterID string
		operation string
		gvk       schema.GroupVersionKind
		namespace string
		resName   string
		wantNil   bool
	}{
		{
			name:      "nil error returns nil",
			err:       nil,
			clusterID: "test-ns/test-cluster",
			operation: "apply",
			wantNil:   true,
		},
		{
			name:      "wraps regular error",
			err:       fmt.Errorf("test error"),
			clusterID: "test-ns/test-cluster",
			operation: "patch",
			gvk:       schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
			namespace: "default",
			resName:   "my-deployment",
			wantNil:   false,
		},
		{
			name: "preserves existing ClusterError",
			err: &ClusterError{
				ClusterID: "existing-ns/existing-cluster",
				Operation: "existing-op",
				Cause:     fmt.Errorf("original cause"),
			},
			clusterID: "new-ns/new-cluster",
			operation: "new-op",
			wantNil:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WrapClusterError(tt.err, tt.clusterID, tt.operation, tt.gvk, tt.namespace, tt.resName)

			if tt.wantNil {
				if got != nil {
					t.Errorf("WrapClusterError() = %v, want nil", got)
				}
				return
			}

			if got == nil {
				t.Fatal("WrapClusterError() = nil, want non-nil")
			}

			var ce *ClusterError
			if !stderr.As(got, &ce) {
				t.Fatalf("WrapClusterError() did not return a ClusterError")
			}

			// If wrapping an existing ClusterError, it should preserve the original context
			if existingCE, ok := tt.err.(*ClusterError); ok {
				if ce.ClusterID != existingCE.ClusterID {
					t.Errorf("ClusterID = %q, want %q", ce.ClusterID, existingCE.ClusterID)
				}
				if ce.Operation != existingCE.Operation {
					t.Errorf("Operation = %q, want %q", ce.Operation, existingCE.Operation)
				}
			} else {
				// New wrap should have the provided context
				if ce.ClusterID != tt.clusterID {
					t.Errorf("ClusterID = %q, want %q", ce.ClusterID, tt.clusterID)
				}
				if ce.Operation != tt.operation {
					t.Errorf("Operation = %q, want %q", ce.Operation, tt.operation)
				}
			}

			// Unwrap should work
			if ce.Cause == nil && tt.err != nil {
				t.Error("Cause should not be nil")
			}
		})
	}
}

func TestAsClusterError(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		wantOk bool
	}{
		{
			name:   "nil error",
			err:    nil,
			wantOk: false,
		},
		{
			name:   "regular error",
			err:    fmt.Errorf("test error"),
			wantOk: false,
		},
		{
			name: "ClusterError",
			err: &ClusterError{
				ClusterID: "test-ns/test-cluster",
				Operation: "apply",
				Cause:     fmt.Errorf("test"),
			},
			wantOk: true,
		},
		{
			name: "wrapped ClusterError",
			err: fmt.Errorf("outer: %w", &ClusterError{
				ClusterID: "test-ns/test-cluster",
				Operation: "apply",
				Cause:     fmt.Errorf("inner"),
			}),
			wantOk: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ce *ClusterError
			got := AsClusterError(tt.err, &ce)

			if got != tt.wantOk {
				t.Errorf("AsClusterError() = %v, want %v", got, tt.wantOk)
			}

			if tt.wantOk && ce == nil {
				t.Error("AsClusterError() returned true but ce is nil")
			}
		})
	}
}

func TestErrorChaining(t *testing.T) {
	// Create a chain of errors
	originalErr := apierrors.NewNotFound(schema.GroupResource{Group: "apps", Resource: "deployments"}, "test-deployment")
	wrappedErr := WrapClusterError(originalErr, "test-ns/test-cluster", "apply",
		schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
		"default", "test-deployment")
	doubleWrappedErr := fmt.Errorf("additional context: %w", wrappedErr)

	// Test that apierrors.IsNotFound works through the chain
	if !apierrors.IsNotFound(doubleWrappedErr) {
		t.Error("apierrors.IsNotFound() should return true for wrapped NotFound error")
	}

	// Test that errors.Is works
	if !stderr.Is(doubleWrappedErr, originalErr) {
		t.Error("errors.Is() should return true for original error through chain")
	}

	// Test that errors.As works for ClusterError
	var ce *ClusterError
	if !stderr.As(doubleWrappedErr, &ce) {
		t.Fatal("errors.As() should find ClusterError in chain")
	}

	if ce.ClusterID != "test-ns/test-cluster" {
		t.Errorf("ClusterID = %q, want %q", ce.ClusterID, "test-ns/test-cluster")
	}
}
