package remoteclient

import (
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/internal/clientutil"
)

// URL selection constants
const (
	activeURL int = iota
	primaryURL
	secondaryURL
)

// builderConfig holds the configuration for building remote clients.
type builderConfig struct {
	// Required: either (client + cd) or kubeconfigSecret
	client client.Client
	cd     *hivev1.ClusterDeployment

	// Alternative: direct kubeconfig secret
	kubeconfigSecret *corev1.Secret

	// Controller name for metrics and field manager
	controllerName hivev1.ControllerName

	// Cache configuration
	cache    clientutil.ClientCache
	useCache bool

	// URL selection
	urlSelection int // activeURL, primaryURL, or secondaryURL
}

// BuilderOption is a functional option for configuring a Builder.
type BuilderOption func(*builderConfig)

// WithClusterDeployment configures the builder to use a ClusterDeployment.
// This is the primary way to create remote clients.
func WithClusterDeployment(c client.Client, cd *hivev1.ClusterDeployment) BuilderOption {
	return func(cfg *builderConfig) {
		cfg.client = c
		cfg.cd = cd
	}
}

// WithKubeconfigSecret configures the builder to use a kubeconfig secret directly.
// This is useful when you have the secret but not the ClusterDeployment.
func WithKubeconfigSecret(secret *corev1.Secret) BuilderOption {
	return func(cfg *builderConfig) {
		cfg.kubeconfigSecret = secret
	}
}

// WithControllerName sets the controller name for metrics and field manager.
// This is required for proper metrics labeling and field ownership tracking.
func WithControllerName(name hivev1.ControllerName) BuilderOption {
	return func(cfg *builderConfig) {
		cfg.controllerName = name
	}
}

// WithCache enables client caching with the specified cache.
// Cached clients are reused across reconciliations, providing 90-97% performance improvement.
// The cache automatically invalidates entries on certificate rotation and API URL failover.
func WithCache(cache clientutil.ClientCache) BuilderOption {
	return func(cfg *builderConfig) {
		cfg.cache = cache
		cfg.useCache = true
	}
}

