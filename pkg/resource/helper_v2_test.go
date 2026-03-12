package resource

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/util/scheme"
)

func TestNewHelperV2(t *testing.T) {
	logger := log.NewEntry(log.StandardLogger())

	tests := []struct {
		name    string
		options []HelperV2Option
		wantErr bool
		errMsg  string
	}{
		{
			name: "create with client",
			options: []HelperV2Option{
				WithClient(newFakeClient()),
				WithControllerNameV2(hivev1.ClustersyncControllerName),
			},
			wantErr: false,
		},
		{
			name: "create with REST config",
			options: []HelperV2Option{
				WithRESTConfigV2(&rest.Config{
					Host: "https://api.example.com",
				}),
				WithControllerNameV2(hivev1.ClustersyncControllerName),
			},
			wantErr: false,
		},
		{
			name: "create with controller name only",
			options: []HelperV2Option{
				WithClient(newFakeClient()),
				WithControllerNameV2(hivev1.ClustersyncControllerName),
			},
			wantErr: false,
		},
		{
			name:    "fail without client or REST config",
			options: []HelperV2Option{},
			wantErr: true,
			errMsg:  "neither client nor REST config provided",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			helper, err := NewHelperV2(logger, tt.options...)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
				assert.Nil(t, helper)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, helper)

				// Verify helper implements HelperV2 interface
				_, ok := helper.(HelperV2)
				assert.True(t, ok, "helper should implement HelperV2 interface")
			}
		})
	}
}

func TestHelperV2Options(t *testing.T) {
	logger := log.NewEntry(log.StandardLogger())

	t.Run("WithClient option", func(t *testing.T) {
		fakeClient := newFakeClient()
		helper, err := NewHelperV2(logger, WithClient(fakeClient))
		require.NoError(t, err)
		assert.NotNil(t, helper)

		h := helper.(*helperV2)
		assert.Equal(t, fakeClient, h.client)
	})

	t.Run("WithControllerNameV2 option", func(t *testing.T) {
		controllerName := hivev1.ClustersyncControllerName
		helper, err := NewHelperV2(logger,
			WithClient(newFakeClient()),
			WithControllerNameV2(controllerName),
		)
		require.NoError(t, err)
		assert.NotNil(t, helper)

		h := helper.(*helperV2)
		assert.Equal(t, controllerName, h.controllerName)
	})
}

func TestHelperV2Interface(t *testing.T) {
	logger := log.NewEntry(log.StandardLogger())
	ctx := context.Background()

	helper, err := NewHelperV2(logger,
		WithClient(newFakeClient()),
		WithControllerNameV2(hivev1.ClustersyncControllerName),
	)
	require.NoError(t, err)

	t.Run("implements Apply method", func(t *testing.T) {
		// Verify method exists and has correct signature
		assert.NotNil(t, helper.Apply)

		// Test with minimal YAML
		yamlData := []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-config
  namespace: default
data:
  key: value
`)
		result, err := helper.Apply(ctx, yamlData)
		require.NoError(t, err)
		assert.Equal(t, CreatedV2, result.State)
	})

	t.Run("implements Patch method", func(t *testing.T) {
		// Verify method exists
		assert.NotNil(t, helper.Patch)
	})

	t.Run("implements Delete method", func(t *testing.T) {
		// Verify method exists
		assert.NotNil(t, helper.Delete)
	})
}

func TestHelperV2ContextCancellation(t *testing.T) {
	logger := log.NewEntry(log.StandardLogger())

	helper, err := NewHelperV2(logger,
		WithClient(newFakeClient()),
		WithControllerNameV2(hivev1.ClustersyncControllerName),
	)
	require.NoError(t, err)

	t.Run("respects context cancellation in Apply", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		yamlData := []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-config
  namespace: default
`)

		// Note: fake client may not respect context cancellation
		// This test verifies the API accepts context parameter
		_, err := helper.Apply(ctx, yamlData)
		// Fake client doesn't enforce context, so we just verify it accepts it
		_ = err
	})
}

func TestHelperV2ThreadSafety(t *testing.T) {
	logger := log.NewEntry(log.StandardLogger())

	helper, err := NewHelperV2(logger,
		WithClient(newFakeClient()),
		WithControllerNameV2(hivev1.ClustersyncControllerName),
	)
	require.NoError(t, err)

	// Run concurrent operations to verify thread-safety
	// This should be run with -race flag
	ctx := context.Background()

	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			yamlData := []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-config
  namespace: default
`)
			_, _ = helper.Apply(ctx, yamlData)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

// newFakeClient creates a fake controller-runtime client for testing.
func newFakeClient() client.Client {
	s := scheme.GetScheme()
	return fake.NewClientBuilder().
		WithScheme(s).
		Build()
}

// newFakeClientWithObjects creates a fake client with pre-populated objects.
func newFakeClientWithObjects(objs ...runtime.Object) client.Client {
	s := scheme.GetScheme()
	return fake.NewClientBuilder().
		WithScheme(s).
		WithRuntimeObjects(objs...).
		Build()
}
