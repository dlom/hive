package utils

import (
	log "github.com/sirupsen/logrus"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/flowcontrol"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/internal/clientutil"
)

// NewClientWithMetricsOrDie creates a controller-runtime client for the local cluster with metrics tracking.
//
// The client uses the manager's cache for reads and is configured with:
//   - Transport wrapper for Prometheus metrics (labeled with controller name)
//   - Unified field manager name (via clientutil.FieldManagerName)
//   - Optional custom rate limiter
//
// This should be used in all Hive controllers for local cluster operations.
// For remote cluster clients, use pkg/remoteclient.Builder instead.
func NewClientWithMetricsOrDie(mgr manager.Manager, controllerName hivev1.ControllerName, rateLimiter *flowcontrol.RateLimiter) client.Client {
	cfg := rest.CopyConfig(mgr.GetConfig())
	if rateLimiter != nil {
		cfg.RateLimiter = *rateLimiter
	}

	// Apply metrics wrapper (remote=false for local cluster)
	cfg = clientutil.CopyConfigWithMetrics(cfg, controllerName, false)

	options := client.Options{
		Scheme: mgr.GetScheme(),
		Mapper: mgr.GetRESTMapper(),
		Cache: &client.CacheOptions{
			Reader: mgr.GetCache(),
		},
	}

	c, err := client.New(cfg, options)
	if err != nil {
		log.WithError(err).Fatal("unable to initialize metrics wrapped client")
	}

	// Use unified field manager naming
	fieldManager := clientutil.FieldManagerName(controllerName)
	return client.WithFieldOwner(c, fieldManager)
}
