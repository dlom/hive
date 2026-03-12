package config

import (
	"context"
	"net"

	"k8s.io/client-go/rest"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/internal/clientutil/metrics"
)

// CopyConfigWithMetrics creates a deep copy of the REST config and applies the metrics wrapper.
// This function ensures immutability - it never mutates the input config.
//
// The metrics wrapper is applied exactly once, even if called multiple times. This fixes the
// bug in pkg/controller/utils/clientwrapper.go where wrappers could accumulate.
//
// Parameters:
//   - cfg: The REST config to copy (not modified)
//   - controllerName: The name of the controller for metrics labels
//   - remote: Whether this is for a remote cluster (for metrics labels)
//
// Returns: A new REST config with the metrics wrapper applied
func CopyConfigWithMetrics(cfg *rest.Config, controllerName hivev1.ControllerName, remote bool) *rest.Config {
	if cfg == nil {
		return nil
	}

	// Deep copy the config
	newCfg := rest.CopyConfig(cfg)

	// Apply metrics wrapper immutably
	metrics.AddControllerMetricsTransportWrapper(newCfg, controllerName, remote)

	return newCfg
}

// PrepareConfigForClient creates a new REST config with URL and IP overrides applied.
// This function ensures immutability - it never mutates the input config.
//
// Parameters:
//   - cfg: The REST config to copy (not modified)
//   - apiURLOverride: Optional API URL to use instead of the one in the config
//   - ipOverride: Optional IP address to dial instead of resolving the hostname
//
// Returns: A new REST config with overrides applied
func PrepareConfigForClient(cfg *rest.Config, apiURLOverride string, ipOverride string) *rest.Config {
	if cfg == nil {
		return nil
	}

	// Deep copy the config
	newCfg := rest.CopyConfig(cfg)

	// Apply API URL override if specified
	if apiURLOverride != "" {
		newCfg.Host = apiURLOverride
	}

	// Apply IP override via custom dialer if specified
	if ipOverride != "" {
		newCfg.Dial = createDialerWithIPOverride(ipOverride)
	}

	return newCfg
}

// createDialerWithIPOverride creates a custom dialer that replaces the hostname with a specific IP.
// This is used for HIVE-2272 workaround for Kubernetes memory leak.
func createDialerWithIPOverride(ipOverride string) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		// Only support TCP
		if network != "tcp" {
			return nil, &net.OpError{
				Op:  "dial",
				Net: network,
				Err: &net.AddrError{Err: "unsupported network", Addr: addr},
			}
		}

		// Extract port from original address
		_, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, &net.OpError{
				Op:   "dial",
				Net:  network,
				Addr: &net.TCPAddr{},
				Err:  err,
			}
		}

		// Replace hostname with IP override
		newAddr := net.JoinHostPort(ipOverride, port)

		// Use standard dialer with context
		return (&net.Dialer{}).DialContext(ctx, network, newAddr)
	}
}

