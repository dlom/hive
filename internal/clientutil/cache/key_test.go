package cache

import (
	"testing"
)

func TestNewCacheKey(t *testing.T) {
	clusterID := "hive/test-cluster"
	version := "12345"
	apiURL := "https://api.test-cluster.example.com:6443"

	key := NewCacheKey(clusterID, version, apiURL)

	if key.ClusterID != clusterID {
		t.Errorf("ClusterID = %q, want %q", key.ClusterID, clusterID)
	}
	if key.KubeconfigVersion != version {
		t.Errorf("KubeconfigVersion = %q, want %q", key.KubeconfigVersion, version)
	}
	if key.APIURL != apiURL {
		t.Errorf("APIURL = %q, want %q", key.APIURL, apiURL)
	}
}

func TestCacheKey_String(t *testing.T) {
	tests := []struct {
		name     string
		key      CacheKey
		expected string
	}{
		{
			name: "standard key",
			key: CacheKey{
				ClusterID:         "hive/test-cluster",
				KubeconfigVersion: "12345",
				APIURL:            "https://api.test-cluster.example.com:6443",
			},
			expected: "hive/test-cluster#12345#https://api.test-cluster.example.com:6443",
		},
		{
			name: "key with special characters",
			key: CacheKey{
				ClusterID:         "namespace-1/cluster-test",
				KubeconfigVersion: "v123-456",
				APIURL:            "https://10.0.0.1:6443",
			},
			expected: "namespace-1/cluster-test#v123-456#https://10.0.0.1:6443",
		},
		{
			name: "key with empty version",
			key: CacheKey{
				ClusterID:         "hive/test-cluster",
				KubeconfigVersion: "",
				APIURL:            "https://api.test-cluster.example.com:6443",
			},
			expected: "hive/test-cluster##https://api.test-cluster.example.com:6443",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.key.String()
			if got != tt.expected {
				t.Errorf("String() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestCacheKey_Hash(t *testing.T) {
	tests := []struct {
		name          string
		key1          CacheKey
		key2          CacheKey
		shouldBeEqual bool
	}{
		{
			name: "identical keys have same hash",
			key1: CacheKey{
				ClusterID:         "hive/test-cluster",
				KubeconfigVersion: "12345",
				APIURL:            "https://api.test-cluster.example.com:6443",
			},
			key2: CacheKey{
				ClusterID:         "hive/test-cluster",
				KubeconfigVersion: "12345",
				APIURL:            "https://api.test-cluster.example.com:6443",
			},
			shouldBeEqual: true,
		},
		{
			name: "different cluster IDs have different hashes",
			key1: CacheKey{
				ClusterID:         "hive/cluster1",
				KubeconfigVersion: "12345",
				APIURL:            "https://api.test-cluster.example.com:6443",
			},
			key2: CacheKey{
				ClusterID:         "hive/cluster2",
				KubeconfigVersion: "12345",
				APIURL:            "https://api.test-cluster.example.com:6443",
			},
			shouldBeEqual: false,
		},
		{
			name: "different versions have different hashes",
			key1: CacheKey{
				ClusterID:         "hive/test-cluster",
				KubeconfigVersion: "12345",
				APIURL:            "https://api.test-cluster.example.com:6443",
			},
			key2: CacheKey{
				ClusterID:         "hive/test-cluster",
				KubeconfigVersion: "67890",
				APIURL:            "https://api.test-cluster.example.com:6443",
			},
			shouldBeEqual: false,
		},
		{
			name: "different API URLs have different hashes",
			key1: CacheKey{
				ClusterID:         "hive/test-cluster",
				KubeconfigVersion: "12345",
				APIURL:            "https://api-primary.example.com:6443",
			},
			key2: CacheKey{
				ClusterID:         "hive/test-cluster",
				KubeconfigVersion: "12345",
				APIURL:            "https://api-secondary.example.com:6443",
			},
			shouldBeEqual: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash1 := tt.key1.Hash()
			hash2 := tt.key2.Hash()

			if tt.shouldBeEqual {
				if hash1 != hash2 {
					t.Errorf("Hash() for equal keys: %d != %d", hash1, hash2)
				}
			} else {
				// While hash collisions are theoretically possible, they should be
				// extremely rare for our use case
				if hash1 == hash2 {
					t.Errorf("Hash() for different keys unexpectedly equal: %d == %d", hash1, hash2)
				}
			}
		})
	}
}

func TestCacheKey_Equals(t *testing.T) {
	tests := []struct {
		name     string
		key1     CacheKey
		key2     CacheKey
		expected bool
	}{
		{
			name: "identical keys are equal",
			key1: CacheKey{
				ClusterID:         "hive/test-cluster",
				KubeconfigVersion: "12345",
				APIURL:            "https://api.test-cluster.example.com:6443",
			},
			key2: CacheKey{
				ClusterID:         "hive/test-cluster",
				KubeconfigVersion: "12345",
				APIURL:            "https://api.test-cluster.example.com:6443",
			},
			expected: true,
		},
		{
			name: "different cluster IDs not equal",
			key1: CacheKey{
				ClusterID:         "hive/cluster1",
				KubeconfigVersion: "12345",
				APIURL:            "https://api.test-cluster.example.com:6443",
			},
			key2: CacheKey{
				ClusterID:         "hive/cluster2",
				KubeconfigVersion: "12345",
				APIURL:            "https://api.test-cluster.example.com:6443",
			},
			expected: false,
		},
		{
			name: "different versions not equal",
			key1: CacheKey{
				ClusterID:         "hive/test-cluster",
				KubeconfigVersion: "12345",
				APIURL:            "https://api.test-cluster.example.com:6443",
			},
			key2: CacheKey{
				ClusterID:         "hive/test-cluster",
				KubeconfigVersion: "67890",
				APIURL:            "https://api.test-cluster.example.com:6443",
			},
			expected: false,
		},
		{
			name: "different API URLs not equal",
			key1: CacheKey{
				ClusterID:         "hive/test-cluster",
				KubeconfigVersion: "12345",
				APIURL:            "https://api-primary.example.com:6443",
			},
			key2: CacheKey{
				ClusterID:         "hive/test-cluster",
				KubeconfigVersion: "12345",
				APIURL:            "https://api-secondary.example.com:6443",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.key1.Equals(tt.key2)
			if got != tt.expected {
				t.Errorf("Equals() = %v, want %v", got, tt.expected)
			}

			// Test symmetry
			got2 := tt.key2.Equals(tt.key1)
			if got2 != got {
				t.Errorf("Equals() not symmetric: %v != %v", got, got2)
			}
		})
	}
}

func TestCacheKey_HashConsistency(t *testing.T) {
	// Verify that the same key produces the same hash multiple times
	key := CacheKey{
		ClusterID:         "hive/test-cluster",
		KubeconfigVersion: "12345",
		APIURL:            "https://api.test-cluster.example.com:6443",
	}

	hash1 := key.Hash()
	hash2 := key.Hash()
	hash3 := key.Hash()

	if hash1 != hash2 || hash2 != hash3 {
		t.Errorf("Hash() not consistent: %d, %d, %d", hash1, hash2, hash3)
	}
}

func TestCacheKey_AutoInvalidationScenarios(t *testing.T) {
	// Test scenarios that should trigger automatic cache invalidation

	t.Run("certificate rotation changes version", func(t *testing.T) {
		keyBefore := CacheKey{
			ClusterID:         "hive/test-cluster",
			KubeconfigVersion: "12345",
			APIURL:            "https://api.test-cluster.example.com:6443",
		}

		keyAfter := CacheKey{
			ClusterID:         "hive/test-cluster",
			KubeconfigVersion: "67890", // ResourceVersion changed
			APIURL:            "https://api.test-cluster.example.com:6443",
		}

		if keyBefore.Equals(keyAfter) {
			t.Error("Keys should not be equal after certificate rotation")
		}

		if keyBefore.Hash() == keyAfter.Hash() {
			t.Error("Hashes should differ after certificate rotation")
		}
	})

	t.Run("API URL failover changes URL", func(t *testing.T) {
		keyPrimary := CacheKey{
			ClusterID:         "hive/test-cluster",
			KubeconfigVersion: "12345",
			APIURL:            "https://api-primary.example.com:6443",
		}

		keySecondary := CacheKey{
			ClusterID:         "hive/test-cluster",
			KubeconfigVersion: "12345",
			APIURL:            "https://api-secondary.example.com:6443", // URL changed
		}

		if keyPrimary.Equals(keySecondary) {
			t.Error("Keys should not be equal after API URL failover")
		}

		if keyPrimary.Hash() == keySecondary.Hash() {
			t.Error("Hashes should differ after API URL failover")
		}
	})
}
