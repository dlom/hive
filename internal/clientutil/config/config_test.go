package config

import (
	"context"
	"net"
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
	if copied.WrapTransport == nil {
		t.Error("CopyConfigWithMetrics() did not apply transport wrapper")
	}

	// Verify original was not wrapped
	if original.WrapTransport != nil {
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

