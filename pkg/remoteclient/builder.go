package remoteclient

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/internal/clientutil"
	utilscheme "github.com/openshift/hive/pkg/util/scheme"
)

// ============================================================================
// Builder Implementation
// ============================================================================

// builder implements Builder with caching and context support.
// Supports both ClusterDeployment (with URL overrides) and direct kubeconfig secrets.
type builder struct {
	config builderConfig
}

// NewBuilderWithOptions creates a new builder with functional options.
//
// Example usage:
//
//	builder := remoteclient.NewBuilderWithOptions(
//	    remoteclient.WithClusterDeployment(client, cd),
//	    remoteclient.WithControllerName(hivev1.ClustersyncControllerName),
//	    remoteclient.WithCache(sharedCache),
//	)
//
//	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
//	defer cancel()
//
//	remoteClient, err := builder.BuildWithContext(ctx)
func NewBuilderWithOptions(opts ...BuilderOption) Builder {
	cfg := builderConfig{
		useCache:     false,
		urlSelection: activeURL,
	}

	for _, opt := range opts {
		opt(&cfg)
	}

	return &builder{config: cfg}
}

// ============================================================================
// Builder Interface Methods
// ============================================================================

// BuildWithContext creates a controller-runtime client with context support.
// If caching is enabled, this will return a cached client on subsequent calls
// with the same cache key (cluster ID + kubeconfig version + API URL).
func (b *builder) BuildWithContext(ctx context.Context) (client.Client, error) {
	// If caching is disabled, create client directly
	if !b.config.useCache {
		return b.buildClientUncached(ctx)
	}

	// Use cache
	cacheKey, err := b.generateCacheKey(ctx)
	if err != nil {
		return nil, err
	}

	factory := func(ctx context.Context) (client.Client, error) {
		return b.buildClientUncached(ctx)
	}

	return b.config.cache.Get(ctx, cacheKey, factory)
}

// BuildKubeClientWithContext creates a typed Kubernetes client with context support.
func (b *builder) BuildKubeClientWithContext(ctx context.Context) (kubeclient.Interface, error) {
	cfg, err := b.RESTConfigWithContext(ctx)
	if err != nil {
		return nil, err
	}

	kubeClient, err := kubeclient.NewForConfig(cfg)
	if err != nil {
		return nil, b.wrapError(err, "build-kube-client")
	}

	return kubeClient, nil
}

// RESTConfigWithContext returns the REST config with context support.
func (b *builder) RESTConfigWithContext(ctx context.Context) (*rest.Config, error) {
	// Load kubeconfig secret
	secret, err := b.loadSecret(ctx)
	if err != nil {
		return nil, err
	}

	// Get unadulterated REST config from kubeconfig
	cfg, err := clientutil.RestConfigFromSecret(secret, false)
	if err != nil {
		return nil, b.wrapError(err, "parse-kubeconfig")
	}

	kubeconfigURL := cfg.Host

	// Apply metrics wrapper immutably
	cfg = clientutil.CopyConfigWithMetrics(cfg, b.config.controllerName, true)

	// Only apply URL and IP overrides if we have a ClusterDeployment
	// When using a direct kubeconfig secret, use it as-is
	if b.config.cd != nil {
		apiURL := b.getAPIURL(kubeconfigURL)
		ipOverride := b.config.cd.Spec.ControlPlaneConfig.APIServerIPOverride
		cfg = clientutil.PrepareConfigForClient(cfg, apiURL, ipOverride)
	}

	return cfg, nil
}

// UsePrimaryAPIURL returns a new builder configured to use the primary API URL.
// The builder is immutable - this returns a new instance.
func (b *builder) UsePrimaryAPIURL() Builder {
	newConfig := b.config
	newConfig.urlSelection = primaryURL
	return &builder{config: newConfig}
}

// UseSecondaryAPIURL returns a new builder configured to use the secondary API URL.
// The builder is immutable - this returns a new instance.
func (b *builder) UseSecondaryAPIURL() Builder {
	newConfig := b.config
	newConfig.urlSelection = secondaryURL
	return &builder{config: newConfig}
}

// ============================================================================
// Internal Methods
// ============================================================================

// buildClientUncached creates a new client without using the cache.
func (b *builder) buildClientUncached(ctx context.Context) (client.Client, error) {
	cfg, err := b.RESTConfigWithContext(ctx)
	if err != nil {
		return nil, err
	}

	// Verify reachability with context timeout
	if err := b.verifyReachability(ctx, cfg); err != nil {
		return nil, err
	}

	// Create controller-runtime client
	c, err := client.New(cfg, client.Options{
		Scheme: utilscheme.GetScheme(),
	})
	if err != nil {
		return nil, b.wrapError(err, "create-client")
	}

	// Set field owner using unified naming
	fieldManager := clientutil.FieldManagerName(b.config.controllerName)
	return client.WithFieldOwner(c, fieldManager), nil
}

// verifyReachability checks if the cluster is reachable via a simple version check.
// This uses a single lightweight API call (GET /version) instead of fetching all
// API groups and resources, reducing overhead from 3+ seconds to ~50-100ms.
func (b *builder) verifyReachability(ctx context.Context, cfg *rest.Config) error {
	dc, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return b.wrapError(err, "create-discovery-client")
	}

	// Simple version check - single API call instead of querying all resources
	_, err = dc.ServerVersion()
	if err != nil {
		return b.wrapError(err, "verify-reachability")
	}

	return nil
}

// generateCacheKey generates the cache key for this builder.
func (b *builder) generateCacheKey(ctx context.Context) (clientutil.CacheKey, error) {
	secret, err := b.loadSecret(ctx)
	if err != nil {
		return clientutil.CacheKey{}, err
	}

	// Get REST config to extract the kubeconfig URL
	cfg, err := clientutil.RestConfigFromSecret(secret, false)
	if err != nil {
		return clientutil.CacheKey{}, b.wrapError(err, "parse-kubeconfig-for-cache-key")
	}

	kubeconfigURL := cfg.Host
	apiURL := b.getAPIURL(kubeconfigURL)

	if b.config.cd != nil {
		return generateCacheKey(b.config.cd, secret, apiURL), nil
	}

	return generateCacheKeyFromSecret(secret, apiURL), nil
}

// loadSecret loads the kubeconfig secret.
func (b *builder) loadSecret(ctx context.Context) (*corev1.Secret, error) {
	if b.config.kubeconfigSecret != nil {
		return b.config.kubeconfigSecret, nil
	}

	if b.config.cd == nil || b.config.client == nil {
		return nil, fmt.Errorf("neither kubeconfig secret nor (client + ClusterDeployment) provided")
	}

	return loadKubeconfigSecret(ctx, b.config.client, b.config.cd)
}

// getAPIURL determines which API URL to use.
func (b *builder) getAPIURL(kubeconfigURL string) string {
	if b.config.cd == nil {
		return kubeconfigURL
	}

	return determineAPIURL(b.config.cd, b.config.urlSelection, kubeconfigURL)
}

// wrapError wraps an error with cluster context.
func (b *builder) wrapError(err error, operation string) error {
	if err == nil {
		return nil
	}

	var clusterID string
	var gvk schema.GroupVersionKind
	var namespace string
	var name string

	if b.config.cd != nil {
		clusterID = fmt.Sprintf("%s/%s", b.config.cd.Namespace, b.config.cd.Name)
		gvk = hivev1.SchemeGroupVersion.WithKind("ClusterDeployment")
		namespace = b.config.cd.Namespace
		name = b.config.cd.Name
	} else if b.config.kubeconfigSecret != nil {
		clusterID = fmt.Sprintf("%s/%s", b.config.kubeconfigSecret.Namespace, b.config.kubeconfigSecret.Name)
		gvk = corev1.SchemeGroupVersion.WithKind("Secret")
		namespace = b.config.kubeconfigSecret.Namespace
		name = b.config.kubeconfigSecret.Name
	} else {
		clusterID = "unknown"
		gvk = hivev1.SchemeGroupVersion.WithKind("Unknown")
	}

	return clientutil.WrapClusterError(
		err,
		clusterID,
		operation,
		gvk,
		namespace,
		name,
	)
}
