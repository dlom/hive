// Package clientutil provides shared infrastructure for Kubernetes client management.
//
// # Subpackages
//
//   - cache: Thread-safe LRU client cache with TTL expiration
//   - config: Immutable REST config utilities with metrics and kubeconfig parsing
//   - errors: Cluster-context error wrapping
//   - fieldmanager: Unified field manager naming for Server-Side Apply
//   - metrics: HTTP transport metrics for Kubernetes client requests
//
// # Usage
//
// Get the shared cache and use it with remote client builder:
//
//	cache := clientutil.GetSharedCache(hivev1.MyControllerName)
//	builder := remoteclient.NewBuilderWithOptions(
//	    remoteclient.WithClusterDeployment(client, cd),
//	    remoteclient.WithControllerName(hivev1.MyControllerName),
//	    remoteclient.WithCache(cache),
//	)
//	remoteClient, err := builder.BuildWithContext(ctx)
//
// # Cache Invalidation
//
// The cache automatically invalidates entries when the CacheKey changes:
//   - Kubeconfig secret ResourceVersion changes (certificate rotation)
//   - API URL changes (failover between primary/secondary)
//   - TTL expires (default: 10 minutes, configurable)
//
// # Field Manager Naming
//
// Use unified field manager naming for Server-Side Apply:
//
//	fieldManager := clientutil.FieldManagerName(controllerName)
//	// Returns: "hive-{controllername}"
//
// # Error Wrapping
//
// Wrap errors with cluster and operation context:
//
//	err := clientutil.WrapClusterError(
//	    originalErr,
//	    "hive/my-cluster",
//	    "apply",
//	    gvk,
//	    namespace,
//	    name,
//	)
//
// The wrapped error preserves the underlying error for use with errors.Is and errors.As.
//
// # Thread Safety
//
// All exported functions and types are thread-safe:
//   - Cache uses sync.RWMutex for concurrent access
//   - Config utilities are pure functions (no shared state)
//   - Metrics are thread-safe (Prometheus guarantee)
package clientutil
