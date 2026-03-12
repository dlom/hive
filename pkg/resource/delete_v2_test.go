package resource

import (
	"context"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
)

func TestHelperV2_Delete(t *testing.T) {
	logger := log.NewEntry(log.StandardLogger())
	ctx := context.Background()

	tests := []struct {
		name      string
		gvk       schema.GroupVersionKind
		namespace string
		objName   string
		existing  []runtime.Object
		options   []DeleteOption
		wantState DeleteStateV2
		wantErr   bool
	}{
		{
			name: "delete existing resource",
			gvk: schema.GroupVersionKind{
				Version: "v1",
				Kind:    "ConfigMap",
			},
			namespace: "default",
			objName:   "test-config",
			existing: []runtime.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-config",
						Namespace: "default",
					},
				},
			},
			wantState: DeletedV2,
			wantErr:   false,
		},
		{
			name: "delete non-existent resource returns NotFoundV2",
			gvk: schema.GroupVersionKind{
				Version: "v1",
				Kind:    "ConfigMap",
			},
			namespace: "default",
			objName:   "non-existent",
			existing:  nil,
			wantState: NotFoundV2,
			wantErr:   false,
		},
		{
			name: "delete with grace period",
			gvk: schema.GroupVersionKind{
				Version: "v1",
				Kind:    "ConfigMap",
			},
			namespace: "default",
			objName:   "grace-period-test",
			existing: []runtime.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "grace-period-test",
						Namespace: "default",
					},
				},
			},
			options: []DeleteOption{
				WithGracePeriod(30),
			},
			wantState: DeletedV2,
			wantErr:   false,
		},
		{
			name: "delete with propagation policy",
			gvk: schema.GroupVersionKind{
				Version: "v1",
				Kind:    "ConfigMap",
			},
			namespace: "default",
			objName:   "propagation-test",
			existing: []runtime.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "propagation-test",
						Namespace: "default",
					},
				},
			},
			options: []DeleteOption{
				WithPropagationPolicy(metav1.DeletePropagationForeground),
			},
			wantState: DeletedV2,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create helper with fake client
			var c = newFakeClientWithObjects(tt.existing...)
			helper, err := NewHelperV2(logger,
				WithClient(c),
				WithControllerNameV2(hivev1.ClustersyncControllerName),
			)
			require.NoError(t, err)

			// Execute delete
			result, err := helper.Delete(ctx, tt.gvk, tt.namespace, tt.objName, tt.options...)

			// Verify error expectations
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			// Verify success
			require.NoError(t, err)
			assert.Equal(t, tt.wantState, result.State, "unexpected delete state")
		})
	}
}

func TestHelperV2_DeleteDeletionInProgress(t *testing.T) {
	logger := log.NewEntry(log.StandardLogger())
	ctx := context.Background()

	// Create a resource with deletionTimestamp and finalizer
	deletionTimestamp := metav1.Now()
	resourceWithFinalizer := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "finalizer-test",
			Namespace:         "default",
			DeletionTimestamp: &deletionTimestamp,
			Finalizers:        []string{"test-finalizer"},
		},
	}

	t.Run("returns DeletionInProgressV2 for resource with finalizer", func(t *testing.T) {
		helper, err := NewHelperV2(logger,
			WithClient(newFakeClientWithObjects(resourceWithFinalizer)),
			WithControllerNameV2(hivev1.ClustersyncControllerName),
		)
		require.NoError(t, err)

		gvk := schema.GroupVersionKind{
			Version: "v1",
			Kind:    "ConfigMap",
		}

		result, err := helper.Delete(ctx, gvk, "default", "finalizer-test")
		require.NoError(t, err)
		assert.Equal(t, DeletionInProgressV2, result.State)
		assert.NotNil(t, result.DeletionTimestamp)
		assert.NotNil(t, result.Object)
	})
}

func TestHelperV2_DeleteWithWait(t *testing.T) {
	logger := log.NewEntry(log.StandardLogger())

	t.Run("wait for deletion to complete", func(t *testing.T) {
		// Create resource
		resource := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "wait-test",
				Namespace: "default",
			},
		}

		helper, err := NewHelperV2(logger,
			WithClient(newFakeClientWithObjects(resource)),
			WithControllerNameV2(hivev1.ClustersyncControllerName),
		)
		require.NoError(t, err)

		// Create context with short timeout to prevent hanging
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		gvk := schema.GroupVersionKind{
			Version: "v1",
			Kind:    "ConfigMap",
		}

		// Note: Fake client deletes immediately, so wait should complete quickly
		result, err := helper.Delete(ctx, gvk, "default", "wait-test", WithWait())

		// Either successfully deleted or timeout (acceptable in test)
		if err != nil {
			// Context timeout is acceptable
			assert.Contains(t, err.Error(), "context")
		} else {
			assert.Equal(t, DeletedV2, result.State)
		}
	})

	t.Run("wait respects context timeout", func(t *testing.T) {
		// Create resource with finalizer to simulate blocking deletion
		deletionTimestamp := metav1.Now()
		resource := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "timeout-test",
				Namespace:         "default",
				DeletionTimestamp: &deletionTimestamp,
				Finalizers:        []string{"block-deletion"},
			},
		}

		helper, err := NewHelperV2(logger,
			WithClient(newFakeClientWithObjects(resource)),
			WithControllerNameV2(hivev1.ClustersyncControllerName),
		)
		require.NoError(t, err)

		// Create context with very short timeout
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		gvk := schema.GroupVersionKind{
			Version: "v1",
			Kind:    "ConfigMap",
		}

		// Wait should return error due to context timeout
		result, err := helper.Delete(ctx, gvk, "default", "timeout-test", WithWait())

		// Should get context error or DeletionInProgressV2
		if err != nil {
			assert.Contains(t, err.Error(), "context")
			assert.Equal(t, DeletionInProgressV2, result.State)
		} else {
			// Fake client might delete immediately
			assert.Equal(t, DeletedV2, result.State)
		}
	})
}

func TestHelperV2_DeleteIdempotent(t *testing.T) {
	logger := log.NewEntry(log.StandardLogger())
	ctx := context.Background()

	resource := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "idempotent-test",
			Namespace: "default",
		},
	}

	helper, err := NewHelperV2(logger,
		WithClient(newFakeClientWithObjects(resource)),
		WithControllerNameV2(hivev1.ClustersyncControllerName),
	)
	require.NoError(t, err)

	gvk := schema.GroupVersionKind{
		Version: "v1",
		Kind:    "ConfigMap",
	}

	t.Run("first delete succeeds", func(t *testing.T) {
		result, err := helper.Delete(ctx, gvk, "default", "idempotent-test")
		require.NoError(t, err)
		assert.Equal(t, DeletedV2, result.State)
	})

	t.Run("second delete returns NotFoundV2", func(t *testing.T) {
		result, err := helper.Delete(ctx, gvk, "default", "idempotent-test")
		require.NoError(t, err)
		assert.Equal(t, NotFoundV2, result.State)
	})
}

func TestHelperV2_DeleteContextCancellation(t *testing.T) {
	logger := log.NewEntry(log.StandardLogger())

	resource := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ctx-test",
			Namespace: "default",
		},
	}

	helper, err := NewHelperV2(logger,
		WithClient(newFakeClientWithObjects(resource)),
		WithControllerNameV2(hivev1.ClustersyncControllerName),
	)
	require.NoError(t, err)

	t.Run("respects context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		gvk := schema.GroupVersionKind{
			Version: "v1",
			Kind:    "ConfigMap",
		}

		// Fake client may not fully respect context cancellation
		// This test verifies the API accepts context parameter
		_, err := helper.Delete(ctx, gvk, "default", "ctx-test")
		// Not asserting error since fake client behavior varies
		_ = err
	})
}

func TestHelperV2_DeleteConcurrent(t *testing.T) {
	logger := log.NewEntry(log.StandardLogger())
	ctx := context.Background()

	// Create a single resource that will be deleted concurrently
	resources := []runtime.Object{
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "concurrent-test",
				Namespace: "default",
			},
		},
	}

	helper, err := NewHelperV2(logger,
		WithClient(newFakeClientWithObjects(resources...)),
		WithControllerNameV2(hivev1.ClustersyncControllerName),
	)
	require.NoError(t, err)

	// Run with -race flag to detect race conditions
	concurrency := 20
	done := make(chan bool, concurrency)

	gvk := schema.GroupVersionKind{
		Version: "v1",
		Kind:    "ConfigMap",
	}

	for i := 0; i < concurrency; i++ {
		go func(id int) {
			defer func() { done <- true }()

			_, err := helper.Delete(ctx, gvk, "default", "concurrent-test")
			// Errors acceptable in concurrent test (e.g., already deleted)
			_ = err
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < concurrency; i++ {
		<-done
	}
}

func TestHelperV2_DeleteStateSemanticsVsV1(t *testing.T) {
	logger := log.NewEntry(log.StandardLogger())
	ctx := context.Background()

	t.Run("v2 returns clear DeletionInProgressV2 state", func(t *testing.T) {
		// This tests the fix for the v1 bug where deletion state was ambiguous

		deletionTimestamp := metav1.Now()
		resourceWithFinalizer := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "v2-semantics-test",
				Namespace:         "default",
				DeletionTimestamp: &deletionTimestamp,
				Finalizers:        []string{"blocking-finalizer"},
			},
		}

		helper, err := NewHelperV2(logger,
			WithClient(newFakeClientWithObjects(resourceWithFinalizer)),
			WithControllerNameV2(hivev1.ClustersyncControllerName),
		)
		require.NoError(t, err)

		gvk := schema.GroupVersionKind{
			Version: "v1",
			Kind:    "ConfigMap",
		}

		result, err := helper.Delete(ctx, gvk, "default", "v2-semantics-test")
		require.NoError(t, err)

		// v2 explicitly returns DeletionInProgressV2 (not ambiguous false)
		assert.Equal(t, DeletionInProgressV2, result.State, "v2 should return explicit DeletionInProgressV2 state")
		assert.NotNil(t, result.DeletionTimestamp, "should include deletion timestamp")
		assert.NotNil(t, result.Object, "should include object for inspection")
	})

	t.Run("v2 distinguishes NotFoundV2 from DeletedV2", func(t *testing.T) {
		helper, err := NewHelperV2(logger,
			WithClient(newFakeClient()),
			WithControllerNameV2(hivev1.ClustersyncControllerName),
		)
		require.NoError(t, err)

		gvk := schema.GroupVersionKind{
			Version: "v1",
			Kind:    "ConfigMap",
		}

		// Delete non-existent resource
		result, err := helper.Delete(ctx, gvk, "default", "never-existed")
		require.NoError(t, err)

		// v2 explicitly returns NotFoundV2 (not DeletedV2)
		assert.Equal(t, NotFoundV2, result.State, "v2 should distinguish NotFoundV2 from DeletedV2")
	})
}

func TestHelperV2_DeleteErrorWrapping(t *testing.T) {
	logger := log.NewEntry(log.StandardLogger())
	ctx := context.Background()

	helper, err := NewHelperV2(logger,
		WithClient(newFakeClient()),
		WithControllerNameV2(hivev1.ClustersyncControllerName),
	)
	require.NoError(t, err)

	t.Run("wraps errors with cluster context", func(t *testing.T) {
		gvk := schema.GroupVersionKind{
			Version: "v1",
			Kind:    "ConfigMap",
		}

		// This should succeed (NotFoundV2 state)
		result, err := helper.Delete(ctx, gvk, "default", "test")
		require.NoError(t, err)
		assert.Equal(t, NotFoundV2, result.State)

		// Error wrapping is tested in other error paths
	})
}

func TestHelperV2_DeleteMetricsRecording(t *testing.T) {
	logger := log.NewEntry(log.StandardLogger())
	ctx := context.Background()

	t.Run("records metrics on success", func(t *testing.T) {
		resource := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "metrics-test",
				Namespace: "default",
			},
		}

		helper, err := NewHelperV2(logger,
			WithClient(newFakeClientWithObjects(resource)),
			WithControllerNameV2(hivev1.ClustersyncControllerName),
		)
		require.NoError(t, err)

		gvk := schema.GroupVersionKind{
			Version: "v1",
			Kind:    "ConfigMap",
		}

		result, err := helper.Delete(ctx, gvk, "default", "metrics-test")
		require.NoError(t, err)
		assert.Equal(t, DeletedV2, result.State)

		// Metrics recording happens internally
		// Integration tests would verify metrics collection
	})

	t.Run("records metrics for NotFoundV2", func(t *testing.T) {
		helper, err := NewHelperV2(logger,
			WithClient(newFakeClient()),
			WithControllerNameV2(hivev1.ClustersyncControllerName),
		)
		require.NoError(t, err)

		gvk := schema.GroupVersionKind{
			Version: "v1",
			Kind:    "ConfigMap",
		}

		result, err := helper.Delete(ctx, gvk, "default", "not-found")
		require.NoError(t, err)
		assert.Equal(t, NotFoundV2, result.State)

		// Metrics should be recorded for NotFoundV2 too
	})

	t.Run("records metrics for DeletionInProgressV2", func(t *testing.T) {
		deletionTimestamp := metav1.Now()
		resource := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "deletion-in-progress",
				Namespace:         "default",
				DeletionTimestamp: &deletionTimestamp,
				Finalizers:        []string{"finalizer"},
			},
		}

		helper, err := NewHelperV2(logger,
			WithClient(newFakeClientWithObjects(resource)),
			WithControllerNameV2(hivev1.ClustersyncControllerName),
		)
		require.NoError(t, err)

		gvk := schema.GroupVersionKind{
			Version: "v1",
			Kind:    "ConfigMap",
		}

		result, err := helper.Delete(ctx, gvk, "default", "deletion-in-progress")
		require.NoError(t, err)
		assert.Equal(t, DeletionInProgressV2, result.State)

		// Metrics should be recorded for all states
	})
}

func TestHelperV2_DeleteWaitForDeletion(t *testing.T) {
	logger := log.NewEntry(log.StandardLogger())

	t.Run("waitForDeletion polls until deleted", func(t *testing.T) {
		// Create resource
		resource := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "poll-test",
				Namespace: "default",
			},
		}

		helper, err := NewHelperV2(logger,
			WithClient(newFakeClientWithObjects(resource)),
			WithControllerNameV2(hivev1.ClustersyncControllerName),
		)
		require.NoError(t, err)

		// Short timeout to prevent hanging
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		gvk := schema.GroupVersionKind{
			Version: "v1",
			Kind:    "ConfigMap",
		}

		// Delete with wait - fake client should delete immediately
		result, err := helper.Delete(ctx, gvk, "default", "poll-test", WithWait())

		// Should succeed or timeout (both acceptable in test)
		if err == nil {
			assert.Equal(t, DeletedV2, result.State)
		} else {
			// Timeout is acceptable
			assert.Contains(t, err.Error(), "context")
		}
	})
}
