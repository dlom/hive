# Hive Client Infrastructure Modernization - Overview

## Executive Summary

This modernization effort addresses critical technical debt in Hive's remote cluster management infrastructure by rewriting `pkg/resource` and `pkg/remoteclient` packages. The redesign introduces shared infrastructure in `internal/clientutil`, eliminates critical bugs (3+ second initialization, memory leaks, thread-unsafe global mutation), and achieves 90-97% performance improvement through client caching and Server-Side Apply.

The effort spans three new packages with clearly separated concerns: shared utilities (caching, config, errors), resource operations (Apply/Patch/Delete), and client creation (connection management, reachability). Together, they enable Hive controllers to efficiently manage hundreds of remote Kubernetes clusters.

---

## The Problem

### Current Architecture

```
Controllers
    ↓
    ├── pkg/resource (resource operations)
    │   ├── Wraps kubectl CLI libraries
    │   ├── Creates factory per operation (no caching)
    │   ├── Fetches OpenAPI schema (3+ seconds)
    │   └── Mutates os.Args for field manager
    │
    └── pkg/remoteclient (client creation)
        ├── Creates clients per reconciliation
        ├── Blocking discovery calls (no timeout)
        ├── No cache invalidation strategy
        └── Field manager "hive2-" prefix

Issues: Memory leaks, no caching, 3+ sec overhead, thread-unsafe, inconsistent field managers
```

### Critical Bugs Identified

**From pkg/resource:**
1. os.Args global mutation (thread-unsafe)
2. REST config mutation (memory leak)
3. Factory recreation per operation (no reuse)
4. OpenAPI schema fetch (3+ second overhead)
5. No context support (can't timeout/cancel)
6. Four field manager versions (hive4/5/6/7)
7. Deletion timing semantic bug

**From pkg/remoteclient:**
1. No client caching (recreate every reconciliation)
2. No context support (can't timeout/cancel)
3. REST config mutation potential
4. Blocking reachability checks
5. No cache invalidation strategy
6. Field manager "hive2-" inconsistency
7. Discovery client recreation

### Performance Impact (Current)

| Scenario | Time | Issue |
|----------|------|-------|
| First resource apply | 3700ms | Schema fetch (3s) + factory (500ms) + apply (200ms) |
| Subsequent applies | 3700ms | No caching, repeats everything |
| 1000 cluster sync | 62 min | 3.7s × 1000 clusters |
| Memory usage | Growing | Leaks from wrapper accumulation |

---

## The Solution: Three-Package Architecture

### Modernized Architecture

```
Controllers
    ↓
    ├── pkg/resource v2 (resource operations)
    │   ├── Server-Side Apply (native K8s)
    │   ├── Context-aware API
    │   ├── Clean operation semantics
    │   └── Uses shared infrastructure ↓
    │
    ├── pkg/remoteclient v2 (client creation)
    │   ├── Client caching
    │   ├── Context-aware API
    │   ├── Auto cache invalidation
    │   └── Uses shared infrastructure ↓
    │
    └── internal/clientutil (shared infrastructure)
        ├── Client cache (LRU + TTL)
        ├── REST config utilities (immutable)
        ├── Field manager naming (unified)
        ├── Error types (typed predicates)
        └── Metrics infrastructure

Benefits: No leaks, caching, <500ms ops, thread-safe, consistent field managers
```

### Package Separation of Concerns

| Package | Scope | Key Responsibilities |
|---------|-------|---------------------|
| **internal/clientutil** | Shared infrastructure | • Client cache (LRU, TTL, health checks)<br>• REST config utilities (immutable)<br>• Field manager naming `FieldManagerName()`<br>• Error types (ClusterError)<br>• Metrics (transport wrapper, cache, ops) |
| **pkg/resource v2** | Resource operations | • Apply (Server-Side Apply)<br>• Patch (all types: SSA, strategic, merge, JSON)<br>• Delete (clear state semantics)<br>• Context-first API<br>• Operation-specific errors |
| **pkg/remoteclient v2** | Client creation | • Build clients (controller-runtime, dynamic, typed)<br>• Kubeconfig loading from secrets<br>• API URL management (primary/secondary)<br>• Reachability checking<br>• Cache invalidation on cert rotation/failover |

---

## Shared Infrastructure (internal/clientutil)

### Components

**Client Cache:**
- LRU eviction (configurable max size, default 500)
- TTL expiration (configurable duration, default 10 minutes)
- Cache key: cluster ID + kubeconfig ResourceVersion + API URL
- Health checks (default 2-minute intervals)
- Thread-safe concurrent access
- Metrics: hits, misses, evictions, size

**REST Config Utilities:**
- `CopyConfigWithMetrics()` - Apply metrics wrapper immutably
- `PrepareConfigForClient()` - Apply URL/IP overrides immutably
- `ConfigEquals()` - Compare configs for cache keys
- Prevents wrapper accumulation bug
- Guarantees immutability

**Field Manager Naming:**
- `FieldManagerName(controllerName)` → `"hive-{controller}"`
- Single source of truth
- Deprecates hive1, hive2, hive4-7 prefixes
- Consistent across all packages

**Error Types:**
- `ClusterError` struct with cluster/operation/resource context
- Typed predicates: IsNotFound, IsTimeout, IsConnectionFailed, etc.
- Error wrapping with `errors.Is()` and `errors.As()` support

**Metrics Infrastructure:**
- Single transport wrapper implementation
- Cache metrics (hits, misses, evictions)
- Operation metrics (duration, count)
- Consistent labels across packages

### Benefits

- **Eliminates Duplication:** Single implementation of caching, config handling, metrics
- **Ensures Consistency:** Field manager naming, error types used by all packages
- **Performance:** Client reuse, lightweight reachability checks, no schema fetching
- **Reliability:** Thread-safe, no memory leaks, proper timeout handling

---

## Resource Helper v2

### Scope

Modernizes resource operations (Apply, Patch, Delete) for Kubernetes resources on remote clusters.

### Key Improvements

**Server-Side Apply Migration:**
- Eliminates kubectl CLI coupling
- No OpenAPI schema required (removes 3+ second overhead)
- Field ownership tracking server-side
- Automatic conflict resolution
- Uses controller-runtime `client.Patch()` with `client.Apply`

**Context-Aware API:**
- All operations accept `context.Context` first parameter
- Timeout and cancellation support
- Distributed tracing compatible
- Example: `Apply(ctx, object, WithFieldManager("name"))`

**Clean Operation Semantics:**
- **Apply:** Creates or updates, returns Created/Configured/Unchanged
- **Patch:** Multiple types (SSA, strategic, merge, JSON), returns Patched/Unchanged
- **Delete:** Clear states (Deleted, NotFound, DeletionInProgress), no semantic bugs

**Unified API:**
- Single Apply method (not Apply/ApplyRuntimeObject/CreateOrUpdate/Create)
- Accepts both []byte and runtime.Object
- Options pattern: WithFieldManager, WithForce, WithDryRun
- Consistent signatures across all operations

**Integration with Shared Infrastructure:**
- Uses ClientCache for client reuse
- Uses FieldManagerName for consistent naming
- Uses ClusterError for typed errors
- Uses metrics infrastructure for instrumentation

---

## Remote Client v2

### Scope

Manages Kubernetes client creation and connection lifecycle for remote clusters.

### Key Improvements

**Client Caching:**
- Cache key includes kubeconfig ResourceVersion + API URL
- LRU eviction with configurable size (default 500)
- TTL expiration (default 10 minutes)
- Automatic invalidation on certificate rotation (ResourceVersion change)
- Automatic invalidation on failover (API URL change)

**Context-Aware API:**
- All Build methods accept context
- Timeout control for reachability checks
- Cancellation support
- Example: `BuildWithContext(ctx, WithCache(cache))`

**Automatic Cache Invalidation:**
- Certificate rotation triggers ResourceVersion change → cache miss
- API URL failover triggers URL change → cache miss
- TTL prevents holding stale credentials
- Health checks detect unreachable clusters → eviction

**API URL Failover Support:**
- Primary URL: APIURLOverride or kubeconfig URL
- Secondary URL: Alternate of above
- Builder pattern: `builder.UsePrimaryAPIURL()` / `UseSecondaryURL()`
- Cache key includes URL for automatic invalidation

**Integration with Shared Infrastructure:**
- Uses ClientCache for caching
- Uses REST config utilities for immutability
- Uses FieldManagerName for consistency
- Uses ClusterError for typed errors
- Uses metrics for instrumentation

---

## Performance Impact

### Current State (v1)

| Operation | Time | Root Cause |
|-----------|------|------------|
| Helper initialization | 3000ms | OpenAPI schema fetch |
| Single Apply | 700ms | Factory + discovery + apply |
| Total per apply | 3700ms | Init + operation |
| 1000 clusters (first pass) | 62 min | 3.7s × 1000 |
| 1000 clusters (steady state) | 62 min | No caching, same as first |
| Memory | Growing | Wrapper accumulation leak |

### Modernized State (v2)

| Operation | Time | Improvement |
|-----------|------|-------------|
| Helper initialization | <100ms | No schema needed (SSA) |
| Single Apply (cached) | 100ms | Server-side logic only |
| Single Apply (uncached) | 300ms | Client creation + apply |
| Total cached | 110ms | **97% faster** |
| Total uncached | 300ms | **92% faster** |
| 1000 clusters (first pass) | 5 min | 300ms × 1000 |
| 1000 clusters (steady state) | 25 sec | 95% cache hit: 0.95×10ms + 0.05×300ms |
| Memory | Stable | Bounded cache, no leaks |

### Performance Summary

| Scenario | Current | Modernized | Improvement |
|----------|---------|------------|-------------|
| Single cluster (cached) | 3700ms | 110ms | **97%** |
| Single cluster (uncached) | 3700ms | 300ms | **92%** |
| 1000 clusters (first) | 62 min | 5 min | **92%** |
| 1000 clusters (steady) | 62 min | 25 sec | **99.3%** |
| Memory footprint | Unbounded | Bounded (500 clients) | Stable |

---

## Migration Strategy

### Phase 1: Create Shared Infrastructure (1-2 weeks)

**Goal:** Establish foundation without breaking existing code.

**Tasks:**
1. Create `internal/clientutil/` package structure
2. Implement client cache (LRU + TTL + health checks)
3. Implement REST config utilities
4. Implement error types and field manager naming
5. Comprehensive unit tests
6. Integration tests with real cache

**Risk:** Low (additive only, no breaking changes)

### Phase 2: Modernize remoteclient (2-3 weeks)

**Goal:** Add caching and context support to client creation.

**Tasks:**
1. Implement context-aware Builder interface
2. Integrate with shared client cache
3. Implement cache key with ResourceVersion + API URL
4. Add functional options constructor
5. Maintain backward compatibility
6. Update tests and documentation

**Risk:** Medium (interface changes, backward compatible)

### Phase 3: Migrate Controllers to Cached remoteclient (1 week per controller)

**Goal:** Gain performance benefits in production.

**Priority Order:**
1. clustersync (highest impact - manages many clusters)
2. clusterstate (high frequency reconciliation)
3. hibernation, clusterversion (medium frequency)
4. Others (lower priority)

**Tasks per Controller:**
1. Create shared cache in controller setup
2. Update builder usage to context-aware API
3. Add context propagation through reconciliation
4. Measure performance improvements
5. Update unit and integration tests

**Risk:** Low (backward compatible, easy rollback)

### Phase 4: Implement Resource Helper v2 (3-4 weeks)

**Goal:** Modernize resource operations with Server-Side Apply.

**Tasks:**
1. Implement v2 API with context support
2. Use Server-Side Apply instead of kubectl
3. Integrate with shared cache
4. Use shared REST config utilities
5. Comprehensive unit tests
6. Integration tests with envtest
7. Migration guide and documentation

**Risk:** High (major rewrite, extensive testing needed)

### Phase 5: Migrate Controllers to Resource Helper v2 (1-2 weeks per controller)

**Goal:** Complete modernization.

**Priority Order:**
1. controlplanecerts (simplest - only ApplyRuntimeObject)
2. remoteingress (simple - single Apply)
3. clustersync (complex - multiple operations)
4. operator/hive (currently recreates helper per reconcile)

**Tasks per Controller:**
1. Update to v2 API
2. Replace v1 Helper with v2
3. Update operation calls (Add context, update signatures)
4. Update tests
5. Verify performance improvements

**Risk:** Medium (breaking API changes, careful testing required)

### Phase 6: Deprecate and Remove v1 (Gradual over 6 months)

**Goal:** Clean up technical debt.

**Tasks:**
1. Mark v1 APIs as deprecated
2. Monitor for remaining v1 usage
3. Remove v1 after 2-3 releases
4. Clean up transitional code
5. Update all documentation

**Risk:** Low (plenty of migration time)

---

## Success Criteria

### Performance

- ✓ Client creation (cached): <10ms (p99)
- ✓ Client creation (uncached): <500ms (p99)
- ✓ Apply operation (cached): <100ms (p99)
- ✓ Cache hit rate: >95% (multi-cluster)
- ✓ Memory growth: 0% over 24 hours

### Reliability

- ✓ Zero concurrency bugs (race detector clean)
- ✓ Zero memory leaks (stable profile)
- ✓ 100% error wrapping with context
- ✓ Thread-safe concurrent operations
- ✓ Proper context cancellation

### Code Quality

- ✓ Test coverage >80%
- ✓ Complete API documentation
- ✓ Migration guides
- ✓ All operations instrumented
- ✓ Working examples

---

## Next Steps

### For Implementers

1. **Read Specifications:** Start with `SHARED_CLIENT_UTILITIES_SPECIFICATION.md`
2. **Understand Scope:** Each spec is independent but references shared infrastructure
3. **No Example Code:** Specs define requirements and constraints, not implementation
4. **Reference Existing Code:** Understand bugs by reading original pkg/resource and pkg/remoteclient
5. **Test Driven:** Write tests first based on testing requirements in specs

### For Reviewers

1. **Verify Completeness:** All original bugs addressed
2. **Check Separation:** No overlap between specs
3. **Validate Performance:** Estimates match requirements
4. **Review Integration:** Specs reference each other correctly
5. **Confirm Migration:** Path from v1 to v2 is clear

### For Project Managers

**Timeline:** 9-14 weeks total
- Phase 1 (Shared Infrastructure): 2 weeks
- Phase 2 (remoteclient v2): 3 weeks
- Phase 3 (Controller migration - remoteclient): 3 weeks
- Phase 4 (Resource Helper v2): 4 weeks
- Phase 5 (Controller migration - resource): 4 weeks
- Phase 6 (Deprecation): Gradual over 6 months

**Resource Requirements:**
- 1-2 engineers for implementation
- 1 engineer for testing/QA
- Access to development and staging clusters
- Monitoring infrastructure (Prometheus/Grafana)

**Risk Mitigation:**
- Phase 1-3 have low risk (backward compatible)
- Phase 4-5 require careful testing
- Rollback strategy at each phase
- Metrics comparison before/after

---

Last updated: 2026-03-11
