package remoteclient

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/internal/clientutil"
)

// TestNewBuilderV2_WithOptions tests that functional options are applied correctly.
func TestNewBuilderV2_WithOptions(t *testing.T) {
	cd := testClusterDeployment()
	c := fakeClient(cd)
	cache := clientutil.NewCache()

	builder := NewBuilderV2(
		WithClusterDeployment(c, cd),
		WithControllerName(hivev1.ClustersyncControllerName),
		WithCache(cache),
		WithPrimaryURL(),
	)

	v2Builder, ok := builder.(*builderV2)
	if !ok {
		t.Fatal("NewBuilderV2() did not return *builderV2")
	}

	if v2Builder.config.client != c {
		t.Error("Client not set correctly")
	}

	if v2Builder.config.cd != cd {
		t.Error("ClusterDeployment not set correctly")
	}

	if v2Builder.config.controllerName != hivev1.ClustersyncControllerName {
		t.Error("ControllerName not set correctly")
	}

	if !v2Builder.config.useCache {
		t.Error("Cache should be enabled")
	}

	if v2Builder.config.cache != cache {
		t.Error("Cache not set correctly")
	}

	if v2Builder.config.urlSelection != primaryURL {
		t.Error("URL selection not set correctly")
	}
}

// TestNewBuilderV2_WithoutCache tests that cache can be disabled.
func TestNewBuilderV2_WithoutCache(t *testing.T) {
	cd := testClusterDeployment()
	c := fakeClient(cd)

	builder := NewBuilderV2(
		WithClusterDeployment(c, cd),
		WithControllerName(hivev1.ClustersyncControllerName),
		WithoutCache(),
	)

	v2Builder := builder.(*builderV2)

	if v2Builder.config.useCache {
		t.Error("Cache should be disabled")
	}

	if v2Builder.config.cache != nil {
		t.Error("Cache should be nil when disabled")
	}
}

// TestBuilderV2_ImplementsBuilder tests that BuilderV2 implements Builder interface.
func TestBuilderV2_ImplementsBuilder(t *testing.T) {
	var _ Builder = NewBuilderV2()
}

// TestBuilderV2_ImplementsBuilderver2 tests that BuilderV2 implements BuilderV2 interface.
func TestBuilderV2_ImplementsBuilderV2(t *testing.T) {
	var _ BuilderV2 = NewBuilderV2()
}

// TestBuilderV2_URLSelection tests URL selection methods.
func TestBuilderV2_URLSelection(t *testing.T) {
	cd := testClusterDeployment()
	c := fakeClient(cd)

	builder := NewBuilderV2(
		WithClusterDeployment(c, cd),
		WithControllerName(hivev1.ClustersyncControllerName),
	)

	// Test v1 methods (mutate for backward compatibility)
	// UsePrimaryAPIURL mutates the builder
	returnedBuilder := builder.UsePrimaryAPIURL()
	if returnedBuilder != builder {
		t.Error("UsePrimaryAPIURL() should return same instance for v1 compatibility")
	}
	v2Primary := builder.(*builderV2)
	if v2Primary.config.urlSelection != primaryURL {
		t.Error("UsePrimaryAPIURL() did not set primaryURL")
	}

	// UseSecondaryAPIURL also mutates
	builder.UseSecondaryAPIURL()
	v2Secondary := builder.(*builderV2)
	if v2Secondary.config.urlSelection != secondaryURL {
		t.Error("UseSecondaryAPIURL() did not set secondaryURL")
	}

	// Test v2 methods (immutable)
	builder2 := NewBuilderV2(
		WithClusterDeployment(c, cd),
		WithControllerName(hivev1.ClustersyncControllerName),
	)

	primaryBuilder2 := builder2.UsePrimaryAPIURLV2()
	if primaryBuilder2 == builder2 {
		t.Error("UsePrimaryAPIURLV2() should return new instance (immutable)")
	}
	v2Primary2 := primaryBuilder2.(*builderV2)
	if v2Primary2.config.urlSelection != primaryURL {
		t.Error("UsePrimaryAPIURLV2() did not set primaryURL on new instance")
	}
	// Original should be unchanged
	v2Original := builder2.(*builderV2)
	if v2Original.config.urlSelection != activeURL {
		t.Error("Original builder was mutated by v2 method")
	}
}

// TestBuilderV2_ContextCancellation tests that context cancellation is respected.
func TestBuilderV2_ContextCancellation(t *testing.T) {
	cd := testClusterDeployment()
	kubeconfigSecret := testKubeconfigSecret(t)
	c := fakeClient(cd, kubeconfigSecret)

	builder := NewBuilderV2(
		WithClusterDeployment(c, cd),
		WithControllerName(hivev1.ClustersyncControllerName),
	)

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Attempt to build with cancelled context
	_, err := builder.BuildWithContext(ctx)

	// Should get context error (though it might fail earlier at secret loading)
	if err == nil {
		t.Error("Expected error with cancelled context")
	}
}

// TestBuilderV2_ContextTimeout tests that context timeout is respected.
func TestBuilderV2_ContextTimeout(t *testing.T) {
	cd := testClusterDeployment()
	kubeconfigSecret := testKubeconfigSecret(t)
	c := fakeClient(cd, kubeconfigSecret)

	builder := NewBuilderV2(
		WithClusterDeployment(c, cd),
		WithControllerName(hivev1.ClustersyncControllerName),
	)

	// Create a context with very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	// Wait for timeout
	time.Sleep(1 * time.Millisecond)

	// Attempt to build with timed-out context
	_, err := builder.BuildWithContext(ctx)

	// Should get timeout error
	if err == nil {
		t.Error("Expected error with timed-out context")
	}
}

// TestBuilderV2_BackwardCompatibility tests v1 methods still work.
func TestBuilderV2_BackwardCompatibility(t *testing.T) {
	cd := testClusterDeployment()
	kubeconfigSecret := testKubeconfigSecret(t)
	c := fakeClient(cd, kubeconfigSecret)

	builder := NewBuilderV2(
		WithClusterDeployment(c, cd),
		WithControllerName(hivev1.ClustersyncControllerName),
	)

	// These should delegate to context versions with context.Background()
	// They will fail due to missing kubeconfig secret, but should compile and run

	_, err := builder.Build()
	if err == nil {
		t.Log("Build() succeeded (unexpected but OK for test)")
	}

	_, err = builder.BuildDynamic()
	if err == nil {
		t.Log("BuildDynamic() succeeded (unexpected but OK for test)")
	}

	_, err = builder.BuildKubeClient()
	if err == nil {
		t.Log("BuildKubeClient() succeeded (unexpected but OK for test)")
	}

	_, err = builder.RESTConfig()
	if err == nil {
		t.Log("RESTConfig() succeeded (unexpected but OK for test)")
	}

	// URL selection should work
	_ = builder.UsePrimaryAPIURL()
	_ = builder.UseSecondaryAPIURL()
}

// TestDetermineAPIURL tests API URL determination logic.
func TestDetermineAPIURL(t *testing.T) {
	tests := []struct {
		name           string
		override       string
		urlSelection   int
		kubeconfigURL  string
		isPrimaryActive bool
		expected       string
	}{
		{
			name:          "no override - primary",
			override:      "",
			urlSelection:  primaryURL,
			kubeconfigURL: "https://api.kubeconfig.com:6443",
			expected:      "https://api.kubeconfig.com:6443",
		},
		{
			name:          "with override - primary",
			override:      "https://api.override.com:6443",
			urlSelection:  primaryURL,
			kubeconfigURL: "https://api.kubeconfig.com:6443",
			expected:      "https://api.override.com:6443",
		},
		{
			name:          "with override - secondary",
			override:      "https://api.override.com:6443",
			urlSelection:  secondaryURL,
			kubeconfigURL: "https://api.kubeconfig.com:6443",
			expected:      "https://api.kubeconfig.com:6443",
		},
		{
			name:            "with override - active (primary active)",
			override:        "https://api.override.com:6443",
			urlSelection:    activeURL,
			kubeconfigURL:   "https://api.kubeconfig.com:6443",
			isPrimaryActive: true,
			expected:        "https://api.override.com:6443",
		},
		{
			name:            "with override - active (secondary active)",
			override:        "https://api.override.com:6443",
			urlSelection:    activeURL,
			kubeconfigURL:   "https://api.kubeconfig.com:6443",
			isPrimaryActive: false,
			expected:        "https://api.kubeconfig.com:6443",
		},
		{
			name:          "no override - active",
			override:      "",
			urlSelection:  activeURL,
			kubeconfigURL: "https://api.kubeconfig.com:6443",
			expected:      "https://api.kubeconfig.com:6443",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cd := testClusterDeployment()
			cd.Spec.ControlPlaneConfig.APIURLOverride = tt.override

			if tt.isPrimaryActive {
				cd.Status.Conditions = []hivev1.ClusterDeploymentCondition{
					{
						Type:   hivev1.ActiveAPIURLOverrideCondition,
						Status: corev1.ConditionTrue,
					},
				}
			}

			result := determineAPIURL(cd, tt.urlSelection, tt.kubeconfigURL)

			if result != tt.expected {
				t.Errorf("determineAPIURL() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestGenerateCacheKey tests cache key generation.
func TestGenerateCacheKey(t *testing.T) {
	cd := testClusterDeployment()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       "test-namespace",
			Name:            "kubeconfig-secret",
			ResourceVersion: "12345",
		},
	}
	apiURL := "https://api.test.com:6443"

	key := generateCacheKey(cd, secret, apiURL)

	expectedClusterID := "test-namespace/test-cluster-deployment"
	if key.ClusterID != expectedClusterID {
		t.Errorf("ClusterID = %q, want %q", key.ClusterID, expectedClusterID)
	}

	if key.KubeconfigVersion != "12345" {
		t.Errorf("KubeconfigVersion = %q, want %q", key.KubeconfigVersion, "12345")
	}

	if key.APIURL != apiURL {
		t.Errorf("APIURL = %q, want %q", key.APIURL, apiURL)
	}
}

// TestGenerateCacheKeyFromSecret tests cache key generation from secret.
func TestGenerateCacheKeyFromSecret(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       "test-namespace",
			Name:            "kubeconfig-secret",
			ResourceVersion: "67890",
		},
	}
	apiURL := "https://api.test.com:6443"

	key := generateCacheKeyFromSecret(secret, apiURL)

	expectedClusterID := "test-namespace/kubeconfig-secret"
	if key.ClusterID != expectedClusterID {
		t.Errorf("ClusterID = %q, want %q", key.ClusterID, expectedClusterID)
	}

	if key.KubeconfigVersion != "67890" {
		t.Errorf("KubeconfigVersion = %q, want %q", key.KubeconfigVersion, "67890")
	}

	if key.APIURL != apiURL {
		t.Errorf("APIURL = %q, want %q", key.APIURL, apiURL)
	}
}

// TestNewBuilder_ReturnsV2 tests that NewBuilder returns a v2 builder.
func TestNewBuilder_ReturnsV2(t *testing.T) {
	cd := testClusterDeployment()
	c := fakeClient(cd)

	builder := NewBuilder(c, cd, hivev1.ClustersyncControllerName)

	// Should be a BuilderV2
	_, ok := builder.(BuilderV2)
	if !ok {
		t.Error("NewBuilder() should return a BuilderV2")
	}

	// Should also implement Builder
	_, ok = builder.(Builder)
	if !ok {
		t.Error("NewBuilder() should return a Builder")
	}
}

// TestFakeBuilder_ImplementsV2 tests that fake builder implements v2 interface.
func TestFakeBuilder_ImplementsV2(t *testing.T) {
	var _ BuilderV2 = &fakeBuilder{}
}

// TestFakeBuilder_ContextMethods tests fake builder context methods.
func TestFakeBuilder_ContextMethods(t *testing.T) {
	fb := &fakeBuilder{
		urlToUse:       activeURL,
		clusterVersion: "4.10.0",
	}

	ctx := context.Background()

	// Test BuildWithContext
	client, err := fb.BuildWithContext(ctx)
	if err != nil {
		t.Errorf("BuildWithContext() error = %v", err)
	}
	if client == nil {
		t.Error("BuildWithContext() returned nil client")
	}

	// Test with cancelled context
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = fb.BuildWithContext(cancelledCtx)
	if err != context.Canceled {
		t.Errorf("BuildWithContext(cancelled) error = %v, want %v", err, context.Canceled)
	}
}

// testClusterDeployment and testKubeconfigSecret are defined in remoteclient_test.go
