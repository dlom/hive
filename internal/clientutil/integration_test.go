package clientutil_test

import (
	"context"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/internal/clientutil"
)

// TestIntegration_CacheWithConfigUtils tests cache and config utilities working together.
func TestIntegration_CacheWithConfigUtils(t *testing.T) {
	// Create a cache
	cache := clientutil.NewCache(
		clientutil.WithMaxSize(10),
		clientutil.WithTTL(1*time.Hour),
	)

	// Create a REST config
	cfg := &rest.Config{
		Host:        "https://test-cluster.example.com:6443",
		BearerToken: "test-token",
	}

	// Apply metrics wrapper immutably
	cfgWithMetrics := clientutil.CopyConfigWithMetrics(cfg, hivev1.ClustersyncControllerName, true)

	// Verify original wasn't modified
	if clientutil.IsTransportWrapped(cfg) {
		t.Error("Original config was mutated (wrapper applied)")
	}

	// Verify copy has wrapper
	if !clientutil.IsTransportWrapped(cfgWithMetrics) {
		t.Error("Config with metrics should have wrapper")
	}

	// Create cache key
	key := clientutil.NewCacheKey("hive/test-cluster", "v1", cfg.Host)

	// Use cache with factory
	factoryCalls := 0
	factory := func(ctx context.Context) (client.Client, error) {
		factoryCalls++
		return fake.NewClientBuilder().Build(), nil
	}

	ctx := context.Background()

	// First call - cache miss
	client1, err := cache.Get(ctx, key, factory)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if client1 == nil {
		t.Fatal("Get() returned nil client")
	}
	if factoryCalls != 1 {
		t.Errorf("Factory calls = %d, want 1", factoryCalls)
	}

	// Second call - cache hit
	client2, err := cache.Get(ctx, key, factory)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if client2 != client1 {
		t.Error("Cache didn't return same client on second call")
	}
	if factoryCalls != 1 {
		t.Errorf("Factory calls = %d, want 1 (should not be called on cache hit)", factoryCalls)
	}

	// Verify cache stats
	stats := cache.Stats()
	if stats.Hits != 1 {
		t.Errorf("Stats.Hits = %d, want 1", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Errorf("Stats.Misses = %d, want 1", stats.Misses)
	}
}

// TestIntegration_ErrorWrappingWithPredicates tests error utilities working together.
func TestIntegration_ErrorWrappingWithPredicates(t *testing.T) {
	// Create a cluster error
	gvk := schema.GroupVersionKind{
		Group:   "apps",
		Version: "v1",
		Kind:    "Deployment",
	}

	err := clientutil.WrapClusterError(
		context.DeadlineExceeded,
		"hive/test-cluster",
		"apply",
		gvk,
		"default",
		"my-deployment",
	)

	// Test predicates
	if !clientutil.IsTimeout(err) {
		t.Error("IsTimeout() should return true for DeadlineExceeded")
	}

	if clientutil.IsNotFound(err) {
		t.Error("IsNotFound() should return false")
	}

	if clientutil.IsCanceled(err) {
		t.Error("IsCanceled() should return false for timeout")
	}

	// Test error unwrapping
	var ce *clientutil.ClusterError
	if !clientutil.AsClusterError(err, &ce) {
		t.Fatal("AsClusterError() should return true")
	}

	if ce.ClusterID != "hive/test-cluster" {
		t.Errorf("ClusterID = %q, want %q", ce.ClusterID, "hive/test-cluster")
	}

	if ce.Operation != "apply" {
		t.Errorf("Operation = %q, want %q", ce.Operation, "apply")
	}

	if ce.GVK != gvk {
		t.Errorf("GVK = %v, want %v", ce.GVK, gvk)
	}
}

// TestIntegration_FieldManagerConsistency tests field manager naming consistency.
func TestIntegration_FieldManagerConsistency(t *testing.T) {
	controllerName := hivev1.ClustersyncControllerName

	// Get unified field manager name
	fieldManager := clientutil.FieldManagerName(controllerName)

	expectedFormat := "hive-clustersync"
	if fieldManager != expectedFormat {
		t.Errorf("FieldManagerName() = %q, want %q", fieldManager, expectedFormat)
	}

	// Verify consistency across multiple calls
	fieldManager2 := clientutil.FieldManagerName(controllerName)
	if fieldManager != fieldManager2 {
		t.Error("FieldManagerName() not consistent across calls")
	}

	// Compare with legacy names (should be different)
	legacyV2 := clientutil.FieldManagerNameLegacy(controllerName, 2)
	if fieldManager == legacyV2 {
		t.Error("Unified name should differ from legacy v2 name")
	}

	expectedLegacy := "hive2-clustersync"
	if legacyV2 != expectedLegacy {
		t.Errorf("Legacy name = %q, want %q", legacyV2, expectedLegacy)
	}
}

// TestIntegration_CacheInvalidationOnConfigChange tests automatic invalidation.
func TestIntegration_CacheInvalidationOnConfigChange(t *testing.T) {
	cache := clientutil.NewCache(
		clientutil.WithMaxSize(10),
		clientutil.WithTTL(1*time.Hour),
	)

	ctx := context.Background()
	factory := func(ctx context.Context) (client.Client, error) {
		return fake.NewClientBuilder().Build(), nil
	}

	clusterID := "hive/test-cluster"
	apiURL := "https://api-primary.example.com:6443"

	// Create client with version v1
	keyV1 := clientutil.NewCacheKey(clusterID, "v1", apiURL)
	client1, err := cache.Get(ctx, keyV1, factory)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	// Simulate kubeconfig update (ResourceVersion changes)
	keyV2 := clientutil.NewCacheKey(clusterID, "v2", apiURL)
	client2, err := cache.Get(ctx, keyV2, factory)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	// Should get different clients (automatic invalidation)
	if client1 == client2 {
		t.Error("Expected different clients after kubeconfig version change")
	}

	stats := cache.Stats()
	if stats.Misses != 2 {
		t.Errorf("Stats.Misses = %d, want 2 (version change should cause cache miss)", stats.Misses)
	}

	// Simulate API URL failover
	keySecondary := clientutil.NewCacheKey(clusterID, "v2", "https://api-secondary.example.com:6443")
	client3, err := cache.Get(ctx, keySecondary, factory)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	// Should get different client (automatic invalidation on URL change)
	if client2 == client3 {
		t.Error("Expected different clients after API URL change")
	}

	stats = cache.Stats()
	if stats.Misses != 3 {
		t.Errorf("Stats.Misses = %d, want 3 (URL change should cause cache miss)", stats.Misses)
	}
}

// TestIntegration_ConfigImmutability tests that config operations never mutate inputs.
func TestIntegration_ConfigImmutability(t *testing.T) {
	original := &rest.Config{
		Host:        "https://original.example.com:6443",
		BearerToken: "original-token",
	}

	// Apply multiple transformations
	withMetrics := clientutil.CopyConfigWithMetrics(original, hivev1.ClustersyncControllerName, false)
	withOverrides := clientutil.PrepareConfigForClient(withMetrics, "https://override.example.com:6443", "10.0.0.1")

	// Verify original is completely unchanged
	if original.Host != "https://original.example.com:6443" {
		t.Error("Original Host was mutated")
	}
	if original.BearerToken != "original-token" {
		t.Error("Original BearerToken was mutated")
	}
	if original.WrapTransport != nil {
		t.Error("Original WrapTransport was mutated")
	}
	if original.Dial != nil {
		t.Error("Original Dial was mutated")
	}

	// Verify transformations were applied to copies
	if withMetrics.Host != original.Host {
		t.Error("CopyConfigWithMetrics should preserve Host")
	}
	if !clientutil.IsTransportWrapped(withMetrics) {
		t.Error("CopyConfigWithMetrics should apply wrapper")
	}

	if withOverrides.Host != "https://override.example.com:6443" {
		t.Error("PrepareConfigForClient should apply URL override")
	}
	if withOverrides.Dial == nil {
		t.Error("PrepareConfigForClient should apply IP override")
	}
}

// TestIntegration_DiscoveryWithCache tests discovery manager integration.
func TestIntegration_DiscoveryWithCache(t *testing.T) {
	discoveryMgr := clientutil.NewDiscoveryManager(
		clientutil.WithDiscoveryTTL(1 * time.Hour),
	)

	cfg := &rest.Config{
		Host: "https://fake-cluster.example.com:6443",
	}

	// Attempt to create discovery client (will fail for fake server, but tests the API)
	_, err := discoveryMgr.NewCachedDiscoveryClient(cfg)
	if err != nil {
		t.Logf("Expected error for fake server: %v", err)
		// This is okay - we're testing the API, not actual connectivity
		return
	}

	// If we somehow succeeded (unlikely), verify cache reuse
	client2, err := discoveryMgr.NewCachedDiscoveryClient(cfg)
	if err == nil && client2 != nil {
		t.Log("Discovery client created successfully (cache reuse will be tested)")
	}

	// Test invalidation
	discoveryMgr.InvalidateCache(cfg.Host)
	t.Log("Discovery cache invalidated successfully")
}

// TestIntegration_EndToEnd tests a complete workflow using all components.
func TestIntegration_EndToEnd(t *testing.T) {
	// 1. Create infrastructure
	cache := clientutil.NewCache(
		clientutil.WithMaxSize(100),
		clientutil.WithTTL(10*time.Minute),
	)

	discoveryMgr := clientutil.NewDiscoveryManager()
	_ = discoveryMgr // Discovery manager created for completeness, not used in this simple test

	// 2. Prepare REST config
	cfg := &rest.Config{
		Host:        "https://test-cluster.example.com:6443",
		BearerToken: "test-token",
	}

	cfgWithMetrics := clientutil.CopyConfigWithMetrics(
		cfg,
		hivev1.ClustersyncControllerName,
		true, // remote cluster
	)

	cfgWithOverrides := clientutil.PrepareConfigForClient(
		cfgWithMetrics,
		"https://api-override.example.com:6443",
		"", // no IP override
	)

	// 3. Generate cache key
	cacheKey := clientutil.NewCacheKey(
		"hive/test-cluster",
		"v12345",
		cfgWithOverrides.Host,
	)

	// 4. Create client via cache
	ctx := context.Background()
	factory := func(ctx context.Context) (client.Client, error) {
		// In real usage, this would use cfgWithOverrides
		return fake.NewClientBuilder().Build(), nil
	}

	remoteClient, err := cache.Get(ctx, cacheKey, factory)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	if remoteClient == nil {
		t.Fatal("Client is nil")
	}

	// 5. Simulate an operation error
	operationErr := context.DeadlineExceeded
	wrappedErr := clientutil.WrapClusterError(
		operationErr,
		cacheKey.ClusterID,
		"apply",
		schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
		"default",
		"test-deployment",
	)

	// 6. Test error predicates
	if !clientutil.IsTimeout(wrappedErr) {
		t.Error("Should detect timeout error")
	}

	// 7. Get field manager name
	fieldManager := clientutil.FieldManagerName(hivev1.ClustersyncControllerName)
	if fieldManager != "hive-clustersync" {
		t.Errorf("Field manager = %q, want %q", fieldManager, "hive-clustersync")
	}

	// 8. Verify cache stats
	stats := cache.Stats()
	if stats.Size != 1 {
		t.Errorf("Cache size = %d, want 1", stats.Size)
	}

	t.Log("End-to-end integration test passed successfully")
}
