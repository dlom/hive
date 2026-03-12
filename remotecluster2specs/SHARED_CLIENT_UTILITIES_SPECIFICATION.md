# Shared Client Utilities Package - Implementation Specification

## Purpose

This document specifies the requirements for `internal/clientutil`, a shared utility package providing Kubernetes client management infrastructure for Hive's remote cluster operations. It addresses common technical debt identified in both `pkg/resource` and `pkg/remoteclient` packages.

This specification is intended for:
- LLM code generation tools creating shared infrastructure
- Human developers implementing utility components
- Technical reviewers evaluating modernization proposals

## Background

Hive controllers manage hundreds of remote Kubernetes clusters concurrently through two complementary packages:
- `pkg/remoteclient` - Client creation and connection management
- `pkg/resource` - Resource operations (Apply, Patch, Delete)

Both packages independently implement client lifecycle management, REST configuration handling, field manager naming, and metrics collection. This duplication has led to inconsistent implementations, memory leaks, and performance issues. Controllers using both packages suffer from duplicated client creation overhead (3+ seconds per operation) and unbounded memory growth.

Analysis of both packages identified five infrastructure components that should be shared: client caching, REST config utilities, field manager naming, error types, and metrics. Implementing these as shared utilities eliminates duplication, ensures consistency, and enables performance improvements across all Hive controllers.

---

## Critical Issues Requiring Shared Infrastructure

### From pkg/resource Package

#### REST Config Mutation Leading to Memory Leaks

**Location:** `pkg/resource/restconfig_factory.go`, lines 13-22

```go
func (r *helper) getRESTConfigFactory(namespace string) (cmdutil.Factory, error) {
    if r.metricsEnabled {
        cfg := rest.CopyConfig(r.restConfig)
        controllerutils.AddControllerMetricsTransportWrapper(cfg, r.controllerName, false)
        r.restConfig = cfg  // BUG: Mutating the helper's field!
    }
    //...
}
```

Despite copying the config, the code mutates `r.restConfig` field. First call with metrics enabled permanently changes the helper's REST config. Repeated calls accumulate metric wrappers, causing memory leaks as transport wrappers are never garbage collected.

#### Factory and Discovery Client Recreation on Every Operation

**Locations:** `pkg/resource/apply.go`, `pkg/resource/delete.go`, `pkg/resource/patch.go`

Every public method (Apply, CreateOrUpdate, Create, Delete, Info, Patch) creates a new factory via `r.getFactory()`. Each factory creation creates new discovery clients that hit disk cache repeatedly. No client reuse across operations.

#### Disk-Based Discovery Caching with Race Conditions

**Location:** `pkg/resource/factory_discovery.go`, lines 14-19

```go
func getDiscoveryClient(config *rest.Config, cacheDir string) (discovery.CachedDiscoveryInterface, error) {
    httpCacheDir := filepath.Join(cacheDir, ".kube", "http-cache")
    discoveryCacheDir := computeDiscoverCacheDir(filepath.Join(cacheDir, ".kube", "cache", "discovery"), config.Host)
    return disk.NewCachedDiscoveryClientForConfig(config, discoveryCacheDir, httpCacheDir, time.Duration(10*time.Minute))
}
```

Every discovery client reads/writes to disk cache in `/tmp`. Multiple concurrent helpers write to shared cache directory causing race conditions. 10-minute TTL hardcoded with no configurability.

#### Inconsistent Field Manager Naming

**Locations:** `pkg/resource/apply.go`, lines 159, 185, 233; `pkg/resource/patch.go`, line 57

Four different field manager prefixes used across different operations:
- `"hive4-" + controllerName` in Create method
- `"hive5-" + controllerName` in CreateOrUpdate method
- `"hive6-" + controllerName` in Apply method
- `"hive7-" + controllerName` in Patch method (via os.Args mutation)

These get embedded in Kubernetes API as field ownership metadata, causing conflicts when same resource managed by different code paths.

### From pkg/remoteclient Package

#### No Client Caching

**Location:** `pkg/remoteclient/remoteclient.go`, lines 201-222

Every call to `Build()`, `BuildDynamic()`, or `BuildKubeClient()` creates new clients. Controllers managing multiple clusters recreate clients on every reconciliation. Discovery calls repeated unnecessarily with memory churn from client creation/destruction. No reuse across reconciliation loops.

#### Reachability Check Blocking Client Creation

**Location:** `pkg/remoteclient/remoteclient.go`, lines 206-214

```go
func (b *builder) Build() (client.Client, error) {
    cfg, err := b.RESTConfig()
    // ...
    dc, err := discovery.NewDiscoveryClientForConfig(cfg)
    if err != nil {
        return nil, err
    }
    _, err = restmapper.GetAPIGroupResources(dc)  // BLOCKS for network I/O
    if err != nil {
        return nil, err
    }
    // ...
}
```

Every client creation makes a blocking discovery call with no timeout control. Without caching, this repeats on every reconciliation.

#### Field Manager Inconsistency with Resource Helper

**Location:** `pkg/remoteclient/remoteclient.go`, line 221

Uses `"hive2-" + controllerName` for field manager. Controllers using both remoteclient and resource helper have different field managers for same operations, causing confusing field ownership tracking.

### From pkg/controller/utils Package

#### Conditional Metrics Wrapper Bug

**Location:** `pkg/controller/utils/clientwrapper.go`, lines 84-100

```go
func AddControllerMetricsTransportWrapper(cfg *rest.Config, controllerName hivev1.ControllerName, remote bool) {
    if cfg.Wrap != nil {
        // Already wrapped, check if it's our wrapper type
        if _, ok := cfg.Wrap(nil).(*ControllerMetricsTripper); ok {
            return  // BUG: Skips wrapping if already wrapped, should always wrap
        }
    }
    cfg.Wrap = func(rt http.RoundTripper) http.RoundTripper {
        return &ControllerMetricsTripper{
            RoundTripper:   rt,
            controllerName: string(controllerName),
            remote:         remote,
        }
    }
}
```

Conditional logic incorrectly skips wrapping if config already has a wrapper. This causes metrics to not be collected when configs are reused.

---

## Requirements

### Client Cache Interface

#### Core Functionality

MUST provide LRU cache with configurable maximum size (recommended: 500 clients per controller) and TTL-based expiration (recommended: 10 minutes). Cache operations must be thread-safe for concurrent access from multiple goroutines.

#### Cache Key Requirements

Cache key MUST include:
- Cluster identifier (namespace/name)
- Kubeconfig secret ResourceVersion (enables automatic invalidation on certificate rotation)
- API URL currently in use (enables automatic invalidation on failover)

When kubeconfig secret updates, ResourceVersion changes causing automatic cache miss. When API URL override changes, cache key no longer matches causing automatic cache miss.

#### Automatic Invalidation Triggers

1. **Certificate Rotation**: Kubeconfig secret ResourceVersion change invalidates cache entry
2. **API URL Failover**: API URL change in cache key forces cache miss
3. **TTL Expiration**: Recommended 10 minutes due to frequent certificate rotation in production
4. **Health Check Failures**: Recommended 2-minute intervals with immediate eviction on failure
5. **Manual Eviction**: Expose API for controller-driven cache invalidation

#### Cache Operations

- `Get(ctx, key, factory)` - Retrieve cached client or create via factory function
- `Invalidate(key)` - Force removal of specific cache entry
- `InvalidateAll()` - Clear entire cache
- `Stats()` - Return cache metrics (hits, misses, evictions, size, age distribution)

#### Eviction Policy

When cache reaches maximum size, evict least recently used (LRU) entry. Track access time for each entry to enable LRU policy. Eviction must be thread-safe and not block cache access operations.

---

### REST Config Utilities

#### Immutability Requirement

All functions MUST treat REST configs as immutable. Functions must return new configs and never modify input parameters. This eliminates config mutation bugs present in original implementations.

#### Required Functions

1. **CopyConfigWithMetrics** - Deep copy REST config and apply metrics wrapper once
   - Parameters: config, controller name, remote flag (boolean)
   - Returns: New config with metrics wrapper applied
   - Must not accumulate wrappers on repeated calls

2. **PrepareConfigForClient** - Apply URL and IP overrides defensively
   - Parameters: config, API URL override, IP override
   - Returns: New config with overrides applied
   - Must copy config before applying any modifications

3. **ConfigEquals** - Compare two configs for cache key generation
   - Parameters: config1, config2
   - Returns: Boolean indicating equality
   - Compare relevant fields: host, bearer token hash, cert data hash

#### Wrapper Accumulation Prevention

Detect existing metrics wrappers and apply only once. Do NOT skip wrapping entirely if wrapper exists (this is the bug in controller/utils). Instead, detect if OUR specific wrapper is present and skip only in that case.

#### Defensive Copying

Always use `rest.CopyConfig()` before any mutation. Preserve all fields including: Host, APIPath, ContentConfig, Username, Password, BearerToken, BearerTokenFile, Impersonate, TLSClientConfig, UserAgent, QPS, Burst, RateLimiter, Timeout, Dial, WrapTransport, ExecProvider.

---

### Field Manager Naming

#### Single Source of Truth

Provide single function for field manager name generation. All packages MUST use this function to ensure consistency.

#### Function Signature

`FieldManagerName(controllerName) string`
- Returns: `"hive-" + string(controllerName)`
- Example: `FieldManagerName("clustersync")` returns `"hive-clustersync"`

#### Deprecation of Legacy Prefixes

Document but do not use legacy prefixes:
- `"hive1-"` - Used by controller/utils
- `"hive2-"` - Used by remoteclient
- `"hive4-"`, `"hive5-"`, `"hive6-"`, `"hive7-"` - Used by resource helper

New implementations must use unified naming scheme without version prefixes.

#### Migration Support

Provide optional `FieldManagerNameLegacy(controllerName, version)` for migration scenarios where controller must temporarily use old field manager names during transition period. Document that this is temporary and should be removed after migration completes.

---

### Error Types

#### ClusterError Structure

Define structured error type for cluster operations:
- Cluster identifier (namespace/name)
- Operation type (e.g., "build-client", "apply", "patch", "delete")
- GVK (Group/Version/Kind) if applicable
- Namespace and name of resource if applicable
- Underlying cause error

#### Standard Error Predicates

Export functions for error type checking:
- `IsNotFound(err)` - Resource does not exist
- `IsAlreadyExists(err)` - Resource already exists (cannot create)
- `IsConflict(err)` - Update conflict (optimistic concurrency failure)
- `IsTimeout(err)` - Operation timed out
- `IsConnectionFailed(err)` - Network connection failed
- `IsAuthenticationFailed(err)` - Authentication or authorization failed
- `IsInvalidResource(err)` - Resource validation failed

#### Error Wrapping

Provide `WrapClusterError(err, clusterID, operation, resource)` function to wrap errors with cluster context. Support Go error unwrapping via `errors.Is()` and `errors.As()`.

#### Error Categories

Organize errors into categories:
1. **Connection Errors** - Network, DNS, certificate issues
2. **Authentication/Authorization Errors** - Credentials, permissions
3. **API Errors** - NotFound, AlreadyExists, Conflict, Invalid, Forbidden
4. **Timeout/Cancellation Errors** - Context deadline, cancellation

---

### Metrics Infrastructure

#### Transport Wrapper Implementation

Single implementation of `AddControllerMetricsTransportWrapper(cfg, controllerName, remote)` to replace buggy version in controller/utils. Must fix conditional wrapping bug by always applying wrapper but detecting duplicate wrapping of same type.

#### Standard Metric Definitions

**Transport Metrics (existing):**
- `hive_kube_client_requests_total{controller, method, resource, remote, status}` - Request count
- `hive_kube_client_request_seconds{controller, method, resource, remote}` - Request duration histogram
- `hive_kube_client_cancellations_total{controller, remote}` - Cancellation count

**Cache Metrics (new):**
- `hive_client_cache_hits_total{package, controller}` - Cache hit count
- `hive_client_cache_misses_total{package, controller}` - Cache miss count
- `hive_client_cache_size{package, controller}` - Current cache size
- `hive_client_cache_evictions_total{package, controller, reason}` - Evictions by reason (lru, ttl, health, manual)

**Operation Metrics (new):**
- `hive_resource_operation_duration_seconds{controller, operation, gvk, result}` - Operation duration
- `hive_resource_operation_total{controller, operation, gvk, result}` - Operation count

#### Consistent Labeling

All metrics must use consistent label names:
- `controller` - Controller name (e.g., "clustersync")
- `method` - HTTP method (GET, POST, PUT, PATCH, DELETE)
- `resource` - Kubernetes resource (e.g., "pods")
- `remote` - Boolean indicating remote cluster vs local
- `status` - HTTP status code
- `package` - Package name ("remoteclient" or "resource")
- `operation` - Operation type (apply, patch, delete, create, update)
- `gvk` - Group/Version/Kind of resource
- `result` - Operation result (success, failure, conflict, timeout)
- `reason` - Eviction reason (lru, ttl, health, manual)

#### Metric Registration

Register all metrics at initialization time (init function or package-level var). Do NOT register metrics at runtime as this causes race conditions with Prometheus registry.

---

## Implementation Guidance

### Package Location and Organization

Create package at `internal/clientutil/` (NOT `pkg/internal/clientutil`). The `internal/` directory already provides non-exported scope; `pkg/internal` is redundant.

Organize into subpackages by function:
- `internal/clientutil/cache/` - Client cache implementation
- `internal/clientutil/config/` - REST config utilities
- `internal/clientutil/errors/` - Error types and predicates
- `internal/clientutil/metrics/` - Metrics infrastructure
- `internal/clientutil/fieldmanager/` - Field manager naming

Main `internal/clientutil/` package imports and re-exports commonly used types and functions from subpackages.

### Thread-Safety Requirements

All cache operations MUST be thread-safe. Use `sync.RWMutex` for cache map protection. Read operations use RLock for concurrent reads. Write operations (insert, evict, invalidate) use full Lock.

REST config utility functions must be pure (no shared mutable state). They should not rely on package-level variables except for metric registration.

Metrics registration must occur only at init time. Metric collection (increment, observe) is thread-safe in Prometheus client library.

Run all tests with `go test -race ./internal/clientutil/...` to detect data races.

### Testing Strategy

#### Unit Tests for Each Component

1. **Cache Tests**
   - LRU eviction behavior (verify least recently used evicted first)
   - TTL expiration (use fake time for deterministic testing)
   - Concurrent access with multiple goroutines (race detector must pass)
   - Cache key equality and hashing
   - Metrics accuracy (hits, misses, evictions recorded correctly)
   - Health check eviction (manual trigger health check failure)

2. **REST Config Tests**
   - Deep copy correctness (modify copy, verify original unchanged)
   - Immutability verification (function calls don't modify inputs)
   - Metrics wrapper application (verify wrapper present exactly once)
   - Custom dialer integration (verify dialer set correctly)
   - Config equality comparison (identical configs return true, different return false)

3. **Field Manager Tests**
   - Name generation (verify format matches specification)
   - Controller name escaping (handle special characters)
   - Legacy name generation (verify backward compatibility)

4. **Error Type Tests**
   - Error type identification (IsNotFound, IsTimeout, etc.)
   - Error wrapping and unwrapping (errors.Is and errors.As work correctly)
   - Error message formatting (includes cluster, operation, resource details)
   - Nil error handling (predicates return false for nil errors)

6. **Metrics Tests**
   - Wrapper application (verify wrapper present)
   - Duplicate wrapper detection (verify not double-wrapped)
   - Metric collection (verify counters/histograms updated)
   - Label consistency (verify all expected labels present)

#### Integration Tests

Create integration tests that use real Kubernetes API server (via envtest or kind):
1. Cache integration with real client creation
2. Metrics collection end-to-end
3. Concurrent operations from multiple controllers

#### Benchmarks

Benchmark critical paths:
1. Cache Get performance (hit vs miss)
2. REST config copy performance
3. Error wrapping overhead

Target: Cache Get (hit) < 1μs, Cache Get (miss with creation) < 100ms, Config copy < 10μs

### Dependencies

**Required Kubernetes libraries:**
- `k8s.io/client-go/rest` - REST config types
- `k8s.io/client-go/discovery` - Discovery client interfaces
- `k8s.io/client-go/dynamic` - Dynamic client types
- `sigs.k8s.io/controller-runtime/pkg/client` - Controller-runtime client interface

**Required instrumentation libraries:**
- `github.com/prometheus/client_golang/prometheus` - Metrics registration and collection

**Standard library only:**
- `sync` - Mutex and thread-safe primitives
- `time` - TTL and health check timing
- `errors` - Error wrapping and unwrapping
- `net/http` - Transport wrapper implementation

**NO kubectl dependencies:** This package must not import `k8s.io/kubectl` as that is the coupling being removed.

---

## Appendix: Integration Points

### Usage by pkg/remoteclient

remoteclient v2 will use:
- `cache.ClientCache` interface for caching built clients
- `config.CopyConfigWithMetrics()` for immutable config handling
- Lightweight `ServerVersion()` discovery calls for reachability checks (not cached)
- `fieldmanager.FieldManagerName()` for consistent naming
- `errors.ClusterError` for typed errors
- `metrics.*` for transport wrapper and cache metrics

remoteclient provides cache key by combining ClusterDeployment namespace/name, kubeconfig secret ResourceVersion, and current API URL.

### Usage by pkg/resource

resource helper v2 will use:
- `cache.ClientCache` interface for caching resource clients
- `config.CopyConfigWithMetrics()` for config preparation
- No discovery needed (SSA doesn't require discovery)
- `fieldmanager.FieldManagerName()` for Apply/Patch operations
- `errors.ClusterError` with operation-specific predicates
- `metrics.*` for operation duration and result counting

resource helper provides cache key based on REST config identity (cluster URL and auth).

### Shared Cache Pattern

Controllers may create single cache instance shared by both remoteclient and resource helper:

```
cache := clientutil.NewCache(
    clientutil.WithMaxSize(500),
    clientutil.WithTTL(10*time.Minute),
    clientutil.WithHealthCheckInterval(2*time.Minute),
)

// Pass same cache to both packages
remoteBuilder := remoteclient.NewBuilder(
    remoteclient.WithCache(cache),
    // ... other options
)

helper := resource.NewHelper(
    resource.WithCache(cache),
    // ... other options
)
```

Shared cache reduces memory footprint when controllers use both packages to access same clusters.

### Cross-Package Consistency

Field manager names from `fieldmanager.FieldManagerName()` MUST be used consistently:
- remoteclient sets field owner when building controller-runtime clients
- resource helper sets field manager when performing Apply/Patch operations
- Both must use same name for same controller to avoid field ownership conflicts

Error types from `errors` package MUST be used by both packages for consistent error handling in controllers.

Metrics from `metrics` package MUST be used by both packages to provide unified observability.

---

## Document Maintenance

This specification should be updated when:
- New shared infrastructure requirements emerge
- Performance targets change
- Kubernetes client libraries evolve
- Additional packages need to share these utilities

---

Last updated: 2026-03-11
