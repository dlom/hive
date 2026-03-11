# Resource Helper v2 Package - Implementation Specification

## Purpose

This document specifies the requirements for a modernized resource helper package for OpenShift Hive. It describes the critical bugs and architectural flaws in `pkg/resource` that necessitate a complete rewrite, and defines requirements for a new implementation using Server-Side Apply and native Kubernetes client libraries.

This specification is intended for:
- LLM code generation tools creating a new implementation from scratch
- Human developers implementing a replacement package
- Technical reviewers evaluating proposed solutions

## Background & Scope

The Hive resource helper package (`pkg/resource`) provides programmatic access to Kubernetes resource operations (Apply, Patch, Delete) for Hive controllers and operators. The package wraps kubectl's CLI-oriented functionality to enable controllers to manage resources on remote clusters.

The current implementation was written years ago and has accumulated critical technical debt including thread-unsafe global state mutation, heavy coupling to kubectl CLI libraries, and no context support. Performance issues include 3+ second initialization overhead and repeated client creation.

**This specification focuses exclusively on resource operation requirements** (Apply, Patch, Delete semantics and API design). Infrastructure concerns (client caching, REST config handling, discovery management, field manager naming, error types, metrics) are addressed in the Shared Client Utilities Specification.

## Dependencies

### Shared Utilities Specification

The resource helper v2 depends on shared infrastructure components defined in `SHARED_CLIENT_UTILITIES_SPECIFICATION.md`:
- **Client Caching:** Per-cluster client caching with LRU eviction and TTL
- **REST Config Utilities:** Immutable config handling and defensive copying
- **Discovery Management:** Discovery client lifecycle and caching (if needed for backward compatibility)
- **Field Manager Naming:** Consistent field manager naming scheme via `FieldManagerName()`
- **Error Types:** Typed error definitions (AlreadyExists, NotFound, Conflict, etc.)
- **Metrics Infrastructure:** Operation instrumentation and cache metrics

### Server-Side Apply Migration

v2 migrates from kubectl's client-side apply to Kubernetes native Server-Side Apply (SSA), eliminating OpenAPI schema dependency (3+ second overhead), client-side three-way merge complexity, and kubectl library coupling. SSA is available since Kubernetes 1.16 and stable since 1.22.

---

## Critical Requirements from v1 Analysis

### Thread-Unsafe Global State Mutation

**Location:** `pkg/resource/patch.go`, lines 56-58

```go
func (r *helper) setupPatchCommand(name, kind, apiVersion, patchType string, f cmdutil.Factory, patch string, ioStreams genericclioptions.IOStreams) (*kcmdpatch.PatchOptions, error) {
    // This is bizarre and I don't know why it works, but it's the only way I could figure out
    // to set the `manager` properly. HIVE-1744.
    os.Args = []string{
        "hive7-" + string(r.controllerName),
    }
    cmd := kcmdpatch.NewCmdPatch(f, ioStreams)
    cmd.Flags().Parse([]string{})
```

**Problem:** Direct mutation of global `os.Args` array. Non-reentrant and thread-unsafe. Concurrent calls to Patch() will race, causing field manager corruption. The comment acknowledges this is a hack with no clear understanding.

**Requirement:** v2 MUST NOT mutate any global state. Field manager names must be configurable via parameters, not environment manipulation. Use native Kubernetes client APIs that accept field manager as a parameter.

### No Context Support

**Location:** `pkg/resource/helper.go`, lines 24-38

```go
type Helper interface {
    Apply(obj []byte) (ApplyResult, error)
    ApplyRuntimeObject(obj runtime.Object, scheme *runtime.Scheme) (ApplyResult, error)
    CreateOrUpdate(obj []byte) (ApplyResult, error)
    CreateOrUpdateRuntimeObject(obj runtime.Object, scheme *runtime.Scheme) (ApplyResult, error)
    Create(obj []byte) (ApplyResult, error)
    CreateRuntimeObject(obj runtime.Object, scheme *runtime.Scheme) (ApplyResult, error)
    Info(obj []byte) (*Info, error)
    Patch(name types.NamespacedName, kind, apiVersion string, patch []byte, patchType string) error
    Delete(apiVersion, kind, namespace, name string) error
}
```

**In implementation files:**
```go
// pkg/resource/apply.go:157
_, err = c.Resource(gvr).Namespace(info.Namespace).Create(context.TODO(), ...)

// pkg/resource/delete.go:21, 35, 60
switch err := c.Get(context.Background(), key, obj); {
if err := c.Delete(context.Background(), obj); err != nil {
```

**Problem:** No `context.Context` parameter on any interface method. All operations use `context.TODO()` or `context.Background()`. Impossible for callers to implement timeouts, cancellation, or tracing. Operations will hang indefinitely if cluster becomes unresponsive.

**Requirement:** v2 MUST accept `context.Context` as the first parameter of all operations. This is standard practice in modern Go code and Kubernetes client libraries. Respect context cancellation and timeouts. Set reasonable default timeouts if context has no deadline.

### Deletion Timing Semantic Bug

**Location:** `pkg/resource/delete.go`, lines 29-32

```go
if obj.GetDeletionTimestamp() != nil {
    logger.Debug("object has already been deleted")
    // BUT the object still exists!
    return false, nil
}
```

**Problem:** Comment explicitly notes "BUT the object still exists!" Function signature says first return value is "true iff the object was already gone". Returns `false` when object is in deletion state, which is semantically wrong. Callers cannot distinguish between "deleted" and "in deletion grace period".

**Requirement:** v2 MUST have clear semantics for deletion states. Distinguish between "not found", "deletion in progress" (has deletionTimestamp but still exists), and "successfully deleted". Return structured DeleteResult type, not boolean.

---

## Architectural Flaws Requiring Modernization

### Heavy Coupling to kubectl CLI Library

**Location:** `pkg/resource/apply.go`, lines 222-276

The package uses kubectl's `ApplyOptions` and `PatchOptions` directly:

```go
func (r *helper) setupApplyCommand(f cmdutil.Factory, obj []byte, ioStreams genericclioptions.IOStreams) (*kcmdapply.ApplyOptions, *changeTracker, error) {
    r.logger.Debug("setting up apply command")
    flags := kcmdapply.NewApplyFlags(ioStreams)
    o := &kcmdapply.ApplyOptions{
        IOStreams:         ioStreams,
        VisitedUids:       sets.Set[types.UID]{},
        VisitedNamespaces: sets.Set[string]{},
        Recorder:          genericclioptions.NoopRecorder{},
        PrintFlags:        flags.PrintFlags,
        Overwrite:         true,
        OpenAPIPatch:      true,
        FieldManager:      "hive6-" + string(r.controllerName),
    }
    // ... 20+ more lines of setup
}
```

**Problem:** kubectl library is designed for CLI tools, not programmatic use. Tightly couples to kubectl implementation details. Complex setup with many fields to configure manually. kubectl may change internal APIs between versions.

**Requirement:** v2 should use native Kubernetes client libraries (controller-runtime, client-go) instead of kubectl wrappers. Prefer Server-Side Apply APIs directly from the Kubernetes API server. Use controller-runtime's `client.Patch()` method with `client.Apply` patch type.

### Confusing API Surface with Overlapping Methods

**Location:** `pkg/resource/helper.go`, lines 24-38

The Helper interface has 9 methods with significant overlap:
- `Apply()` vs `ApplyRuntimeObject()`
- `CreateOrUpdate()` vs `CreateOrUpdateRuntimeObject()`
- `Create()` vs `CreateRuntimeObject()`

**Problem:** Duplication violates DRY principle. Mix of byte arrays and runtime.Object is confusing. No batch operations. Patch method signature is inconsistent (no runtime.Object variant). Info method returns custom type instead of standard Kubernetes types.

**Requirement:** v2 should have a cleaner API with unified methods that work with both bytes and runtime.Object. Consistent signatures across all operations. Support for batch operations if needed. Use standard Kubernetes types where possible.

---

## API Design

### Core Interface

v2 MUST provide a clean, context-aware interface:

**Context-First Design:**
- All operations accept `context.Context` as first parameter
- Respect context deadlines, cancellation, and tracing
- Set reasonable defaults (e.g., 30-second timeout) if no deadline

**Unified Input Handling:**
- Accept both `[]byte` (YAML/JSON) and `runtime.Object` without separate methods
- Single Apply method instead of Apply/ApplyRuntimeObject/CreateOrUpdate/CreateOrUpdateRuntimeObject/Create/CreateRuntimeObject
- Use type switching or helper functions internally to handle different input types

**Structured Return Values:**
- Return structured result types, not strings or booleans
- ApplyResult indicates: Created, Configured, Unchanged
- DeleteResult indicates: Deleted, NotFound, DeletionInProgress
- PatchResult indicates: Patched, Unchanged

### Apply Operations

**Server-Side Apply as Default:**

Use Kubernetes Server-Side Apply via controller-runtime:
- `client.Patch(ctx, object, client.Apply, &client.PatchOptions{FieldManager: name, Force: &forceFlag})`
- No OpenAPI schema required (eliminates 3+ second overhead)
- Field-level ownership tracking built into API server
- Automatic conflict resolution

**Force Option:**

Support force flag for taking ownership of fields owned by other field managers. Default to false (fails on conflicts). Explicit opt-in required for force.

**Field Manager Configuration:**

Use shared utilities `FieldManagerName()` function for consistent naming. Allow per-operation override via options pattern if needed. Default format: `"hive-{controllername}"`.

**Apply Semantics:**
- Creates resource if it doesn't exist
- Updates resource if it exists (respecting field ownership)
- Returns result indicating Created, Configured (updated), or Unchanged
- Handles conflicts based on force flag

### Patch Operations

**Supported Patch Types:**

Support all Kubernetes patch types:
- Server-Side Apply (primary, recommended)
- Strategic Merge Patch
- JSON Merge Patch
- JSON Patch (RFC 6902)

**No Global State Mutation:**

Field manager must be passed as parameter, never via `os.Args` mutation. Use native client APIs that accept field manager in PatchOptions.

**Type-Safe Patch Type Constants:**

Use standard Kubernetes patch type constants from `types` package:
- `types.ApplyPatchType`
- `types.StrategicMergePatchType`
- `types.MergePatchType`
- `types.JSONPatchType`

**Patch Semantics:**
- Applies partial update to existing resource
- Fails if resource doesn't exist (unlike Apply which creates)
- Returns result indicating Patched or Unchanged
- Includes GVK, namespace, name in error messages

### Delete Operations

**Clear Deletion States:**

Return structured DeleteResult with clear states:
- **Deleted**: Resource successfully removed or already gone
- **NotFound**: Resource never existed
- **DeletionInProgress**: Resource has deletionTimestamp but still exists (finalizers or grace period)

This addresses the semantic bug in v1 where DeletionInProgress was incorrectly treated as "not deleted".

**Graceful Deletion:**

Support grace period and propagation policy options:
- GracePeriodSeconds - how long to wait before forcing deletion
- PropagationPolicy - how to handle dependent objects (Orphan, Background, Foreground)
- Configurable via options pattern

**Wait for Deletion:**

Optional wait flag to poll until resource fully removed:
- Useful when caller needs resource gone before proceeding
- Timeout controlled by context
- Returns DeletionInProgress if timeout expires

### Options Pattern

Use functional options for clean, extensible configuration:

**Apply Options:**
- `WithFieldManager(name string)` - Override default field manager
- `WithForce()` - Take ownership of fields from other managers
- `WithDryRun()` - Validate without persisting changes

**Patch Options:**
- `WithFieldManager(name string)` - Override default field manager
- `WithPatchType(patchType types.PatchType)` - Specify patch type

**Delete Options:**
- `WithGracePeriod(seconds int64)` - Set grace period
- `WithPropagationPolicy(policy metav1.DeletionPropagation)` - Set propagation policy
- `WithWait()` - Wait for deletion to complete

Options allow future extensibility without breaking existing callers.

---

## Migration from kubectl to Native APIs

### Why kubectl Coupling is Problematic

kubectl library (`k8s.io/kubectl/pkg/cmd/*`) is designed for CLI tools:
- Expects command-line flags, IOStreams, and flag parsing
- Performs client-side three-way merge (requires OpenAPI schema)
- Changes internal APIs between versions (not a stable library)
- Requires complex setup (20+ lines to configure ApplyOptions)
- Forces hacks like `os.Args` mutation to set field manager

### Native Kubernetes Client Usage

Use controller-runtime and client-go instead:

**controller-runtime/pkg/client:**
- `client.Client` interface with Patch method for Server-Side Apply
- `client.Apply` patch type for SSA
- Native support for runtime.Object
- Clean API designed for programmatic use

**client-go/dynamic:**
- `dynamic.Interface` for unstructured resource operations
- Resource-based operations without type information
- Works with raw YAML/JSON

**client-go/rest:**
- `rest.CopyConfig()` for defensive copying (see Shared Utilities Spec)
- Standard Kubernetes authentication

### Server-Side Apply Implementation

**Key Advantage:** Server-Side Apply is implemented in kube-apiserver, not client:
- No client-side diffing or three-way merge
- No OpenAPI schema fetching
- Field manager tracked server-side
- Conflicts resolved server-side

**Implementation Pattern:**

Use controller-runtime client with Apply patch type. Parse YAML/JSON to unstructured or typed object. Call Patch with client.Apply. API server handles merge and ownership tracking.

**Backward Compatibility:**

For controllers that need client-side apply semantics (e.g., dry-run without server support), provide fallback using kubectl strategicpatch.StrategicMergePatch. Make this opt-in, not default.

---

## Operation Semantics

### Apply Behavior

**Creation:**
- If resource doesn't exist, create it
- Set field manager to configured value
- Return ApplyResult.Created

**Update:**
- If resource exists, update fields owned by this field manager
- If field owned by different manager, behavior depends on force flag:
  - force=false: Return conflict error
  - force=true: Take ownership of field
- Return ApplyResult.Configured (updated) or ApplyResult.Unchanged (no changes needed)

**Idempotency:**
- Applying same resource multiple times has same effect as applying once
- Second apply returns Unchanged if resource already in desired state
- Safe to retry on transient failures

### Patch Types and Semantics

**Server-Side Apply:**
- Primary patch type for v2
- Recommended for all new code
- Handles conflicts via field ownership tracking

**Strategic Merge Patch:**
- Legacy Kubernetes patch type
- Understands Kubernetes API semantics (merge vs replace for lists)
- Requires OpenAPI schema for some operations
- Supported for backward compatibility

**JSON Merge Patch (RFC 7386):**
- Simple merge: fields in patch replace fields in object
- `null` values delete fields
- No list merging (entire list replaced)

**JSON Patch (RFC 6902):**
- Array of operations: add, remove, replace, move, copy, test
- Precise control over changes
- More complex to construct

### Delete States and Return Values

**DeleteResult Structure:**

Return structured type with:
- State: Deleted, NotFound, or DeletionInProgress
- DeletionTimestamp: When deletion was requested (if DeletionInProgress)
- Object: Final state of object (if retrieved)

**State Semantics:**

**Deleted:**
- Resource successfully deleted and no longer exists, OR
- Resource was already gone (idempotent behavior)
- Safe to proceed assuming resource is gone

**NotFound:**
- Resource never existed
- Different from Deleted: indicates delete was unnecessary
- Allows caller to distinguish "deleted what was there" from "nothing to delete"

**DeletionInProgress:**
- Resource has deletionTimestamp set but still exists
- Finalizers or grace period preventing immediate deletion
- Contains deletionTimestamp so caller can track how long deletion has been pending
- If WithWait() option used, will poll until Deleted or context timeout

**Error vs Status:**

Return errors for unexpected conditions (network failure, auth failure, timeout). Return DeleteResult.NotFound as success case (idempotent delete succeeded). Return DeleteResult.DeletionInProgress as success if not waiting.

---

## Infrastructure Integration

### Client Caching

v2 MUST support client caching via shared utilities. See Shared Client Utilities Specification for:
- Cache interface and implementation
- LRU eviction policy
- TTL-based expiration
- Thread-safety guarantees

v2 integrates by:
- Accepting optional cache via options: `WithCache(cache ClientCache)`
- Using cache to retrieve/store clients by cluster identifier
- Allowing cache-less operation for simple use cases

### REST Config Handling

v2 MUST use immutable REST config utilities from shared spec. Never mutate configs in place. See Shared Client Utilities Specification for:
- `CopyConfigWithMetrics()` function
- Defensive copying patterns
- Wrapper accumulation prevention

### Field Manager Naming

v2 MUST use `FieldManagerName()` from shared utilities spec. No hardcoded prefixes like "hive4", "hive5", "hive6", "hive7". Consistent naming across all operations (Apply, Patch).

### Error Handling

v2 MUST use typed errors from shared utilities spec. Wrap errors with operation context using `WrapClusterError()`. Support error predicates: `IsNotFound()`, `IsAlreadyExists()`, `IsConflict()`, etc.

### Metrics

v2 MUST instrument operations using metrics from shared utilities spec:
- Operation duration histogram by operation type (apply, patch, delete) and GVK
- Operation count by result (success, failure, conflict, timeout)
- Use consistent labels (controller, operation, gvk, result)

See Shared Client Utilities Specification for metric definitions and registration.

---

## Testing Requirements

### Unit Tests

**Apply Operation Tests:**
- Create: Apply non-existent resource returns Created
- Update: Apply existing resource returns Configured
- Unchanged: Apply resource already in desired state returns Unchanged
- Conflict: Apply with field owned by different manager returns error (force=false)
- Force: Apply with force=true takes ownership of conflicting fields
- Context cancellation: Cancel during apply returns context error
- Context timeout: Timeout during apply returns deadline exceeded

**Patch Operation Tests:**
- Different patch types (SSA, strategic, JSON merge, JSON patch)
- Patch non-existent resource returns NotFound error
- Patch existing resource returns Patched
- Unchanged: Patch with no changes returns Unchanged
- Field manager consistency across patch types

**Delete Operation Tests:**
- Delete existing resource returns Deleted
- Delete non-existent resource returns NotFound (idempotent)
- Delete resource with finalizers returns DeletionInProgress
- Wait for deletion polls until resource gone
- Wait timeout returns DeletionInProgress with timeout error
- Grace period and propagation policy respected

**Input Format Tests:**
- Handle YAML byte arrays
- Handle JSON byte arrays
- Handle runtime.Object (typed)
- Handle runtime.Object (unstructured)
- Invalid YAML/JSON returns parse error

**Context Handling Tests:**
- Context with deadline respected
- Context cancellation interrupts operation
- Context without deadline uses default timeout

**Error Handling Tests:**
- Network errors wrapped with operation context
- Auth errors include cluster identifier
- Validation errors include GVK and field path
- All errors support errors.Is() and errors.As()

### Integration Tests

Use envtest or kind for integration testing with real Kubernetes API server:

**Server-Side Apply Integration:**
- Verify field ownership tracked correctly
- Verify conflicts handled as expected
- Verify force flag takes ownership
- Test with multiple field managers (simulate different controllers)

**Large Resource Handling:**
- Test with resources >1MB
- Verify no truncation or corruption
- Performance acceptable for large resources

**Concurrent Operations:**
- 100+ goroutines using helper simultaneously
- All operations complete successfully
- No race conditions (run with `-race` flag)
- Cache prevents duplicate client creation

### Backward Compatibility Tests

Test migration from v1 to v2:
- Resources created by v1 can be updated by v2
- Field managers from v1 (hive4/5/6/7) don't conflict with v2 (hive-{controller})
- Controllers can switch from v1 to v2 without disruption

### Thread-Safety Tests

Run all tests with `go test -race ./...` to detect data races. Specific scenarios:
- Concurrent Apply operations to different resources
- Concurrent Patch operations to same resource (should serialize server-side)
- Concurrent Delete operations
- Concurrent cache access

---

## Migration Guide

### Breaking Changes from Original API

v2 has intentional breaking changes from v1:

1. **Context parameter:** All methods now require `context.Context` as first parameter
2. **Return types:** Structured result types (ApplyResult, PatchResult, DeleteResult) replace strings and booleans
3. **Method consolidation:** Single Apply method replaces Apply/ApplyRuntimeObject/CreateOrUpdate/CreateOrUpdateRuntimeObject/Create/CreateRuntimeObject
4. **Error types:** Typed errors from shared utilities replace generic errors
5. **Field manager naming:** Unified "hive-{controller}" format replaces versioned prefixes

### Compatibility Strategy

**Adapter Layer:**

Create adapter implementing v1 Helper interface using v2 client:
- Delegates to v2 methods
- Translates return types (ApplyResult to string, DeleteResult to boolean)
- Provides context.Background() for methods without context
- Allows gradual migration

**Gradual Migration:**

Migrate controllers one at a time:
1. Update controller to use v2 API
2. Test thoroughly in development
3. Deploy to staging
4. Monitor metrics and errors
5. Roll out to production
6. Move to next controller

**Parallel Operation:**

Run v1 and v2 implementations side-by-side during transition:
- Some controllers use v1
- Some controllers use v2
- Both share same metrics namespace
- Compare performance and behavior

**Metrics Comparison:**

Use Prometheus queries to compare v1 vs v2:
- Operation latency (v2 should be faster)
- Error rates (should be similar)
- Cache hit rates (v2 should be higher with caching enabled)

### Controllers to Migrate (in order of complexity)

1. **Simple:** `pkg/controller/controlplanecerts/` - Only uses ApplyRuntimeObject, single operation type
2. **Medium:** `pkg/controller/remoteingress/` - Single Apply call per reconciliation
3. **Complex:** `pkg/controller/clustersync/` - Heavy user with multiple operations (Apply, CreateOrUpdate, Create, Patch)
4. **Operator:** `pkg/operator/hive/` - Currently recreates helper per reconcile (will benefit most from caching)

---

## Success Criteria

### Reliability Metrics

- Zero concurrency bugs (race detector clean)
- Zero memory leaks (stable memory profile over 24 hours)
- 100% error handling (all errors wrapped with operation context)
- Thread-safe for concurrent use by multiple goroutines
- Proper context cancellation handling

### Code Quality Metrics

- Test coverage >80%
- Complete API documentation with examples
- Migration guide from v1 to v2
- Performance comparison data (v1 vs v2)

### Functional Completeness

- All v1 operations supported (Apply, Patch, Delete)
- Server-Side Apply working correctly
- Field ownership tracking correct
- Conflict resolution as expected
- Batch operations if needed by controllers

### Performance Targets

Performance targets depend on client caching from shared utilities:
- Operation latency with cached client: <100ms (p99)
- Operation latency with uncached client: <500ms (p99)
- Memory usage: Stable over time (no leaks)

See Shared Client Utilities Specification for cache performance requirements.

---

## Appendix: Original Implementation Reference

### Original Implementation Files (DO NOT USE AS REFERENCE FOR IMPLEMENTATION)

These files contain the problems described in this document:

- `pkg/resource/helper.go` - Main interface and implementation
- `pkg/resource/apply.go` - Apply operations with kubectl wrapper
- `pkg/resource/patch.go` - Patch operations with os.Args hack
- `pkg/resource/delete.go` - Delete operations with timing bug
- `pkg/resource/info.go` - Info extraction
- `pkg/resource/restconfig_factory.go` - REST config factory with mutation bug
- `pkg/resource/kubeconfig_factory.go` - Kubeconfig factory
- `pkg/resource/factory_discovery.go` - Discovery client with disk cache
- `pkg/resource/fake.go` - Incomplete fake implementation

### Controller Usage Examples (FOR UNDERSTANDING USAGE PATTERNS ONLY)

- `pkg/controller/clustersync/clustersync_controller.go` - Heavy user with caching patterns
- `pkg/controller/remoteingress/remoteingress_controller.go` - Simple usage example
- `pkg/controller/controlplanecerts/controlplanecerts_controller.go` - Simple usage example
- `pkg/operator/hive/hive_controller.go` - Per-reconcile recreation (antipattern to avoid)

### Related Specifications

- `SHARED_CLIENT_UTILITIES_SPECIFICATION.md` - Infrastructure components (caching, config, discovery, errors, metrics)
- `REMOTECLIENT_V2_SPECIFICATION.md` - Client creation and connection management (complementary package)

---

## Document Maintenance

This specification should be updated when:
- New resource operation requirements emerge from controller usage
- Kubernetes API evolves (new Server-Side Apply features)
- Migration from v1 reveals additional requirements
- Integration patterns with shared utilities change

---

Last updated: 2026-03-11
