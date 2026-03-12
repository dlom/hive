package remoteclient

import (
	"context"
	"fmt"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/internal/clientutil"
	"github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/util/scheme"
)

// NewBuilderFromKubeconfigV2 creates a v2 builder from a kubeconfig secret with optional caching.
// This is used by controllers that work with raw kubeconfig secrets rather than ClusterDeployments.
func NewBuilderFromKubeconfigV2(opts ...BuilderOption) BuilderV2 {
	cfg := builderConfig{
		useCache: false, // Default to no caching unless explicitly enabled
	}

	for _, opt := range opts {
		opt(&cfg)
	}

	return &kubeconfigBuilderV2{config: cfg}
}

type kubeconfigBuilderV2 struct {
	config builderConfig
}

// BuildWithContext creates a controller-runtime client with context support.
// If caching is enabled, this will return a cached client on subsequent calls.
func (b *kubeconfigBuilderV2) BuildWithContext(ctx context.Context) (client.Client, error) {
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

// BuildDynamicWithContext creates a dynamic client with context support.
func (b *kubeconfigBuilderV2) BuildDynamicWithContext(ctx context.Context) (dynamic.Interface, error) {
	cfg, err := b.RESTConfigWithContext(ctx)
	if err != nil {
		return nil, err
	}

	client, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, b.wrapError(err, "build-dynamic-client")
	}

	return client, nil
}

// BuildKubeClientWithContext creates a typed Kubernetes client with context support.
func (b *kubeconfigBuilderV2) BuildKubeClientWithContext(ctx context.Context) (kubeclient.Interface, error) {
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
func (b *kubeconfigBuilderV2) RESTConfigWithContext(ctx context.Context) (*rest.Config, error) {
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
func (b *kubeconfigBuilderV2) buildClientUncached(ctx context.Context) (client.Client, error) {
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

// verifyReachability checks if the cluster is reachable via discovery.
func (b *kubeconfigBuilderV2) verifyReachability(ctx context.Context, cfg *rest.Config) error {
	dc, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return b.wrapError(err, "create-discovery-client")
	}

	// Use context for timeout control
	_, err = restmapper.GetAPIGroupResources(dc)
	if err != nil {
		return b.wrapError(err, "verify-reachability")
	}

	return nil
}

// generateCacheKey generates the cache key for this builder.
func (b *kubeconfigBuilderV2) generateCacheKey(ctx context.Context) (clientutil.CacheKey, error) {
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
func (b *kubeconfigBuilderV2) wrapError(err error, operation string) error {
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

// Backward compatibility methods - delegate to context versions with context.Background()

// Build implements Builder.Build() for backward compatibility.
func (b *kubeconfigBuilderV2) Build() (client.Client, error) {
	return b.BuildWithContext(context.Background())
}

// BuildDynamic implements Builder.BuildDynamic() for backward compatibility.
func (b *kubeconfigBuilderV2) BuildDynamic() (dynamic.Interface, error) {
	return b.BuildDynamicWithContext(context.Background())
}

// BuildKubeClient implements Builder.BuildKubeClient() for backward compatibility.
func (b *kubeconfigBuilderV2) BuildKubeClient() (kubeclient.Interface, error) {
	return b.BuildKubeClientWithContext(context.Background())
}

// RESTConfig implements Builder.RESTConfig() for backward compatibility.
func (b *kubeconfigBuilderV2) RESTConfig() (*rest.Config, error) {
	return b.RESTConfigWithContext(context.Background())
}

// UsePrimaryAPIURL implements Builder.UsePrimaryAPIURL().
// For kubeconfig builder, there's no URL override, so just return self.
func (b *kubeconfigBuilderV2) UsePrimaryAPIURL() Builder {
	return b
}

// UseSecondaryAPIURL implements Builder.UseSecondaryAPIURL().
// For kubeconfig builder, there's no URL override, so just return self.
func (b *kubeconfigBuilderV2) UseSecondaryAPIURL() Builder {
	return b
}

// UsePrimaryAPIURLV2 returns a new builder with primary URL selection.
// For kubeconfig builder, there's no URL override, so just return self.
func (b *kubeconfigBuilderV2) UsePrimaryAPIURLV2() BuilderV2 {
	return b
}

// UseSecondaryAPIURLV2 returns a new builder with secondary URL selection.
// For kubeconfig builder, there's no URL override, so just return self.
func (b *kubeconfigBuilderV2) UseSecondaryAPIURLV2() BuilderV2 {
	return b
}
