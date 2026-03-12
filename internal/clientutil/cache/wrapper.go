package cache

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NewControllerCache creates a controller-specific view of a cache.
// The returned cache automatically injects the controller name into context
// for metrics tracking, allowing each controller to get metrics attributed
// to it while sharing the same underlying cache.
func NewControllerCache(cache ClientCache, controllerName string) ClientCache {
	return &controllerCache{
		cache:          cache,
		controllerName: controllerName,
	}
}

// controllerCache wraps the shared cache and injects controller name into context
// for metrics tracking. This allows each controller to get its own "view" of the
// shared cache without changing the ClientCache interface.
type controllerCache struct {
	cache          ClientCache
	controllerName string
}

// Get retrieves a client from the cache, injecting controller name into context
// for metrics tracking.
func (c *controllerCache) Get(ctx context.Context, key CacheKey, factory ClientFactory) (client.Client, error) {
	// Smuggle controller name via context
	ctx = withControllerName(ctx, c.controllerName)
	return c.cache.Get(ctx, key, factory)
}

// Stats returns current cache statistics.
func (c *controllerCache) Stats() CacheStats {
	return c.cache.Stats()
}

// controllerNameKey is the context key for controller name.
type controllerNameKey struct{}

// withControllerName returns a context with the controller name attached.
func withControllerName(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, controllerNameKey{}, name)
}

// getControllerName extracts the controller name from context.
// Returns "unknown" if not found.
func getControllerName(ctx context.Context) string {
	if name, ok := ctx.Value(controllerNameKey{}).(string); ok {
		return name
	}
	return "unknown"
}
