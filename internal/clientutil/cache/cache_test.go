package cache

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// TestCache_Get_Hit tests that cache hits return existing clients.
func TestCache_Get_Hit(t *testing.T) {
	cache := NewCache(WithMaxSize(10), WithTTL(1*time.Hour))
	key := NewCacheKey("test-ns/test-cluster", "v1", "https://api.test.com:6443")

	ctx := context.Background()

	// Track factory calls
	factoryCalls := 0
	factory := func(ctx context.Context) (client.Client, error) {
		factoryCalls++
		return fake.NewClientBuilder().Build(), nil
	}

	// First call should create client
	client1, err := cache.Get(ctx, key, factory)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if client1 == nil {
		t.Fatal("Get() returned nil client")
	}
	if factoryCalls != 1 {
		t.Errorf("factory called %d times, want 1", factoryCalls)
	}

	// Second call should hit cache
	client2, err := cache.Get(ctx, key, factory)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if client2 != client1 {
		t.Error("Get() returned different client on cache hit")
	}
	if factoryCalls != 1 {
		t.Errorf("factory called %d times, want 1 (should not be called on cache hit)", factoryCalls)
	}
}

// TestCache_Get_Miss tests that cache misses create new clients.
func TestCache_Get_Miss(t *testing.T) {
	cache := NewCache(WithMaxSize(10), WithTTL(1*time.Hour))
	ctx := context.Background()

	key1 := NewCacheKey("test-ns/cluster1", "v1", "https://api1.test.com:6443")
	key2 := NewCacheKey("test-ns/cluster2", "v1", "https://api2.test.com:6443")

	factoryCalls := 0
	factory := func(ctx context.Context) (client.Client, error) {
		factoryCalls++
		return fake.NewClientBuilder().Build(), nil
	}

	client1, err := cache.Get(ctx, key1, factory)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	client2, err := cache.Get(ctx, key2, factory)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if client1 == client2 {
		t.Error("Get() returned same client for different keys")
	}

	if factoryCalls != 2 {
		t.Errorf("factory called %d times, want 2", factoryCalls)
	}
}

// TestCache_LRU_Eviction tests that LRU eviction works correctly.
func TestCache_LRU_Eviction(t *testing.T) {
	cache := NewCache(WithMaxSize(3), WithTTL(1*time.Hour))
	ctx := context.Background()

	factoryCalls := 0
	factory := func(ctx context.Context) (client.Client, error) {
		factoryCalls++
		return fake.NewClientBuilder().Build(), nil
	}

	// Fill cache to capacity
	key1 := NewCacheKey("test-ns/cluster1", "v1", "https://api1.test.com:6443")
	key2 := NewCacheKey("test-ns/cluster2", "v1", "https://api2.test.com:6443")
	key3 := NewCacheKey("test-ns/cluster3", "v1", "https://api3.test.com:6443")

	cache.Get(ctx, key1, factory) // Oldest
	cache.Get(ctx, key2, factory)
	cache.Get(ctx, key3, factory) // Newest

	// Access key1 to make it more recently used than key2
	cache.Get(ctx, key1, factory) // Now: key2 (oldest), key3, key1 (newest)

	// Add new entry, should evict key2 (oldest)
	key4 := NewCacheKey("test-ns/cluster4", "v1", "https://api4.test.com:6443")
	cache.Get(ctx, key4, factory)

	// Accessing key2 should be a miss (it was evicted)
	factoryCallsBefore := factoryCalls
	cache.Get(ctx, key2, factory)
	if factoryCalls != factoryCallsBefore+1 {
		t.Error("Expected cache miss for evicted entry (factory should be called)")
	}
}

// TestCache_TTL_Expiration tests that entries are evicted after TTL.
func TestCache_TTL_Expiration(t *testing.T) {
	shortTTL := 100 * time.Millisecond
	cache := NewCache(WithMaxSize(10), WithTTL(shortTTL))
	ctx := context.Background()

	key := NewCacheKey("test-ns/test-cluster", "v1", "https://api.test.com:6443")

	factoryCalls := 0
	factory := func(ctx context.Context) (client.Client, error) {
		factoryCalls++
		return fake.NewClientBuilder().Build(), nil
	}

	// Create entry
	_, err := cache.Get(ctx, key, factory)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	// Wait for TTL to expire
	time.Sleep(shortTTL + 50*time.Millisecond)

	// Next access should trigger eviction and recreation
	_, err = cache.Get(ctx, key, factory)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if factoryCalls != 2 {
		t.Errorf("factory called %d times, want 2 (once for initial, once after TTL)", factoryCalls)
	}
}

// TestCache_ConcurrentAccess tests thread-safety with concurrent goroutines.
func TestCache_ConcurrentAccess(t *testing.T) {
	cache := NewCache(WithMaxSize(100), WithTTL(1*time.Hour))
	ctx := context.Background()

	numGoroutines := 50
	numOperationsPerGoroutine := 100

	var factoryCalls int64
	factory := func(ctx context.Context) (client.Client, error) {
		atomic.AddInt64(&factoryCalls, 1)
		// Simulate some work
		time.Sleep(1 * time.Millisecond)
		return fake.NewClientBuilder().Build(), nil
	}

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			for j := 0; j < numOperationsPerGoroutine; j++ {
				// Use a mix of keys to test both hits and misses
				keyID := (id + j) % 20 // Reuse keys to get cache hits
				key := NewCacheKey("test-ns/test-cluster", fmt.Sprintf("v%d", keyID), "https://api.test.com:6443")

				_, err := cache.Get(ctx, key, factory)
				if err != nil {
					t.Errorf("Get() error = %v", err)
					return
				}
			}
		}(i)
	}

	wg.Wait()

	t.Logf("Factory calls: %d", factoryCalls)

	// Verify factory was called for each unique key, not for every operation
	// With 20 unique keys and 50 concurrent goroutines, factory should be called ~20 times
	// (plus some overhead for concurrent creation races where multiple goroutines
	// try to create the same client before it's cached)
	if factoryCalls > 200 { // Allow reasonable overhead for concurrent creation races
		t.Errorf("Factory called %d times, expected much fewer for 20 unique keys", factoryCalls)
	}
}

// TestCache_FactoryError tests that factory errors are propagated.
func TestCache_FactoryError(t *testing.T) {
	cache := NewCache(WithMaxSize(10), WithTTL(1*time.Hour))
	ctx := context.Background()

	key := NewCacheKey("test-ns/test-cluster", "v1", "https://api.test.com:6443")

	expectedErr := fmt.Errorf("factory error")
	factoryCalls := 0
	factory := func(ctx context.Context) (client.Client, error) {
		factoryCalls++
		return nil, expectedErr
	}

	_, err := cache.Get(ctx, key, factory)
	if err != expectedErr {
		t.Errorf("Get() error = %v, want %v", err, expectedErr)
	}

	// Error should not be cached - verify by calling again and seeing factory called twice
	cache.Get(ctx, key, factory)
	if factoryCalls != 2 {
		t.Errorf("Factory called %d times, want 2 (errors should not be cached)", factoryCalls)
	}
}

// TestCache_ContextCancellation tests that context cancellation is respected.
func TestCache_ContextCancellation(t *testing.T) {
	cache := NewCache(WithMaxSize(10), WithTTL(1*time.Hour))

	key := NewCacheKey("test-ns/test-cluster", "v1", "https://api.test.com:6443")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	factory := func(ctx context.Context) (client.Client, error) {
		// Check if context is canceled
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			return fake.NewClientBuilder().Build(), nil
		}
	}

	_, err := cache.Get(ctx, key, factory)
	if err != context.Canceled {
		t.Errorf("Get() error = %v, want %v", err, context.Canceled)
	}
}

// TestCache_ResourceVersionInvalidation tests automatic invalidation on kubeconfig update.
func TestCache_ResourceVersionInvalidation(t *testing.T) {
	cache := NewCache(WithMaxSize(10), WithTTL(1*time.Hour))
	ctx := context.Background()

	// Initial key with version v1
	keyV1 := NewCacheKey("test-ns/test-cluster", "v1", "https://api.test.com:6443")

	factoryCalls := 0
	factory := func(ctx context.Context) (client.Client, error) {
		factoryCalls++
		return fake.NewClientBuilder().Build(), nil
	}

	// Create entry with v1
	cache.Get(ctx, keyV1, factory)

	// Simulate kubeconfig update (ResourceVersion changes)
	keyV2 := NewCacheKey("test-ns/test-cluster", "v2", "https://api.test.com:6443")

	// Access with new version should be a miss (automatic invalidation)
	cache.Get(ctx, keyV2, factory)

	if factoryCalls != 2 {
		t.Errorf("factory called %d times, want 2 (version change should cause cache miss)", factoryCalls)
	}
}

// TestCache_APIURLInvalidation tests automatic invalidation on API URL change.
func TestCache_APIURLInvalidation(t *testing.T) {
	cache := NewCache(WithMaxSize(10), WithTTL(1*time.Hour))
	ctx := context.Background()

	// Initial key with primary URL
	keyPrimary := NewCacheKey("test-ns/test-cluster", "v1", "https://api-primary.test.com:6443")

	factoryCalls := 0
	factory := func(ctx context.Context) (client.Client, error) {
		factoryCalls++
		return fake.NewClientBuilder().Build(), nil
	}

	// Create entry with primary URL
	cache.Get(ctx, keyPrimary, factory)

	// Simulate API URL failover
	keySecondary := NewCacheKey("test-ns/test-cluster", "v1", "https://api-secondary.test.com:6443")

	// Access with new URL should be a miss (automatic invalidation)
	cache.Get(ctx, keySecondary, factory)

	if factoryCalls != 2 {
		t.Errorf("factory called %d times, want 2 (URL change should cause cache miss)", factoryCalls)
	}
}
