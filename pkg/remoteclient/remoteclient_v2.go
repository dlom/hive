package remoteclient

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
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

// BuilderV2 is the v2 interface with context support and client caching.
// It extends the v1 Builder interface with context-aware methods.
type BuilderV2 interface {
	Builder // Embed v1 interface for backward compatibility

	// Context-aware methods (preferred for new code)
	BuildWithContext(ctx context.Context) (client.Client, error)
	BuildDynamicWithContext(ctx context.Context) (dynamic.Interface, error)
	BuildKubeClientWithContext(ctx context.Context) (kubeclient.Interface, error)
	RESTConfigWithContext(ctx context.Context) (*rest.Config, error)

	// URL selection methods that return BuilderV2 (for method chaining)
	UsePrimaryAPIURLV2() BuilderV2
	UseSecondaryAPIURLV2() BuilderV2
}

// builderV2 implements BuilderV2 with caching and context support.
type builderV2 struct {
	config builderConfig
}

// NewBuilderV2 creates a new v2 builder with functional options.
//
// Example usage:
//
//	builder := remoteclient.NewBuilderV2(
//	    remoteclient.WithClusterDeployment(client, cd),
//	    remoteclient.WithControllerName(hivev1.ClustersyncControllerName),
//	    remoteclient.WithCache(sharedCache),
//	)
//
//	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
//	defer cancel()
//
//	remoteClient, err := builder.BuildWithContext(ctx)
func NewBuilderV2(opts ...BuilderOption) BuilderV2 {
	cfg := builderConfig{
		useCache:     false, // Default to no caching (v1 behavior)
		urlSelection: activeURL,
	}

	for _, opt := range opts {
		opt(&cfg)
	}

	return &builderV2{config: cfg}
}

// BuildWithContext creates a controller-runtime client with context support.
// If caching is enabled, this will return a cached client on subsequent calls
// with the same cache key (cluster ID + kubeconfig version + API URL).
func (b *builderV2) BuildWithContext(ctx context.Context) (client.Client, error) {
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
func (b *builderV2) BuildDynamicWithContext(ctx context.Context) (dynamic.Interface, error) {
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
func (b *builderV2) BuildKubeClientWithContext(ctx context.Context) (kubeclient.Interface, error) {
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
func (b *builderV2) RESTConfigWithContext(ctx context.Context) (*rest.Config, error) {
	// Load kubeconfig secret
	secret, err := b.loadSecret(ctx)
	if err != nil {
		return nil, err
	}

	// Get unadulterated REST config from kubeconfig
	cfg, err := utils.RestConfigFromSecret(secret, false)
	if err != nil {
		return nil, b.wrapError(err, "parse-kubeconfig")
	}

	// Get the kubeconfig URL from the config (before any overrides)
	kubeconfigURL := cfg.Host

	// Apply metrics wrapper immutably
	cfg = clientutil.CopyConfigWithMetrics(cfg, b.config.controllerName, true)

	// Determine which API URL to use
	apiURL := b.getAPIURL(kubeconfigURL)

	// Apply URL and IP overrides if needed
	var ipOverride string
	if b.config.cd != nil {
		ipOverride = b.config.cd.Spec.ControlPlaneConfig.APIServerIPOverride
	}

	cfg = clientutil.PrepareConfigForClient(cfg, apiURL, ipOverride)

	return cfg, nil
}

// buildClientUncached creates a new client without using the cache.
func (b *builderV2) buildClientUncached(ctx context.Context) (client.Client, error) {
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
func (b *builderV2) verifyReachability(ctx context.Context, cfg *rest.Config) error {
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
func (b *builderV2) generateCacheKey(ctx context.Context) (clientutil.CacheKey, error) {
	secret, err := b.loadSecret(ctx)
	if err != nil {
		return clientutil.CacheKey{}, err
	}

	// Get REST config to extract the kubeconfig URL
	cfg, err := utils.RestConfigFromSecret(secret, false)
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
func (b *builderV2) loadSecret(ctx context.Context) (*corev1.Secret, error) {
	if b.config.kubeconfigSecret != nil {
		return b.config.kubeconfigSecret, nil
	}

	if b.config.cd == nil || b.config.client == nil {
		return nil, fmt.Errorf("neither kubeconfig secret nor (client + ClusterDeployment) provided")
	}

	return loadKubeconfigSecret(ctx, b.config.client, b.config.cd)
}

// getAPIURL determines which API URL to use.
func (b *builderV2) getAPIURL(kubeconfigURL string) string {
	if b.config.cd == nil {
		return kubeconfigURL
	}

	return determineAPIURL(b.config.cd, b.config.urlSelection, kubeconfigURL)
}

// wrapError wraps an error with cluster context.
func (b *builderV2) wrapError(err error, operation string) error {
	if err == nil {
		return nil
	}

	clusterID := "unknown"
	if b.config.cd != nil {
		clusterID = fmt.Sprintf("%s/%s", b.config.cd.Namespace, b.config.cd.Name)
	} else if b.config.kubeconfigSecret != nil {
		clusterID = fmt.Sprintf("%s/%s", b.config.kubeconfigSecret.Namespace, b.config.kubeconfigSecret.Name)
	}

	return clientutil.WrapClusterError(
		err,
		clusterID,
		operation,
		hivev1.SchemeGroupVersion.WithKind("ClusterDeployment"),
		"", // namespace
		"", // name
	)
}

// Backward compatibility methods - delegate to context versions with context.Background()

// Build implements Builder.Build() for backward compatibility.
func (b *builderV2) Build() (client.Client, error) {
	return b.BuildWithContext(context.Background())
}

// BuildDynamic implements Builder.BuildDynamic() for backward compatibility.
func (b *builderV2) BuildDynamic() (dynamic.Interface, error) {
	return b.BuildDynamicWithContext(context.Background())
}

// BuildKubeClient implements Builder.BuildKubeClient() for backward compatibility.
func (b *builderV2) BuildKubeClient() (kubeclient.Interface, error) {
	return b.BuildKubeClientWithContext(context.Background())
}

// RESTConfig implements Builder.RESTConfig() for backward compatibility.
func (b *builderV2) RESTConfig() (*rest.Config, error) {
	return b.RESTConfigWithContext(context.Background())
}

// UsePrimaryAPIURL implements Builder.UsePrimaryAPIURL().
// Returns a new builder with primary URL selection (immutable).
// Note: Returns Builder interface for v1 compatibility. Use UsePrimaryAPIURLV2() for v2 chaining.
func (b *builderV2) UsePrimaryAPIURL() Builder {
	newConfig := b.config
	newConfig.urlSelection = primaryURL
	return &builderV2{config: newConfig}
}

// UseSecondaryAPIURL implements Builder.UseSecondaryAPIURL().
// Returns a new builder with secondary URL selection (immutable).
// Note: Returns Builder interface for v1 compatibility. Use UseSecondaryAPIURLV2() for v2 chaining.
func (b *builderV2) UseSecondaryAPIURL() Builder {
	newConfig := b.config
	newConfig.urlSelection = secondaryURL
	return &builderV2{config: newConfig}
}

// UsePrimaryAPIURLV2 returns a new builder with primary URL selection.
// This is the v2 version that returns BuilderV2 to enable method chaining with BuildWithContext().
func (b *builderV2) UsePrimaryAPIURLV2() BuilderV2 {
	newConfig := b.config
	newConfig.urlSelection = primaryURL
	return &builderV2{config: newConfig}
}

// UseSecondaryAPIURLV2 returns a new builder with secondary URL selection.
// This is the v2 version that returns BuilderV2 to enable method chaining with BuildWithContext().
func (b *builderV2) UseSecondaryAPIURLV2() BuilderV2 {
	newConfig := b.config
	newConfig.urlSelection = secondaryURL
	return &builderV2{config: newConfig}
}
