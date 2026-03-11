# Hive Client Infrastructure Modernization Specifications

This directory contains specifications for modernizing Hive's remote cluster client infrastructure, addressing critical bugs and performance issues in `pkg/resource` and `pkg/remoteclient`.

## Documents

### [OVERVIEW.md](OVERVIEW.md)
High-level summary of the modernization effort with architecture diagrams, performance comparisons, and migration strategy. **Start here** for context.

- Package relationships and dependency graph
- Performance impact analysis (90-97% improvement)
- Migration strategy (6 phases over 9-14 weeks)
- Success criteria

### [SHARED_CLIENT_UTILITIES_SPECIFICATION.md](SHARED_CLIENT_UTILITIES_SPECIFICATION.md)
Specification for `internal/clientutil` package providing shared infrastructure used by both resource helper v2 and remoteclient v2.

**Components:**
- Client Cache (LRU + TTL + health checks)
- REST Config Utilities (immutable operations)
- Discovery Client Management (in-memory caching)
- Field Manager Naming (unified `FieldManagerName()`)
- Error Types (ClusterError with typed predicates)
- Metrics Infrastructure (transport wrapper, cache, operations)

### [RESOURCE_HELPER_V2_SPECIFICATION.md](RESOURCE_HELPER_V2_SPECIFICATION.md)
Specification for modernized resource operations package (Apply, Patch, Delete) using Server-Side Apply and native Kubernetes client libraries.

**Key Features:**
- Server-Side Apply (eliminates 3+ second OpenAPI schema overhead)
- Context-aware API (timeout, cancellation, tracing)
- Clean operation semantics (no deletion timing bugs)
- Native Kubernetes clients (removes kubectl coupling)
- Integration with shared utilities

### [REMOTECLIENT_V2_SPECIFICATION.md](REMOTECLIENT_V2_SPECIFICATION.md)
Specification for modernized client creation and connection management package.

**Key Features:**
- Client caching (LRU + TTL)
- Automatic cache invalidation (cert rotation, URL failover)
- Context-aware API (timeout control)
- Reachability management
- API URL failover support
- Integration with shared utilities

## Specification Philosophy

All specifications follow these principles (from original RESOURCE_HELPER_SPECIFICATION.md):

1. **Concise** - Brevity is the soul of a good spec
2. **Descriptive** - Clear problem statements with code references to existing implementations
3. **Guiding** - Requirements and guidance, not prescriptive implementation
4. **Code-Free** - No example new implementation code (can reference existing buggy code)

## Reading Order

1. **OVERVIEW.md** - Understand the big picture
2. **SHARED_CLIENT_UTILITIES_SPECIFICATION.md** - Foundation for other packages
3. **RESOURCE_HELPER_V2_SPECIFICATION.md** OR **REMOTECLIENT_V2_SPECIFICATION.md** - Based on your focus area

Each specification is self-contained but references shared infrastructure where appropriate to avoid duplication.

## Implementation Guidance

### For Package Implementers

- Read relevant specification thoroughly
- Understand scope (what's in this package vs shared utilities)
- Reference existing code to understand bugs (file paths provided)
- Follow testing requirements
- No example implementation code means multiple implementations can be compared

### For Reviewers

- Verify no overlap between specifications
- Check all cross-references are valid
- Confirm all original bugs addressed
- Validate migration path is clear

---

Last updated: 2026-03-11
