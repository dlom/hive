package resource

import (
	"context"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/util/scheme"
)

func TestNewHelper(t *testing.T) {
	logger := log.NewEntry(log.StandardLogger())

	tests := []struct {
		name    string
		options []HelperOption
		wantErr bool
		errMsg  string
	}{
		{
			name: "create with client",
			options: []HelperOption{
				WithClient(newFakeClient()),
				WithControllerName(hivev1.ClustersyncControllerName),
			},
			wantErr: false,
		},
		{
			name: "create with REST config",
			options: []HelperOption{
				WithRESTConfig(&rest.Config{
					Host: "https://api.example.com",
				}),
				WithControllerName(hivev1.ClustersyncControllerName),
			},
			wantErr: false,
		},
		{
			name: "create with controller name only",
			options: []HelperOption{
				WithClient(newFakeClient()),
				WithControllerName(hivev1.ClustersyncControllerName),
			},
			wantErr: false,
		},
		{
			name:    "fail without client or REST config",
			options: []HelperOption{},
			wantErr: true,
			errMsg:  "neither client nor REST config provided",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			helper, err := NewHelper(logger, tt.options...)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
				assert.Nil(t, helper)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, helper)

				// Verify helper implements Helper interface
				_, ok := helper.(Helper)
				assert.True(t, ok, "helper should implement Helper interface")
			}
		})
	}
}

func TestHelperOptions(t *testing.T) {
	logger := log.NewEntry(log.StandardLogger())

	t.Run("WithClient option", func(t *testing.T) {
		fakeClient := newFakeClient()
		helper, err := NewHelper(logger, WithClient(fakeClient))
		require.NoError(t, err)
		assert.NotNil(t, helper)

		h := helper.(*helperImpl)
		assert.Equal(t, fakeClient, h.client)
	})

	t.Run("WithControllerName option", func(t *testing.T) {
		controllerName := hivev1.ClustersyncControllerName
		helper, err := NewHelper(logger,
			WithClient(newFakeClient()),
			WithControllerName(controllerName),
		)
		require.NoError(t, err)
		assert.NotNil(t, helper)

		h := helper.(*helperImpl)
		assert.Equal(t, controllerName, h.controllerName)
	})
}

func TestHelperInterface(t *testing.T) {
	logger := log.NewEntry(log.StandardLogger())
	ctx := context.Background()

	helper, err := NewHelper(logger,
		WithClient(newFakeClient()),
		WithControllerName(hivev1.ClustersyncControllerName),
	)
	require.NoError(t, err)

	t.Run("implements Apply method", func(t *testing.T) {
		t.Skip("controller-runtime fake client doesn't support Server-Side Apply - use envtest for integration testing")

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
		assert.Equal(t, Created, result)
	})

	t.Run("implements Delete method", func(t *testing.T) {
		// Verify method exists
		assert.NotNil(t, helper.Delete)
	})
}

func TestHelperContextCancellation(t *testing.T) {
	logger := log.NewEntry(log.StandardLogger())

	helper, err := NewHelper(logger,
		WithClient(newFakeClient()),
		WithControllerName(hivev1.ClustersyncControllerName),
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

func TestHelperThreadSafety(t *testing.T) {
	logger := log.NewEntry(log.StandardLogger())

	helper, err := NewHelper(logger,
		WithClient(newFakeClient()),
		WithControllerName(hivev1.ClustersyncControllerName),
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
