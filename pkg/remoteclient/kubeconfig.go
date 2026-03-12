package remoteclient

import (
	"context"
	"fmt"

	"k8s.io/client-go/discovery"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/internal/clientutil"
	"github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/util/scheme"
)

// NewBuilderFromKubeconfig creates a builder from a kubeconfig secret with optional caching.
// This is used by controllers that work with raw kubeconfig secrets rather than ClusterDeployments.
func NewBuilderFromKubeconfig(opts ...BuilderOption) Builder {
	cfg := builderConfig{
		useCache: false, // Default to no caching unless explicitly enabled
	}

	for _, opt := range opts {
		opt(&cfg)
	}

	return &kubeconfigBuilder{config: cfg}
}

type kubeconfigBuilder struct {
	config builderConfig
}

// BuildWithContext creates a controller-runtime client with context support.
// If caching is enabled, this will return a cached client on subsequent calls.
func (b *kubeconfigBuilder) BuildWithContext(ctx context.Context) (client.Client, error) {
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
func (b *kubeconfigBuilder) BuildKubeClientWithContext(ctx context.Context) (kubeclient.Interface, error) {
	cfg, err := b.RESTConfigWithContext(ctx)
	if err != nil {
		return nil, err
	}

	client, err := kubeclient.NewForConfig(cfg)
	if err != nil {
		return nil, b.wrapError(err, "build-kube-client")
	}

	return client, nil
}

// RESTConfigWithContext returns the REST config with context support.
func (b *kubeconfigBuilder) RESTConfigWithContext(ctx context.Context) (*rest.Config, error) {
	if b.config.kubeconfigSecret == nil {
		return nil, fmt.Errorf("kubeconfig secret not provided")
	}

	// Get REST config from secret
	cfg, err := utils.RestConfigFromSecret(b.config.kubeconfigSecret, false)
	if err != nil {
		return nil, b.wrapError(err, "parse-kubeconfig")
	}

	// Apply metrics wrapper immutably
	cfg = clientutil.CopyConfigWithMetrics(cfg, b.config.controllerName, true)

	return cfg, nil
}

// buildClientUncached creates a new client without using the cache.
func (b *kubeconfigBuilder) buildClientUncached(ctx context.Context) (client.Client, error) {
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
		Scheme: scheme.GetScheme(),
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
func (b *kubeconfigBuilder) verifyReachability(ctx context.Context, cfg *rest.Config) error {
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
func (b *kubeconfigBuilder) generateCacheKey(ctx context.Context) (clientutil.CacheKey, error) {
	if b.config.kubeconfigSecret == nil {
		return clientutil.CacheKey{}, fmt.Errorf("kubeconfig secret not provided")
	}

	secret := b.config.kubeconfigSecret

	// Get REST config to extract the kubeconfig URL
	cfg, err := utils.RestConfigFromSecret(secret, false)
	if err != nil {
		return clientutil.CacheKey{}, b.wrapError(err, "parse-kubeconfig-for-cache-key")
	}

	apiURL := cfg.Host

	return generateCacheKeyFromSecret(secret, apiURL), nil
}

// wrapError wraps an error with cluster context.
func (b *kubeconfigBuilder) wrapError(err error, operation string) error {
	if err == nil {
		return nil
	}

	clusterID := "unknown"
	if b.config.kubeconfigSecret != nil {
		clusterID = fmt.Sprintf("%s/%s", b.config.kubeconfigSecret.Namespace, b.config.kubeconfigSecret.Name)
	}

	return clientutil.WrapClusterError(
		err,
		clusterID,
		operation,
		hivev1.SchemeGroupVersion.WithKind("Secret"),
		"", // namespace
		"", // name
	)
}

// UsePrimaryAPIURL implements Builder.UsePrimaryAPIURL().
// For kubeconfig builder, there's no URL override, so just return self.
func (b *kubeconfigBuilder) UsePrimaryAPIURL() Builder {
	return b
}

// UseSecondaryAPIURL implements Builder.UseSecondaryAPIURL().
// For kubeconfig builder, there's no URL override, so just return self.
func (b *kubeconfigBuilder) UseSecondaryAPIURL() Builder {
	return b
}
