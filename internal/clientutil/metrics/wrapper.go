package metrics

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
)

// ControllerMetricsTripper is a RoundTripper implementation which tracks metrics for client requests.
// This is copied from pkg/controller/utils/clientwrapper.go with bug fixes.
type ControllerMetricsTripper struct {
	http.RoundTripper
	Controller hivev1.ControllerName
	Remote     bool
}

// AddControllerMetricsTransportWrapper adds a transport wrapper to the given REST config.
// This fixes the bug in pkg/controller/utils/clientwrapper.go where the wrapper logic
// was incorrect (lines 83-101).
//
// The bug: The function checked if WrapTransport != nil, wrapped it, but then
// unconditionally overwrote it at line 95, losing the wrapped version.
//
// The fix: Only wrap if not already wrapped, OR properly chain if there's an existing wrapper.
func AddControllerMetricsTransportWrapper(cfg *rest.Config, controllerName hivev1.ControllerName, remote bool) {
	if cfg == nil {
		return
	}

	// Check if we already have a metrics wrapper to avoid duplicate wrapping
	if cfg.WrapTransport != nil {
		// Test if the existing wrapper is already our metrics wrapper
		// We do this by wrapping a test transport and checking the type
		testTransport := &http.Transport{}
		wrapped := cfg.WrapTransport(testTransport)
		if _, ok := wrapped.(*ControllerMetricsTripper); ok {
			// Already wrapped with our metrics tripper, don't wrap again
			return
		}

		// Existing wrapper is something else, chain it
		origFunc := cfg.WrapTransport
		cfg.WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
			return &ControllerMetricsTripper{
				RoundTripper: origFunc(rt),
				Controller:   controllerName,
				Remote:       remote,
			}
		}
		return
	}

	// No existing wrapper, set ours
	cfg.WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
		return &ControllerMetricsTripper{
			RoundTripper: rt,
			Controller:   controllerName,
			Remote:       remote,
		}
	}
}

// CancelRequest implements request cancellation and tracks cancellation metrics.
func (cmt *ControllerMetricsTripper) CancelRequest(req *http.Request) {
	remoteStr := strconv.FormatBool(cmt.Remote)
	path, _ := parsePath(req.URL.Path)

	// Record cancellation metric
	metricKubeClientRequestsCancelled.WithLabelValues(
		cmt.Controller.String(),
		req.Method,
		path,
		remoteStr,
	).Inc()

	log.WithFields(log.Fields{
		"controller": cmt.Controller.String(),
		"method":     req.Method,
		"path":       path,
		"remote":     remoteStr,
	}).Warn("cancelled request")
}

// RoundTrip implements the http.RoundTripper interface.
func (cmt *ControllerMetricsTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	startTime := time.Now()
	remoteStr := "false"
	if cmt.Remote {
		remoteStr = "true"
	}
	path, pathErr := parsePath(req.URL.Path)

	// Call the nested RoundTripper
	resp, err := cmt.RoundTripper.RoundTrip(req)
	applyTime := metav1.Now().Sub(startTime)

	if err == nil && pathErr == nil {
		// Record metrics
		metricKubeClientRequests.WithLabelValues(
			cmt.Controller.String(),
			req.Method,
			path,
			remoteStr,
			resp.Status,
		).Inc()

		metricKubeClientRequestSeconds.WithLabelValues(
			cmt.Controller.String(),
			req.Method,
			path,
			remoteStr,
			resp.Status,
		).Observe(applyTime.Seconds())

		// Log slow requests
		if applyTime >= 5*time.Second {
			log.WithFields(log.Fields{
				"controller":    cmt.Controller.String(),
				"method":        req.Method,
				"path":          path,
				"remote":        remoteStr,
				"status":        resp.Status,
				"elapsedMillis": applyTime.Milliseconds(),
			}).Warn("slow client request")
		}
	}

	return resp, err
}

// parsePath returns a group/version/resource string from the given path.
// Used to avoid per-cluster metrics for cardinality reasons.
// Copied from pkg/controller/utils/clientwrapper.go
func parsePath(path string) (string, error) {
	tokens := strings.Split(path[1:], "/")
	switch tokens[0] {
	case "api":
		// Handle core resources
		if len(tokens) == 3 || len(tokens) == 4 {
			return strings.Join([]string{"core", tokens[1], tokens[2]}, "/"), nil
		}
		// Handle operators on direct namespaced resources
		if len(tokens) > 4 && tokens[2] == "namespaces" {
			return strings.Join([]string{"core", tokens[1], tokens[4]}, "/"), nil
		}
	case "apis":
		// Handle resources with apigroups
		if len(tokens) == 4 || len(tokens) == 5 {
			return strings.Join([]string{tokens[1], tokens[2], tokens[3]}, "/"), nil
		}
		if len(tokens) > 5 && tokens[3] == "namespaces" {
			return strings.Join([]string{tokens[1], tokens[2], tokens[5]}, "/"), nil
		}
	}
	return "", fmt.Errorf("unable to parse path for client metrics: %s", path)
}
