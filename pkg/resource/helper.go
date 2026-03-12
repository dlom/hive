package resource

import (
	"context"
	"errors"

	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/internal/clientutil"
)

//go:generate mockgen -source=./helper.go -destination=./mock/helper_generated.go -package=mock

// Helper provides context-aware resource operations using Server-Side Apply.
// It replaces the kubectl-dependent Helper interface with native Kubernetes client APIs.
//
// Key improvements over v1:
//   - Context support for timeout and cancellation
//   - Structured result types instead of strings
//   - Server-Side Apply (no kubectl dependency, no OpenAPI schema overhead)
//   - Unified Apply method (no separate Create/CreateOrUpdate/Apply variants)
//   - Fixed deletion semantics (clear DeletionInProgress state)
//   - No os.Args global mutation
//   - Immutable field manager via clientutil
type Helper interface {
	// Apply applies the given resource using Server-Side Apply.
	// Accepts both []byte (YAML/JSON) and runtime.Object.
	// Context is used for timeout and cancellation.
	//
	// Returns:
	//   - Created: Resource was created
	//   - Configured: Resource was updated
	//   - Unchanged: Resource already in desired state
	Apply(ctx context.Context, obj interface{}, opts ...ApplyOption) (ApplyResult, error)

	// Patch patches the given resource using the specified patch type.
	// Accepts both []byte patch data and runtime.Object.
	//
	// Returns:
	//   - Patched: Resource was patched
	//   - PatchUnchanged: Patch resulted in no changes
	Patch(ctx context.Context, obj interface{}, patch []byte, opts ...PatchOption) (PatchResult, error)

	// PatchWithObject patches a resource by GVK, namespace, and name.
	// This is useful when you have the resource identity but not the full object.
	PatchWithObject(ctx context.Context, gvk schema.GroupVersionKind, namespace, name string, patch []byte, opts ...PatchOption) (PatchResult, error)

	// Delete deletes the specified resource.
	// Returns clear deletion states:
	//   - Deleted: Successfully deleted or already gone
	//   - NotFound: Resource never existed
	//   - DeletionInProgress: Has deletionTimestamp but still exists (finalizers)
	Delete(ctx context.Context, gvk schema.GroupVersionKind, namespace, name string, opts ...DeleteOption) (DeleteResult, error)
}

// helperConfig holds the configuration for a v2 helper.
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

// helper implements Helper using native Kubernetes client APIs.
type helperImpl struct {
	client         client.Client
	logger         log.FieldLogger
	controllerName hivev1.ControllerName
}

// NewHelper creates a new v2 helper with Server-Side Apply support.
//
// Example usage:
//
//	helper, err := resource.NewHelper(
//	    logger,
//	    resource.WithClient(remoteClient),
//	    resource.WithControllerName(hivev1.ClustersyncControllerName),
//	)
//
//	result, err := helper.Apply(ctx, yamlBytes)
//	switch result.State {
//	case resource.Created:
//	    logger.Info("created resource")
//	case resource.Configured:
//	    logger.Info("updated resource")
//	case resource.Unchanged:
//	    logger.Info("no changes needed")
//	}
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
