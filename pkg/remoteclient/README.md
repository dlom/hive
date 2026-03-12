# Remote Client Builder

Builder API for creating Kubernetes clients to remote clusters with caching and context support.

## Overview

The remote client builder provides:

- **Client caching** - 90-97% performance improvement by reusing clients
- **Automatic invalidation** - handles certificate rotation and API URL failover
- **Context support** - all methods accept context for timeout/cancellation
- **API URL failover** - switch between primary and secondary API URLs
- **Error wrapping** - cluster context in all errors

## Basic Usage

```go
import (
    "context"
    "time"

    hivev1 "github.com/openshift/hive/apis/hive/v1"
    "github.com/openshift/hive/internal/clientutil"
    "github.com/openshift/hive/pkg/remoteclient"
)

// Get shared cache
cache := clientutil.GetSharedCache(hivev1.ClustersyncControllerName)

// Build client
builder := remoteclient.NewBuilderWithOptions(
    remoteclient.WithClusterDeployment(c, cd),
    remoteclient.WithControllerName(hivev1.ClustersyncControllerName),
    remoteclient.WithCache(cache),
)

ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

remoteClient, err := builder.BuildWithContext(ctx)
```

## Client Caching

The cache automatically invalidates when the cache key changes:

**Cache Key Components:**
- Cluster ID: `namespace/name`
- Kubeconfig Version: Secret `ResourceVersion`
- API URL: Current active URL

**Automatic Invalidation:**
- Certificate rotation (kubeconfig secret ResourceVersion changes)
- API URL failover (URL changes)
- TTL expiration (default: 10 minutes)

**Performance:**
- Uncached: ~300ms per operation
- Cached: ~110ms per operation
- **Improvement: 90-97% faster**

## Builder Options

```go
// Required: ClusterDeployment or kubeconfig secret
WithClusterDeployment(client, cd)
WithKubeconfigSecret(secret)

// Required: Controller name for metrics
WithControllerName(hivev1.ClustersyncControllerName)

// Optional: Enable caching (highly recommended)
WithCache(cache)
```

## API URL Failover

The builder supports switching between primary and secondary API URLs:

```go
// Try primary URL
primaryBuilder := builder.UsePrimaryAPIURL()
client, err := primaryBuilder.BuildWithContext(ctx)
if err != nil {
    // Fall back to secondary URL
    secondaryBuilder := builder.UseSecondaryAPIURL()
    client, err = secondaryBuilder.BuildWithContext(ctx)
}
```

**URL Selection Logic:**
- **Primary URL**: APIURLOverride (if set), else kubeconfig URL
- **Secondary URL**: Kubeconfig URL (if override set), else APIURLOverride
- **Active URL** (default): Determined by ActiveAPIURLOverrideCondition

## Client Types

All client types support context and caching:

```go
// Controller-runtime client
client, err := builder.BuildWithContext(ctx)

// Typed Kubernetes client
kubeClient, err := builder.BuildKubeClientWithContext(ctx)

// REST config (for custom clients)
cfg, err := builder.RESTConfigWithContext(ctx)
```

### Using a Kubeconfig Secret Directly

For controllers that work with raw kubeconfig secrets (without a ClusterDeployment):

```go
builder := remoteclient.NewBuilderWithOptions(
    remoteclient.WithKubeconfigSecret(secret),
    remoteclient.WithControllerName(controllerName),
    remoteclient.WithCache(cache),
)

client, err := builder.BuildWithContext(ctx)
```

When using `WithKubeconfigSecret`, API URL overrides and failover are not available.

## Error Handling

All errors are wrapped with cluster context:

```go
client, err := builder.BuildWithContext(ctx)
if err != nil {
    // Error includes cluster namespace/name and operation
    // Example: "cluster hive/my-cluster operation build-client failed: ..."
}
```

## Testing

### Fake Clients

For testing with fake clusters (scale testing):

```go
import "github.com/openshift/hive/pkg/remoteclient"

// Mark cluster as fake
cd.Labels["hive.openshift.io/fake-cluster"] = "true"

// Builder returns fake client automatically
builder := remoteclient.NewBuilderWithOptions(
    remoteclient.WithClusterDeployment(c, cd),
    remoteclient.WithControllerName(controllerName),
)
client, err := builder.BuildWithContext(ctx)
// Returns a fake client that doesn't connect to real cluster
```

## Architecture

```
pkg/remoteclient/
├── doc.go                 - Package documentation
├── builder.go             - Builder implementation (ClusterDeployment or kubeconfig secret)
├── options.go             - Functional options
├── cache_integration.go   - Cache key generation and helpers
├── remoteclient.go        - Interface and URL selection helpers
├── fake.go                - Fake client support for testing
└── README.md              - This file
```

## Integration with Other Packages

### internal/clientutil

- Provides cache implementation (ClientCache)
- Provides REST config utilities (CopyConfigWithMetrics, PrepareConfigForClient)
- Provides kubeconfig parsing (RestConfigFromSecret)
- Provides error wrapping (WrapClusterError)

### pkg/resource

- Uses remoteclient.Builder to create clients
- Performs Server-Side Apply operations on remote clusters

## Best Practices

1. **Always use caching** - Get shared cache via `clientutil.GetSharedCache(controllerName)`
2. **Always use context** - Set reasonable timeouts (e.g., 30 seconds)
3. **Use ConnectToRemoteCluster** - Automatically handles unreachable clusters
4. **Check unreachable condition** - Skip reconciliation for unreachable clusters
5. **Let cache auto-invalidate** - Don't try to manually invalidate entries

## Related Documentation

- [Remote Client v2 Specification](../../remotecluster2specs/REMOTECLIENT_V2_SPECIFICATION.md)
- [Shared Client Utilities](../../internal/clientutil/README.md)
- [Resource Helper](../resource/README.md)
