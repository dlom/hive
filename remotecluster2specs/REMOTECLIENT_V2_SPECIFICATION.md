# Remote Client Package - Implementation Specification

## Purpose

This document specifies the requirements for a modernized remote client package for OpenShift Hive. It describes the bugs and limitations in `pkg/remoteclient` and defines requirements for a new implementation supporting client caching, context-aware APIs, and automatic cache invalidation.

This specification is intended for:
- LLM code generation tools creating a modernized implementation
- Human developers implementing remote client builders
- Technical reviewers evaluating proposed solutions

## Background

The Hive remote client package (`pkg/remoteclient`) provides client creation and connection management for remote Kubernetes clusters. It handles kubeconfig extraction from secrets, API URL management (primary/secondary/override), reachability verification, and custom network routing.

The current implementation creates new clients on every call, performs blocking reachability checks without timeout control, and lacks cache invalidation strategies for certificate rotation and API URL failover. Controllers managing hundreds of clusters recreate clients repeatedly, causing performance overhead and memory churn.

**This specification focuses exclusively on client creation and connection management.** Infrastructure concerns (client caching, REST config utilities, discovery management, field manager naming, error types, metrics) are addressed in the Shared Client Utilities Specification. Resource operations (Apply, Patch, Delete) are addressed in the Resource Helper v2 Specification.

---

## Critical Bugs in Original Implementation

### 1. No Client Caching

**Location:** `pkg/remoteclient/remoteclient.go`, lines 201-222

Every call to `Build()`, `BuildDynamic()`, or `BuildKubeClient()` creates new clients. Controllers managing multiple clusters recreate clients on every reconciliation with no reuse across reconciliation loops.

**Impact:** Memory churn from repeated client creation/destruction. Discovery calls repeated unnecessarily. Controllers managing 1000 clusters spend significant time in client creation.

**Requirement:** v2 MUST support client caching with configurable LRU eviction and TTL-based expiration. Cache integration via shared utilities package.

### 2. No Context Support

**Location:** All Builder interface methods

```go
type Builder interface {
    Build() (client.Client, error)
    BuildDynamic() (dynamic.Interface, error)
    BuildKubeClient() (kubeclient.Interface, error)
    RESTConfig() (*rest.Config, error)
    UsePrimaryAPIURL() Builder
    UseSecondaryAPIURL() Builder
}
```

**Problem:** No `context.Context` parameter on any method. Cannot implement timeouts on client creation, cannot cancel in-flight operations, no distributed tracing support. Blocking discovery calls cannot be interrupted.

**Requirement:** v2 MUST provide context-aware methods accepting `context.Context` as first parameter. Maintain backward-compatible methods without context for gradual migration.

### 3. Potential REST Config Mutation

**Location:** `pkg/remoteclient/remoteclient.go`, line 268

```go
func (b *builder) RESTConfig() (*rest.Config, error) {
    cfg, err := unadulteratedRESTConfig(b.c, b.cd)
    if err != nil {
        return nil, err
    }

    utils.AddControllerMetricsTransportWrapper(cfg, b.controllerName, true)
    // cfg is mutated in-place, but it's a fresh copy so currently safe

    if override := b.cd.Spec.ControlPlaneConfig.APIURLOverride; override != "" {
        cfg.Host = override  // Mutating the config
    }
    // ...
}
```

**Problem:** Currently safe because `unadulteratedRESTConfig()` always returns a fresh config, but relies on implementation detail of `RestConfigFromSecret()`. Could break if that function changes to cache/reuse configs. Not obviously immutable from API perspective.

**Requirement:** v2 MUST use immutable REST config utilities from shared specification. Always use defensive copying before mutation. See Shared Client Utilities Specification for `CopyConfigWithMetrics()` and `PrepareConfigForClient()` functions.

### 4. Reachability Check Blocking Client Creation

**Location:** `pkg/remoteclient/remoteclient.go`, lines 206-214

```go
func (b *builder) Build() (client.Client, error) {
    cfg, err := b.RESTConfig()
    // ...

    // Verify reachability of client
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

**Problem:** Every client creation makes a blocking discovery call with no timeout control. Fails fast if cluster unreachable (good), but without caching this repeats every reconciliation.

**Requirement:** v2 MUST respect context timeouts for reachability checks. Use cached discovery clients from shared utilities to avoid repeated discovery calls. Allow reachability check to be optional (skip if client already cached).

### 5. No Cache Invalidation Strategy

**Problem:** If clients were cached, there's no mechanism to invalidate them when:
- Kubeconfig secret is updated (certificate rotation)
- API URL override changes (failover)
- Cluster becomes unreachable

**Requirement:** Cache key MUST include kubeconfig secret ResourceVersion and current API URL. When secret updates, ResourceVersion changes causing automatic cache miss. TTL-based expiration (recommended 10 minutes). Health checks (recommended 2 minutes) for automatic eviction. See Shared Client Utilities Specification for cache implementation.

### 6. Field Manager Inconsistency

**Location:** `pkg/remoteclient/remoteclient.go`, line 221

Uses `"hive2-" + controllerName` for field manager. Different from resource helper which uses `"hive4-"`, `"hive5-"`, `"hive6-"`, `"hive7-"`. Controllers using both packages have inconsistent field managers.

**Requirement:** v2 MUST use `FieldManagerName()` from shared utilities specification. Consistent naming across all Hive packages. Default format: `"hive-{controllername}"` without version prefix.

### 7. Discovery Client Recreation

**Location:** `pkg/remoteclient/remoteclient.go`, lines 207-214

Discovery client created for reachability check, then thrown away. Controller-runtime client creates its own internal discovery infrastructure. No reuse between verification step and actual client usage.

**Requirement:** v2 MUST use cached discovery clients from shared utilities. Reuse discovery client across multiple operations. See Shared Client Utilities Specification for discovery management.

---

## Architectural Flaws

### 1. No Multi-Cluster Client Reuse

Controllers managing multiple clusters create builders per reconciliation. Each builder creates clients independently. No sharing across controllers accessing same clusters.

**Requirement:** v2 MUST support shared cache across multiple controller instances. Controllers can inject same cache instance into multiple builders. Reduces memory footprint and improves performance.

### 2. Blocking Reachability Verification

Reachability verification happens synchronously during Build(). If cluster unreachable, Build() blocks until network timeout (default 30+ seconds). No way to set shorter timeout or cancel verification.

**Requirement:** v2 MUST respect context timeout for verification. Controllers can set reasonable timeout (e.g., 5 seconds) via context. Context cancellation interrupts verification immediately.

### 3. URL Failover Without Caching Awareness

API URL failover (primary to secondary) requires creating new builder and calling Build() again. No coordination with cache to invalidate old client and create new one with different URL.

**Requirement:** Cache key MUST include current API URL. Changing URL automatically invalidates cache entry. Next Build() creates client with new URL.

---

## Requirements for New Implementation

### Functional Requirements

#### Context-First API

All client creation methods MUST accept `context.Context`:
- `BuildWithContext(ctx)` - Create controller-runtime client
- `BuildDynamicWithContext(ctx)` - Create dynamic client
- `BuildKubeClientWithContext(ctx)` - Create typed Kubernetes client
- `RESTConfigWithContext(ctx)` - Get REST configuration

Maintain backward compatibility with non-context methods that delegate to context versions using `context.Background()`.

#### Client Caching Support

v2 MUST integrate with client cache from shared utilities:
- Accept cache via options: `WithCache(cache ClientCache)`
- Generate cache key from: cluster identifier, kubeconfig ResourceVersion, API URL
- Retrieve client from cache if present
- Create and cache client if not present
- Support cache-less operation (WithoutCache option)

See Shared Client Utilities Specification for ClientCache interface.

#### Immutable REST Config Handling

v2 MUST use REST config utilities from shared specification:
- Use `CopyConfigWithMetrics()` to apply metrics wrapper immutably
- Use `PrepareConfigForClient()` to apply URL and IP overrides immutably
- Never mutate input configs or builder state configs

See Shared Client Utilities Specification for config utility functions.

#### Cache Invalidation Strategy

Cache key construction enables automatic invalidation:

**Certificate Rotation:**
- Include kubeconfig secret ResourceVersion in cache key
- When secret updates, ResourceVersion changes
- Cache key no longer matches → automatic cache miss
- New client created with fresh credentials

**API URL Failover:**
- Include current API URL (primary, secondary, or override) in cache key
- When `ActiveAPIURLOverrideCondition` changes, `determineAPIURL()` returns different value
- Cache key no longer matches → automatic cache miss
- New client created with new URL

**TTL Expiration:**
- Recommended TTL: 10 minutes (balances performance vs staleness)
- Prevents holding stale credentials too long
- Automatic eviction after TTL

**Health Check Failures:**
- Recommended interval: 2 minutes
- Background health checks detect unreachable clusters
- Failed health check evicts client from cache
- Next access attempts fresh connection

**Manual Invalidation:**
- When cluster marked unreachable, call `cache.Invalidate(key)`
- When ClusterDeployment deleted, invalidate cache entry
- Exposed via cache interface

#### Reachability Management

v2 MUST provide reachability checking and condition management:
- Verify cluster reachable during Build (optional, can skip if cached)
- Update `UnreachableCondition` on ClusterDeployment based on check result
- Provide `Unreachable(cd)` function to check condition without connecting
- Return typed errors for different failure modes (network, auth, timeout)

See Shared Client Utilities Specification for ClusterError types.

#### API URL Failover Support

v2 MUST support API URL management:
- **Primary URL**: APIURLOverride if set, else kubeconfig URL
- **Secondary URL**: Alternate of the above
- **Active URL**: Determined by `ActiveAPIURLOverrideCondition` status
- Builder pattern: `builder.UsePrimaryAPIURL()` or `builder.UseSecondaryURL()`

URL selection immutable (returns new builder). Cache key includes selected URL for automatic invalidation on failover.

#### Custom Dialer Support

v2 MUST support custom dialer for API server IP override:
- APIServerIPOverride (HIVE-2272 workaround for Kubernetes memory leak)
- Replace hostname with IP while preserving port
- Support TCP only (return error for UDP)
- Validate address format (must have port)

See `pkg/remoteclient/dialer.go` for current implementation pattern.

### Performance Requirements

#### Client Reuse

First access creates client (~300ms with discovery). Cache hit returns existing client (<10ms map lookup). Cache miss rate <5% in multi-cluster scenarios (95%+ hit rate).

#### Non-Blocking Initialization

Builder creation must be fast (<1ms). Expensive operations (client creation, discovery) deferred until Build called. Context timeout prevents indefinite blocking.

#### Cache Efficiency

LRU eviction with configurable max size (recommended: 500 per controller). TTL expiration (recommended: 10 minutes). Health checks (recommended: 2 minute interval). Memory usage proportional to cached clients, not total clusters.

See Shared Client Utilities Specification for cache performance requirements.

### Non-Functional Requirements

#### Testing Support (Fake Builder)

v2 MUST provide fake builder for testing:
- Returns fake clients populated with realistic test data
- Simulates unreachable clusters (for testing error handling)
- Simulates API URL failover scenarios
- No network I/O required

Current `fakeBuilder` implementation provides good pattern to follow.

#### Metrics

v2 MUST instrument operations using metrics from shared utilities:
- Builder creation duration by type (controller-runtime, dynamic, typed)
- Cache hit/miss rates (via shared cache)
- Reachability check duration and result
- Client creation counts

See Shared Client Utilities Specification for metric definitions.

#### Thread-Safety

v2 MUST be thread-safe:
- Builders immutable (UsePrimaryAPIURL returns new builder)
- No shared mutable state between builders
- Cache operations thread-safe (handled by shared cache implementation)

---

## Implementation Guidance

### Builder Interface Design

**Context-Aware Interface:**

```go
type Builder interface {
    // Context-aware methods (preferred)
    BuildWithContext(ctx context.Context) (client.Client, error)
    BuildDynamicWithContext(ctx context.Context) (dynamic.Interface, error)
    BuildKubeClientWithContext(ctx context.Context) (kubeclient.Interface, error)
    RESTConfigWithContext(ctx context.Context) (*rest.Config, error)

    // Backward-compatible methods (delegate to context versions)
    Build() (client.Client, error)
    BuildDynamic() (dynamic.Interface, error)
    BuildKubeClient() (kubeclient.Interface, error)
    RESTConfig() (*rest.Config, error)

    // URL selection (returns new Builder, immutable)
    UsePrimaryAPIURL() Builder
    UseSecondaryAPIURL() Builder
}
```

**Functional Options Constructor:**

```go
type BuilderOption func(*builderConfig)

// Core options
func WithClusterDeployment(client client.Client, cd *hivev1.ClusterDeployment) BuilderOption
func WithKubeconfigSecret(secret *corev1.Secret) BuilderOption
func WithControllerName(name hivev1.ControllerName) BuilderOption

// Caching options
func WithCache(cache ClientCache) BuilderOption
func WithCacheTTL(duration time.Duration) BuilderOption
func WithoutCache() BuilderOption

// Network options
func WithPrimaryURL() BuilderOption
func WithSecondaryURL() BuilderOption

// Metrics options
func WithMetrics() BuilderOption

// Testing options
func WithFakeCluster(version string) BuilderOption
```

**Usage Example:**

Simple usage without caching:
```
builder := remoteclient.NewBuilder(
    remoteclient.WithClusterDeployment(client, cd),
    remoteclient.WithControllerName(ControllerName),
    remoteclient.WithMetrics(),
)

ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

remoteClient, err := builder.BuildWithContext(ctx)
```

With shared cache:
```
// Create shared cache once at controller initialization
cache := clientutil.NewLRUCache(
    clientutil.WithMaxSize(500),
    clientutil.WithTTL(10*time.Minute),
    clientutil.WithHealthCheckInterval(2*time.Minute),
)

// Use cache in builder
builder := remoteclient.NewBuilder(
    remoteclient.WithClusterDeployment(client, cd),
    remoteclient.WithControllerName(ControllerName),
    remoteclient.WithCache(cache),
)

// Cached client will be reused across reconciliations
remoteClient, err := builder.BuildWithContext(ctx)
```

### Cache Key Construction

Cache key MUST include:

```go
type CacheKey struct {
    ClusterID         string  // namespace/name of ClusterDeployment
    KubeconfigVersion string  // Secret ResourceVersion
    APIURL            string  // Current API URL in use
}
```

**Key Generation:**

1. For ClusterDeployment-based builder:
   - Fetch kubeconfig secret from ClusterDeployment reference
   - Extract secret ResourceVersion
   - Determine API URL based on primary/secondary/override selection
   - Construct key: `{namespace}/{name}#{resourceVersion}#{apiURL}`

2. For kubeconfig secret-based builder:
   - Use secret namespace/name as cluster ID
   - Extract secret ResourceVersion
   - Parse API URL from kubeconfig data
   - Construct key: `{namespace}/{name}#{resourceVersion}#{apiURL}`

**Critical for Invalidation:**

ResourceVersion changes trigger cache miss (certificate rotation). API URL changes trigger cache miss (failover). No manual invalidation needed for these cases.

### Automatic Invalidation Triggers

**Certificate Rotation (Automatic):**
- Kubeconfig secret updated → ResourceVersion changes
- Cache key no longer matches → cache miss
- New client created with fresh credentials
- Old client evicted by LRU or TTL

**API URL Failover (Automatic):**
- `ActiveAPIURLOverrideCondition` changes
- Builder determines different API URL
- Cache key no longer matches → cache miss
- New client created with new URL

**TTL Expiration (Automatic):**
- After 10 minutes (configurable), cache entry expires
- Next access creates fresh client
- Ensures credentials don't get too stale

**Health Check Failures (Automatic):**
- Background goroutine checks cluster health every 2 minutes
- Discovery call or health endpoint check
- On failure, evict from cache
- Next access attempts fresh connection

**Manual Invalidation:**
- Controller detects cluster unreachable
- Calls `cache.Invalidate(key)` explicitly
- Next access creates fresh client

### Shared Infrastructure Components

v2 MUST use shared utilities from shared specification:

**Client Caching:**
- Use `ClientCache` interface from `internal/clientutil/cache`
- LRU eviction, TTL expiration, health checks provided by shared implementation
- v2 only provides cache key generation

**REST Config Utilities:**
- Use `CopyConfigWithMetrics()` from `internal/clientutil/config`
- Use `PrepareConfigForClient()` for URL/IP overrides
- Immutability guaranteed by shared utilities

**Discovery Management:**
- Use `NewCachedDiscoveryClient()` from `internal/clientutil/discovery`
- In-memory caching, no disk I/O
- Shared across clients for same cluster

**Field Manager Naming:**
- Use `FieldManagerName()` from `internal/clientutil/fieldmanager`
- Consistent "hive-{controller}" format
- Replaces "hive2-" prefix

**Error Types:**
- Use `ClusterError` from `internal/clientutil/errors`
- Typed predicates: IsConnectionFailed, IsAuthenticationFailed, IsTimeout
- Wrap errors with cluster context

**Metrics:**
- Use metrics from `internal/clientutil/metrics`
- Transport wrapper, cache hit/miss, operation duration
- Consistent labels across packages

See Shared Client Utilities Specification for detailed requirements on each component.

### ClusterDeployment Integration

**Kubeconfig Loading:**

Extract kubeconfig from secret referenced in `ClusterDeployment.Spec.ClusterMetadata.AdminKubeconfigSecretRef`. Use `RestConfigFromSecret()` utility (or equivalent) to parse kubeconfig data into `rest.Config`.

**API URL Determination:**

1. Check `ClusterDeployment.Spec.ControlPlaneConfig.APIURLOverride`
2. Check `ActiveAPIURLOverrideCondition` status to determine active URL
3. For primary: Use override if set, else use kubeconfig URL
4. For secondary: Use alternate of the above
5. Cache key includes determined URL for automatic invalidation on change

**Unreachable Condition Management:**

Update `UnreachableCondition` on ClusterDeployment after reachability check:
- Success: Set status=False (cluster is reachable)
- Failure: Set status=True with reason and message
- Include last probe time
- Idempotent condition update (only update if changed)

**Custom Dialer for IP Override:**

If `ClusterDeployment.Spec.ControlPlaneConfig.APIServerIPOverride` set:
- Create custom dialer that replaces hostname with IP
- Preserve port from original address
- Apply to REST config Dial function
- See `pkg/remoteclient/dialer.go` for implementation pattern

### Custom Dialer for IP Override

Pattern from current implementation:

```go
type dialer struct {
    ipOverride string
}

func (d *dialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
    if network != "tcp" {
        return nil, fmt.Errorf("network %s not supported", network)
    }
    _, port, err := net.SplitHostPort(address)
    if err != nil {
        return nil, err
    }
    address = net.JoinHostPort(d.ipOverride, port)
    return (&net.Dialer{}).DialContext(ctx, network, address)
}
```

Requirements:
- Support TCP only (return error for UDP)
- Preserve port from original address
- Use net.Dialer for actual connection
- Respect context timeout and cancellation

---

## Migration Considerations

### Breaking Changes from Original API

v2 has intentional breaking changes:

1. **Context parameter:** New methods accept context as first parameter
2. **Functional options:** Constructor uses options pattern instead of positional parameters
3. **Cache integration:** Cache is optional but requires explicit configuration
4. **Field manager naming:** Changes from "hive2-" to "hive-{controller}"

### Compatibility Strategy

**Backward-Compatible Methods:**

Provide non-context methods that delegate to context versions:
```
func (b *builder) Build() (client.Client, error) {
    return b.BuildWithContext(context.Background())
}
```

Allows gradual migration without breaking existing code.

**Adapter Pattern:**

Create adapter implementing old interface using new builder:
```
func NewLegacyBuilder(client, cd, controllerName) LegacyBuilder {
    return &legacyAdapter{
        builder: NewBuilder(
            WithClusterDeployment(client, cd),
            WithControllerName(controllerName),
        ),
    }
}
```

Controllers can switch to new interface incrementally.

**Migration Path:**

1. Deploy v2 alongside v1
2. Migrate one controller at a time
3. Monitor metrics for performance improvements
4. Remove v1 after all controllers migrated

---

## Testing Requirements

### Unit Tests

**Builder Creation:**
- Test with ClusterDeployment
- Test with kubeconfig secret
- Test with various option combinations
- Test URL selection (primary, secondary)

**Client Creation:**
- Create controller-runtime client
- Create dynamic client
- Create typed Kubernetes client
- Verify field manager set correctly

**Cache Integration:**
- Cache hit returns existing client
- Cache miss creates new client
- Cache key includes ResourceVersion and API URL
- Manual invalidation removes client

**Context Handling:**
- Context timeout interrupts client creation
- Context cancellation stops reachability check
- Context without deadline uses reasonable default

**Error Handling:**
- Network errors wrapped as ClusterError
- Auth errors include cluster identifier
- Timeout errors distinguished from other errors

### Integration Tests

**Real Cluster Testing:**

Use envtest or kind:
- Create real clients for local cluster
- Verify clients functional (list pods, etc.)
- Test with unreachable cluster (fail gracefully)

**Cache Behavior:**
- LRU eviction (fill cache beyond max size)
- TTL expiry (use fake time or long test)
- Health check eviction (simulate unreachable cluster)

**Concurrent Operations:**
- Multiple goroutines building clients simultaneously
- Race detector must pass (`go test -race`)
- No client duplication (cache returns same instance)

### Fake Builder Tests

**Fake Cluster Simulation:**
- Fake builder returns fake client
- Fake client populated with test resources
- No network I/O required
- Simulates different cluster versions

---

## Success Criteria

### Performance Metrics

- Client creation (uncached): <500ms (p99)
- Client creation (cached): <10ms (p99)
- Cache hit rate: >95% for multi-cluster scenarios
- Memory growth: 0% over 24 hours

### Reliability Metrics

- Zero concurrency bugs (race detector clean)
- Proper error handling (100% of errors wrapped with context)
- Thread-safe for concurrent use
- Context cancellation handled correctly

### Code Quality Metrics

- Test coverage: >80%
- Complete API documentation
- Migration guide from v1
- Integration with shared utilities verified

---

## Appendix: File Reference

### Original Implementation Files (FOR UNDERSTANDING CURRENT BEHAVIOR)

- `pkg/remoteclient/remoteclient.go` - Main builder implementation
- `pkg/remoteclient/kubeconfig.go` - Kubeconfig secret builder
- `pkg/remoteclient/dialer.go` - Custom dialer for IP override
- `pkg/remoteclient/fake.go` - Fake builder for testing

### Controller Usage Examples (FOR UNDERSTANDING USAGE PATTERNS)

- `pkg/controller/clusterstate/clusterstate_controller.go` - Builder creation and usage
- `pkg/controller/clustersync/clustersync_controller.go` - Integration with resource helper
- `pkg/controller/unreachable/unreachable_controller.go` - Reachability checking and failover
- `pkg/controller/hibernation/hibernation_controller.go` - Multiple client types (Build, BuildKubeClient)

### Related Specifications

- `SHARED_CLIENT_UTILITIES_SPECIFICATION.md` - Infrastructure components (caching, config, discovery, errors, metrics)
- `RESOURCE_HELPER_V2_SPECIFICATION.md` - Resource operations (Apply, Patch, Delete) using clients from this package

---

## Document Maintenance

This specification should be updated when:
- New client types needed (beyond controller-runtime, dynamic, typed)
- ClusterDeployment API evolves
- New reachability checking requirements emerge
- Integration patterns with shared utilities change

---

Last updated: 2026-03-11
