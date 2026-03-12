// Package remoteclient provides builders for creating Kubernetes clients to remote clusters.
//
// This package is the primary API for Hive controllers to connect to managed clusters.
// It handles kubeconfig loading, client caching, API URL failover, and automatic cache
// invalidation on certificate rotation.
//
// # Basic Usage
//
// Create a builder with functional options and build a client:
//
//	cache := clientutil.GetSharedCache(hivev1.ClustersyncControllerName)
//	builder := remoteclient.NewBuilderWithOptions(
//	    remoteclient.WithClusterDeployment(c, cd),
//	    remoteclient.WithControllerName(hivev1.ClustersyncControllerName),
//	    remoteclient.WithCache(cache),
//	)
//
//	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
//	defer cancel()
//
//	remoteClient, err := builder.BuildWithContext(ctx)
//
// # Client Caching
//
// Client caching provides 90-97% performance improvement by reusing clients across
// reconciliations. The cache automatically invalidates entries when:
//   - Kubeconfig secret ResourceVersion changes (certificate rotation)
//   - API URL changes (failover between primary/secondary)
//   - TTL expires (default: 10 minutes, configurable)
//
// Enable caching by passing a cache via WithCache():
//
//	cache := clientutil.GetSharedCache(controllerName)
//	builder := remoteclient.NewBuilderWithOptions(
//	    remoteclient.WithCache(cache),
//	    // ... other options
//	)
//
// # API URL Failover
//
// The builder supports API URL failover for high availability. Controllers can switch
// between primary and secondary URLs:
//
//	// Try primary URL first
//	builder := builder.UsePrimaryAPIURL()
//	client, err := builder.BuildWithContext(ctx)
//	if err != nil {
//	    // Fall back to secondary URL
//	    builder = builder.UseSecondaryAPIURL()
//	    client, err = builder.BuildWithContext(ctx)
//	}
//
// By default, builders use the "active" URL as determined by the
// ActiveAPIURLOverrideCondition on the ClusterDeployment.
//
// The builder can be configured with either a ClusterDeployment (for full integration
// with API URL overrides and failover) or a raw kubeconfig secret (for simpler cases).
//
// # Context Support
//
// All build methods accept context.Context for timeout and cancellation:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
//	defer cancel()
//
//	client, err := builder.BuildWithContext(ctx)
//
// Context timeouts are respected during:
//   - Kubeconfig loading from secrets
//   - REST client creation
//   - Reachability verification
package remoteclient
