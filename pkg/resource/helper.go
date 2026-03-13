package resource

import (
	"context"
	"errors"

	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/internal/clientutil"
)

//go:generate mockgen -source=./helper.go -destination=./mock/helper_generated.go -package=mock

// Helper provides resource operations using Server-Side Apply.
type Helper interface {
	// Apply applies the given resource using Server-Side Apply.
	// Accepts []byte (YAML/JSON) or runtime.Object.
	Apply(ctx context.Context, obj interface{}) (ApplyState, error)

	// Patch patches a resource by GVK, namespace, and name.
	Patch(ctx context.Context, gvk schema.GroupVersionKind, namespace, name string, patch []byte, patchType types.PatchType) (PatchState, error)

	// Delete deletes a resource.
	Delete(ctx context.Context, gvk schema.GroupVersionKind, namespace, name string) (DeleteState, error)
}

// helperConfig holds the configuration for a Helper.
type helperConfig struct {
	client         client.Client
	restConfig     *rest.Config
	logger         log.FieldLogger
	controllerName hivev1.ControllerName
}

// HelperOption is a functional option for configuring a Helper.
type HelperOption func(*helperConfig)

// WithClient sets the controller-runtime client to use for operations.
// This is the preferred way to configure the helper.
func WithClient(c client.Client) HelperOption {
	return func(cfg *helperConfig) {
		cfg.client = c
	}
}

// WithRESTConfig sets the REST config to create a client.
// Use this when you have a REST config but not a client.
func WithRESTConfig(restCfg *rest.Config) HelperOption {
	return func(cfg *helperConfig) {
		cfg.restConfig = restCfg
	}
}

// WithControllerName sets the controller name for field manager and metrics.
// This is required for proper field ownership tracking.
func WithControllerName(name hivev1.ControllerName) HelperOption {
	return func(cfg *helperConfig) {
		cfg.controllerName = name
	}
}

type helperImpl struct {
	client         client.Client
	logger         log.FieldLogger
	controllerName hivev1.ControllerName
}

// NewHelper creates a new resource helper.
func NewHelper(logger log.FieldLogger, opts ...HelperOption) (Helper, error) {
	cfg := &helperConfig{
		logger: logger,
	}

	for _, opt := range opts {
		opt(cfg)
	}

	// If client not provided, create from REST config
	if cfg.client == nil && cfg.restConfig != nil {
		c, err := client.New(cfg.restConfig, client.Options{})
		if err != nil {
			return nil, clientutil.WrapClusterError(
				err,
				"unknown",
				"create-client",
				schema.GroupVersionKind{},
				"", "",
			)
		}
		cfg.client = c
	}

	if cfg.client == nil {
		return nil, clientutil.WrapClusterError(
			errors.New("neither client nor REST config provided"),
			"unknown",
			"create-helper",
			schema.GroupVersionKind{},
			"", "",
		)
	}

	return &helperImpl{
		client:         cfg.client,
		logger:         logger,
		controllerName: cfg.controllerName,
	}, nil
}

func (h *helperImpl) wrapError(err error, operation string, gvk schema.GroupVersionKind, namespace, name string) error {
	if err == nil {
		return nil
	}

	return clientutil.WrapClusterError(
		err,
		"remote-cluster", // ClusterID would come from context in real usage
		operation,
		gvk,
		namespace,
		name,
	)
}

func (h *helperImpl) recordOperation(operation string, gvk schema.GroupVersionKind, result string, dur float64) {
	if h.controllerName == "" {
		return
	}

	recordOperation(
		string(h.controllerName),
		operation,
		gvk.String(),
		result,
		dur,
	)
}
