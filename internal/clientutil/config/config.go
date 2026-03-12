// Package config provides utilities for preparing REST configs for remote cluster connections.
//
// This package handles:
//   - Adding metrics transport wrappers to track HTTP requests by controller
//   - Applying API URL overrides for cluster failover scenarios
//   - Applying IP overrides for direct routing to cluster API servers
//   - Working around upstream Kubernetes memory leaks (HIVE-2272)
//
// All functions in this package are immutable - they return new configs without
// modifying the input.
package config

import (
	"context"
	"net"
	"net/http"
	"time"

	"k8s.io/client-go/rest"
	machnet "k8s.io/apimachinery/pkg/util/net"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/internal/clientutil/metrics"
)

// CopyConfigWithMetrics returns a deep copy of the REST config with metrics transport wrapper applied.
//
// The wrapper tracks all HTTP requests made through this config, labeling them with the controller
// name and remote flag for observability in Prometheus. The wrapper is applied exactly once and
// will not re-wrap already wrapped configs.
//
// The input config is never modified.
func CopyConfigWithMetrics(cfg *rest.Config, controllerName hivev1.ControllerName, remote bool) *rest.Config {
	if cfg == nil {
		return nil
	}

	newCfg := rest.CopyConfig(cfg)
	metrics.AddControllerMetricsTransportWrapper(newCfg, controllerName, remote)

	return newCfg
}

// PrepareConfigForClient returns a deep copy of the REST config with URL and IP overrides applied.
//
// The apiURLOverride replaces the config's Host field, used for API URL failover when the
// ClusterDeployment specifies an APIURLOverride.
//
// The ipOverride installs a custom dialer that replaces DNS resolution with a direct IP,
// used when the ClusterDeployment specifies an APIServerIPOverride for direct routing.
// When IP override is used, a proxy workaround is also applied to prevent memory leaks
// in the Kubernetes HTTP client (see https://github.com/kubernetes/kubernetes/issues/118703).
//
// The input config is never modified.
func PrepareConfigForClient(cfg *rest.Config, apiURLOverride string, ipOverride string) *rest.Config {
	if cfg == nil {
		return nil
	}

	newCfg := rest.CopyConfig(cfg)

	if apiURLOverride != "" {
		newCfg.Host = apiURLOverride
	}

	if ipOverride != "" {
		newCfg.Dial = createDialerWithIPOverride(ipOverride)

		// HIVE-2272: When using a custom dialer, set a custom Proxy function to prevent
		// the default proxy logic from leaking memory (kubernetes/kubernetes#118703).
		// TODO: Remove when upstream fix is available.
		newCfg.Proxy = machnet.NewProxierWithNoProxyCIDR(http.ProxyFromEnvironment)
	}

	return newCfg
}

// createDialerWithIPOverride returns a dial function that replaces the hostname with a fixed IP address.
//
// This is used when ClusterDeployment.Spec.ControlPlaneConfig.APIServerIPOverride is set,
// allowing direct routing to a cluster API server without DNS resolution. The port from the
// original address is preserved.
//
// The dialer is configured with 30 second timeout and TCP keepalive matching Kubernetes
// client defaults. Only TCP connections are supported.
func createDialerWithIPOverride(ipOverride string) func(context.Context, string, string) (net.Conn, error) {
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		if network != "tcp" {
			return nil, &net.OpError{
				Op:  "dial",
				Net: network,
				Err: &net.AddrError{Err: "unsupported network", Addr: addr},
			}
		}

		_, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, &net.OpError{
				Op:   "dial",
				Net:  network,
				Addr: &net.TCPAddr{},
				Err:  err,
			}
		}

		newAddr := net.JoinHostPort(ipOverride, port)
		return dialer.DialContext(ctx, network, newAddr)
	}
}

