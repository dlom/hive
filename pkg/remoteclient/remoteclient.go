package remoteclient

//go:generate mockgen -source=./remoteclient.go -destination=./mock/remoteclient_generated.go -package=mock

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
)

// ============================================================================
// Builder Interface
// ============================================================================

// Builder creates API clients to remote clusters with context support and client caching.
type Builder interface {
	// BuildWithContext creates a controller-runtime client with context support.
	// If caching is enabled, returns a cached client on subsequent calls.
	BuildWithContext(ctx context.Context) (client.Client, error)

	// BuildKubeClientWithContext creates a typed Kubernetes client with context support.
	BuildKubeClientWithContext(ctx context.Context) (kubeclient.Interface, error)

	// RESTConfigWithContext returns the REST config with context support.
	RESTConfigWithContext(ctx context.Context) (*rest.Config, error)

	// UsePrimaryAPIURL returns a new builder configured to use the primary API URL.
	// If there is an API URL override, that is the primary. Otherwise, the kubeconfig URL is primary.
	UsePrimaryAPIURL() Builder

	// UseSecondaryAPIURL returns a new builder configured to use the secondary API URL.
	// If there is an API URL override, the kubeconfig URL is secondary.
	UseSecondaryAPIURL() Builder
}

// ============================================================================
// URL Selection Helpers (used internally by cache_integration.go)
// ============================================================================

// IsPrimaryURLActive returns true if the APIURLOverride is active (or not set).
// When no override is configured, the kubeconfig URL is always considered primary/active.
func IsPrimaryURLActive(cd *hivev1.ClusterDeployment) bool {
	if cd.Spec.ControlPlaneConfig.APIURLOverride == "" {
		return true
	}
	// Find ActiveAPIURLOverrideCondition
	for _, condition := range cd.Status.Conditions {
		if condition.Type == hivev1.ActiveAPIURLOverrideCondition {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}
