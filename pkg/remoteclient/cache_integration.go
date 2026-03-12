package remoteclient

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/internal/clientutil"
)

// ============================================================================
// Cache Integration - Cache key generation and URL selection helpers
// ============================================================================

// generateCacheKey creates a cache key for a ClusterDeployment.
// The key includes:
// - Cluster identifier (namespace/name)
// - Kubeconfig secret ResourceVersion (auto-invalidates on cert rotation)
// - API URL (auto-invalidates on failover)
func generateCacheKey(
	cd *hivev1.ClusterDeployment,
	kubeconfigSecret *corev1.Secret,
	apiURL string,
) clientutil.CacheKey {
	clusterID := fmt.Sprintf("%s/%s", cd.Namespace, cd.Name)
	kubeconfigVersion := kubeconfigSecret.ResourceVersion

	return clientutil.NewCacheKey(clusterID, kubeconfigVersion, apiURL)
}

// generateCacheKeyFromSecret creates a cache key when using a secret directly.
func generateCacheKeyFromSecret(
	secret *corev1.Secret,
	apiURL string,
) clientutil.CacheKey {
	clusterID := fmt.Sprintf("%s/%s", secret.Namespace, secret.Name)
	kubeconfigVersion := secret.ResourceVersion

	return clientutil.NewCacheKey(clusterID, kubeconfigVersion, apiURL)
}

// determineAPIURL determines which API URL to use based on the URL selection.
// This implements the logic for primary, secondary, and active URL selection.
func determineAPIURL(cd *hivev1.ClusterDeployment, urlSelection int, kubeconfigURL string) string {
	override := cd.Spec.ControlPlaneConfig.APIURLOverride

	switch urlSelection {
	case primaryURL:
		// Primary: override if set, else kubeconfig URL
		if override != "" {
			return override
		}
		return kubeconfigURL

	case secondaryURL:
		// Secondary: kubeconfig URL if override set, else override
		if override != "" {
			return kubeconfigURL
		}
		return override

	case activeURL:
		// Active: determined by ActiveAPIURLOverrideCondition
		if override == "" {
			// No override, use kubeconfig URL
			return kubeconfigURL
		}

		// Check if override is active
		if IsPrimaryURLActive(cd) {
			return override
		}
		return kubeconfigURL

	default:
		// Default to active URL behavior
		return determineAPIURL(cd, activeURL, kubeconfigURL)
	}
}

// loadKubeconfigSecret loads the kubeconfig secret for a ClusterDeployment.
func loadKubeconfigSecret(ctx context.Context, c client.Client, cd *hivev1.ClusterDeployment) (*corev1.Secret, error) {
	if cd.Spec.ClusterMetadata == nil {
		return nil, fmt.Errorf("cluster metadata is nil")
	}

	secretRef := cd.Spec.ClusterMetadata.AdminKubeconfigSecretRef
	secret := &corev1.Secret{}
	key := client.ObjectKey{
		Namespace: cd.Namespace,
		Name:      secretRef.Name,
	}

	if err := c.Get(ctx, key, secret); err != nil {
		return nil, clientutil.WrapClusterError(
			err,
			fmt.Sprintf("%s/%s", cd.Namespace, cd.Name),
			"load-kubeconfig",
			hivev1.SchemeGroupVersion.WithKind("ClusterDeployment"),
			cd.Namespace,
			cd.Name,
		)
	}

	return secret, nil
}
