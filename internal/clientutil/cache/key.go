package cache

import (
	"fmt"
	"hash/fnv"
)

// CacheKey represents a unique identifier for a cached client.
// The key includes cluster identifier, kubeconfig version, and API URL to enable
// automatic cache invalidation on certificate rotation and API URL failover.
type CacheKey struct {
	// ClusterID is the namespace/name of the cluster (e.g., "hive/my-cluster")
	ClusterID string

	// KubeconfigVersion is the ResourceVersion of the kubeconfig secret.
	// When the secret updates (e.g., certificate rotation), the ResourceVersion
	// changes, causing automatic cache invalidation.
	KubeconfigVersion string

	// APIURL is the current API URL in use (primary, secondary, or override).
	// When the API URL changes (e.g., failover), the cache key no longer matches,
	// causing automatic cache invalidation.
	APIURL string
}

// NewCacheKey creates a new cache key from its components.
func NewCacheKey(clusterID, kubeconfigVersion, apiURL string) CacheKey {
	return CacheKey{
		ClusterID:         clusterID,
		KubeconfigVersion: kubeconfigVersion,
		APIURL:            apiURL,
	}
}

// String returns a string representation of the cache key.
// Format: {clusterID}#{kubeconfigVersion}#{apiURL}
func (k CacheKey) String() string {
	return fmt.Sprintf("%s#%s#%s", k.ClusterID, k.KubeconfigVersion, k.APIURL)
}

// Hash returns a hash value for the cache key suitable for use as a map key.
// This enables efficient cache lookups using a map[uint64]entry structure if needed.
func (k CacheKey) Hash() uint64 {
	h := fnv.New64a()
	h.Write([]byte(k.String()))
	return h.Sum64()
}

// Equals returns true if two cache keys are equal.
func (k CacheKey) Equals(other CacheKey) bool {
	return k.ClusterID == other.ClusterID &&
		k.KubeconfigVersion == other.KubeconfigVersion &&
		k.APIURL == other.APIURL
}
