package config

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"k8s.io/client-go/rest"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
)

func TestCopyConfigWithMetrics_NilConfig(t *testing.T) {
	result := CopyConfigWithMetrics(nil, hivev1.ClustersyncControllerName, false)
	if result != nil {
		t.Errorf("CopyConfigWithMetrics(nil) = %v, want nil", result)
	}
}

func TestCopyConfigWithMetrics_Immutability(t *testing.T) {
	original := &rest.Config{
		Host:        "https://original.example.com:6443",
		BearerToken: "original-token",
	}

	// Copy with metrics
	copied := CopyConfigWithMetrics(original, hivev1.ClustersyncControllerName, false)

	// Verify we got a different instance
	if copied == original {
		t.Error("CopyConfigWithMetrics() returned same instance, want different instance")
	}

	// Modify the copy
	copied.Host = "https://modified.example.com:6443"
	copied.BearerToken = "modified-token"

	// Verify original is unchanged
	if original.Host != "https://original.example.com:6443" {
		t.Errorf("Original Host = %q, want %q (original was mutated!)", original.Host, "https://original.example.com:6443")
	}
	if original.BearerToken != "original-token" {
		t.Errorf("Original BearerToken = %q, want %q (original was mutated!)", original.BearerToken, "original-token")
	}
}

func TestCopyConfigWithMetrics_AppliesWrapper(t *testing.T) {
	original := &rest.Config{
		Host: "https://test.example.com:6443",
	}

	// Copy with metrics
	copied := CopyConfigWithMetrics(original, hivev1.ClustersyncControllerName, false)

	// Verify wrapper was applied
	if !IsTransportWrapped(copied) {
		t.Error("CopyConfigWithMetrics() did not apply transport wrapper")
	}

	// Verify original was not wrapped
	if IsTransportWrapped(original) {
		t.Error("CopyConfigWithMetrics() mutated original config's WrapTransport")
	}
}

func TestPrepareConfigForClient_NilConfig(t *testing.T) {
	result := PrepareConfigForClient(nil, "", "")
	if result != nil {
		t.Errorf("PrepareConfigForClient(nil) = %v, want nil", result)
	}
}

func TestPrepareConfigForClient_APIURLOverride(t *testing.T) {
	original := &rest.Config{
		Host: "https://original.example.com:6443",
	}

	overrideURL := "https://override.example.com:6443"
	result := PrepareConfigForClient(original, overrideURL, "")

	// Verify override was applied
	if result.Host != overrideURL {
		t.Errorf("Host = %q, want %q", result.Host, overrideURL)
	}

	// Verify original is unchanged
	if original.Host != "https://original.example.com:6443" {
		t.Error("Original config was mutated")
	}
}

func TestPrepareConfigForClient_IPOverride(t *testing.T) {
	original := &rest.Config{
		Host: "https://api.example.com:6443",
	}

	ipOverride := "10.0.0.1"
	result := PrepareConfigForClient(original, "", ipOverride)

	// Verify dialer was set
	if result.Dial == nil {
		t.Error("Dial function was not set")
	}

	// Verify original is unchanged
	if original.Dial != nil {
		t.Error("Original config Dial was mutated")
	}
}

func TestPrepareConfigForClient_BothOverrides(t *testing.T) {
	original := &rest.Config{
		Host: "https://original.example.com:6443",
	}

	overrideURL := "https://override.example.com:6443"
	ipOverride := "10.0.0.1"
	result := PrepareConfigForClient(original, overrideURL, ipOverride)

	// Verify both overrides were applied
	if result.Host != overrideURL {
		t.Errorf("Host = %q, want %q", result.Host, overrideURL)
	}
	if result.Dial == nil {
		t.Error("Dial function was not set")
	}

	// Verify original is unchanged
	if original.Host != "https://original.example.com:6443" {
		t.Error("Original config Host was mutated")
	}
	if original.Dial != nil {
		t.Error("Original config Dial was mutated")
	}
}

func TestCreateDialerWithIPOverride_TCP(t *testing.T) {
	ipOverride := "10.0.0.1"
	dialer := createDialerWithIPOverride(ipOverride)

	// Test dialing with a very short timeout to prevent hanging
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := dialer(ctx, "tcp", "api.example.com:6443")

	// We expect either a connection error or timeout (no server at 10.0.0.1:6443)
	// The important thing is that the dialer tried to connect to 10.0.0.1, not api.example.com
	if err == nil {
		t.Error("Expected connection error for fake server")
	}

	// The error could be a timeout or connection refused, both are acceptable
	t.Logf("Got expected error: %v", err)
}

func TestCreateDialerWithIPOverride_UnsupportedNetwork(t *testing.T) {
	ipOverride := "10.0.0.1"
	dialer := createDialerWithIPOverride(ipOverride)

	ctx := context.Background()
	_, err := dialer(ctx, "udp", "api.example.com:6443")

	if err == nil {
		t.Error("Expected error for UDP network")
	}

	opErr, ok := err.(*net.OpError)
	if !ok {
		t.Fatalf("Expected *net.OpError, got %T", err)
	}

	if opErr.Op != "dial" {
		t.Errorf("OpError.Op = %q, want %q", opErr.Op, "dial")
	}
}

func TestCreateDialerWithIPOverride_InvalidAddress(t *testing.T) {
	ipOverride := "10.0.0.1"
	dialer := createDialerWithIPOverride(ipOverride)

	ctx := context.Background()
	_, err := dialer(ctx, "tcp", "invalid-address-no-port")

	if err == nil {
		t.Error("Expected error for invalid address")
	}
}

func TestConfigEquals_BothNil(t *testing.T) {
	if !ConfigEquals(nil, nil) {
		t.Error("ConfigEquals(nil, nil) = false, want true")
	}
}

func TestConfigEquals_OneNil(t *testing.T) {
	cfg := &rest.Config{Host: "https://test.example.com:6443"}

	if ConfigEquals(cfg, nil) {
		t.Error("ConfigEquals(cfg, nil) = true, want false")
	}
	if ConfigEquals(nil, cfg) {
		t.Error("ConfigEquals(nil, cfg) = true, want false")
	}
}

func TestConfigEquals_SameConfigs(t *testing.T) {
	cfg1 := &rest.Config{
		Host:        "https://test.example.com:6443",
		BearerToken: "test-token",
		TLSClientConfig: rest.TLSClientConfig{
			CertData: []byte("cert-data"),
			KeyData:  []byte("key-data"),
			CAData:   []byte("ca-data"),
		},
	}

	cfg2 := &rest.Config{
		Host:        "https://test.example.com:6443",
		BearerToken: "test-token",
		TLSClientConfig: rest.TLSClientConfig{
			CertData: []byte("cert-data"),
			KeyData:  []byte("key-data"),
			CAData:   []byte("ca-data"),
		},
	}

	if !ConfigEquals(cfg1, cfg2) {
		t.Error("ConfigEquals() = false for identical configs, want true")
	}
}

func TestConfigEquals_DifferentHost(t *testing.T) {
	cfg1 := &rest.Config{Host: "https://test1.example.com:6443"}
	cfg2 := &rest.Config{Host: "https://test2.example.com:6443"}

	if ConfigEquals(cfg1, cfg2) {
		t.Error("ConfigEquals() = true for different hosts, want false")
	}
}

func TestConfigEquals_DifferentToken(t *testing.T) {
	cfg1 := &rest.Config{
		Host:        "https://test.example.com:6443",
		BearerToken: "token1",
	}
	cfg2 := &rest.Config{
		Host:        "https://test.example.com:6443",
		BearerToken: "token2",
	}

	if ConfigEquals(cfg1, cfg2) {
		t.Error("ConfigEquals() = true for different tokens, want false")
	}
}

func TestConfigEquals_DifferentCertData(t *testing.T) {
	cfg1 := &rest.Config{
		Host: "https://test.example.com:6443",
		TLSClientConfig: rest.TLSClientConfig{
			CertData: []byte("cert1"),
		},
	}
	cfg2 := &rest.Config{
		Host: "https://test.example.com:6443",
		TLSClientConfig: rest.TLSClientConfig{
			CertData: []byte("cert2"),
		},
	}

	if ConfigEquals(cfg1, cfg2) {
		t.Error("ConfigEquals() = true for different cert data, want false")
	}
}

func TestConfigEquals_IgnoresTimeout(t *testing.T) {
	// ConfigEquals should ignore fields like Timeout that don't affect client identity
	cfg1 := &rest.Config{
		Host:    "https://test.example.com:6443",
		Timeout: 30,
	}
	cfg2 := &rest.Config{
		Host:    "https://test.example.com:6443",
		Timeout: 60,
	}

	// Note: Current implementation doesn't compare Timeout, so this should be equal
	// This is intentional - Timeout is a behavior setting, not an identity field
	if !ConfigEquals(cfg1, cfg2) {
		t.Error("ConfigEquals() = false when only Timeout differs, want true (Timeout should be ignored)")
	}
}

func TestIsTransportWrapped(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *rest.Config
		expected bool
	}{
		{
			name:     "nil config",
			cfg:      nil,
			expected: false,
		},
		{
			name:     "no wrapper",
			cfg:      &rest.Config{},
			expected: false,
		},
		{
			name: "with wrapper",
			cfg: &rest.Config{
				WrapTransport: func(rt http.RoundTripper) http.RoundTripper {
					return rt
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsTransportWrapped(tt.cfg)
			if got != tt.expected {
				t.Errorf("IsTransportWrapped() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestBytesEqual(t *testing.T) {
	tests := []struct {
		name     string
		a        []byte
		b        []byte
		expected bool
	}{
		{
			name:     "both nil",
			a:        nil,
			b:        nil,
			expected: true,
		},
		{
			name:     "both empty",
			a:        []byte{},
			b:        []byte{},
			expected: true,
		},
		{
			name:     "equal",
			a:        []byte("test"),
			b:        []byte("test"),
			expected: true,
		},
		{
			name:     "different content",
			a:        []byte("test1"),
			b:        []byte("test2"),
			expected: false,
		},
		{
			name:     "different length",
			a:        []byte("test"),
			b:        []byte("test123"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := bytesEqual(tt.a, tt.b)
			if got != tt.expected {
				t.Errorf("bytesEqual() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestGetHTTPClient(t *testing.T) {
	// Test with nil config
	client, err := GetHTTPClient(nil)
	if err != nil {
		t.Errorf("GetHTTPClient(nil) error = %v, want nil", err)
	}
	if client != nil {
		t.Errorf("GetHTTPClient(nil) = %v, want nil", client)
	}

	// Test with valid config
	cfg := &rest.Config{
		Host: "https://test.example.com:6443",
	}
	client, err = GetHTTPClient(cfg)
	if err != nil {
		t.Fatalf("GetHTTPClient() error = %v", err)
	}
	if client == nil {
		t.Fatal("GetHTTPClient() returned nil client")
	}
	if client.Transport == nil {
		t.Error("HTTP client has nil Transport")
	}
}
