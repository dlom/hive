# Client Utilities Package

Shared infrastructure for Kubernetes client management in Hive.

## Overview

The `internal/clientutil` package provides:

- **Client caching** with LRU eviction and TTL expiration
- **REST config utilities** with immutable operations and metrics
- **Kubeconfig parsing** with security validation (HIVE-2485)
- **Error wrapping** with cluster and operation context
- **Field manager naming** for Server-Side Apply consistency
- **HTTP transport metrics** for observability

## Package Structure

```
internal/clientutil/
├── cache/           # LRU client cache with TTL
├── config/          # REST config and kubeconfig utilities
├── errors/          # Cluster-context error wrapping
├── fieldmanager/    # Field manager naming conventions
├── metrics/         # HTTP transport metrics
└── clientutil.go    # Re-exports for convenience
```

## Usage

### Shared Cache

The shared cache is used exclusively for remote cluster clients via `pkg/remoteclient.Builder`:

```go
import (
    hivev1 "github.com/openshift/hive/apis/hive/v1"
    "github.com/openshift/hive/internal/clientutil"
    "github.com/openshift/hive/pkg/remoteclient"
)

// Get controller-specific cache view
cache := clientutil.GetSharedCache(hivev1.ClustersyncControllerName)

// Use with remote client builder
builder := remoteclient.NewBuilderWithOptions(
    remoteclient.WithClusterDeployment(c, cd),
    remoteclient.WithControllerName(hivev1.ClustersyncControllerName),
    remoteclient.WithCache(cache),
)

ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

remoteClient, err := builder.BuildWithContext(ctx)
```

### REST Config Utilities

All config operations are immutable - they return new configs without modifying the input:

```go
// Add metrics wrapper
cfg := clientutil.CopyConfigWithMetrics(
    originalCfg,
    hivev1.ClustersyncControllerName,
    true, // remote=true for remote cluster
)

// Apply URL and IP overrides
cfg = clientutil.PrepareConfigForClient(
    cfg,
    "https://api-override.example.com:6443", // API URL override
    "10.0.0.1",                               // IP override
)

// Parse kubeconfig from secret
cfg, err := clientutil.RestConfigFromSecret(kubeconfigSecret, false)

// Validate kubeconfig for security (HIVE-2485)
config, err := clientutil.ValidateKubeconfig(kubeconfigData)
```

### Error Wrapping

Wrap errors with cluster and operation context for better debugging:

```go
err := clientutil.WrapClusterError(
    originalErr,
    "hive/my-cluster",
    "apply",
    schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
    "default",
    "my-deployment",
)

// Error includes full context:
// "cluster hive/my-cluster operation apply on apps/v1, Kind=Deployment default/my-deployment failed: ..."
```

### Field Manager Naming

Use consistent field manager naming for Server-Side Apply:

```go
fieldManager := clientutil.FieldManagerName(hivev1.ClustersyncControllerName)
// Returns: "hive-clustersync"

client := client.WithFieldOwner(baseClient, fieldManager)
```

## Cache Invalidation

The cache automatically invalidates entries when the `CacheKey` changes:

- **Certificate rotation**: Kubeconfig secret `ResourceVersion` changes
- **API URL failover**: Active API URL changes (primary ↔ secondary)
- **TTL expiration**: Entries older than TTL (default: 10 minutes)

Cache keys are computed from:
```go
type CacheKey struct {
    ClusterID         string  // "namespace/name"
    KubeconfigVersion string  // Secret ResourceVersion
    APIURL            string  // Current API URL
}
```

When any component changes, the cache treats it as a miss and creates a new client.

## Metrics

The package exports HTTP transport metrics for all Kubernetes client requests:

```
hive_kube_client_requests_total{controller, method, resource, remote, status}
hive_kube_client_request_seconds{controller, method, resource, remote, status}
hive_kube_client_requests_cancelled_total{controller, method, resource, remote}
```

Cache metrics (private, used internally):
```
hive_client_cache_hits_total{controller}
hive_client_cache_misses_total{controller}
hive_client_cache_size{controller}
hive_client_cache_evictions_total{controller, reason}
```

All metrics are automatically recorded - no manual instrumentation needed.

## Thread Safety

All operations are thread-safe:

- **Cache**: Uses `sync.RWMutex` for concurrent access
- **Config utilities**: Pure functions with no shared state
- **Metrics**: Thread-safe via Prometheus guarantees

Verified with:
```bash
go test -race ./internal/clientutil/...
```

## Testing

```bash
# Run all tests
go test ./internal/clientutil/...

# With race detector
go test -race ./internal/clientutil/...

# With coverage
go test -cover ./internal/clientutil/...
```

## Integration

### pkg/remoteclient

Uses clientutil for:
- Client caching with automatic invalidation
- REST config preparation with metrics
- Kubeconfig parsing and validation
- Error wrapping with cluster context

### pkg/resource

Uses clientutil for:
- Field manager naming for Server-Side Apply
- Error wrapping with operation context

### Controllers

Controllers should:
1. Get shared cache via `GetSharedCache(controllerName)`
2. Pass cache to `remoteclient.Builder` options
3. Use `FieldManagerName()` for Server-Side Apply
4. Wrap errors with `WrapClusterError()` for better debugging

## Related Documentation

- [Shared Client Utilities Specification](../../remotecluster2specs/SHARED_CLIENT_UTILITIES_SPECIFICATION.md)
- [Remote Client v2 Specification](../../remotecluster2specs/REMOTECLIENT_V2_SPECIFICATION.md)
- [Resource Helper v2 Specification](../../remotecluster2specs/RESOURCE_HELPER_V2_SPECIFICATION.md)
