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

// HelperV2 provides context-aware resource operations using Server-Side Apply.
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
type HelperV2 interface {
	// Apply applies the given resource using Server-Side Apply.
	// Accepts both []byte (YAML/JSON) and runtime.Object.
	// Context is used for timeout and cancellation.
	//
	// Returns:
	//   - Created: Resource was created
	//   - Configured: Resource was updated
	//   - Unchanged: Resource already in desired state
	Apply(ctx context.Context, obj interface{}, opts ...ApplyOption) (ApplyResultV2, error)

	// Patch patches the given resource using the specified patch type.
	// Accepts both []byte patch data and runtime.Object.
	//
	// Returns:
	//   - PatchedV2: Resource was patched
	//   - PatchUnchangedV2: Patch resulted in no changes
	Patch(ctx context.Context, obj interface{}, patch []byte, opts ...PatchOption) (PatchResultV2, error)

	// PatchWithObject patches a resource by GVK, namespace, and name.
	// This is useful when you have the resource identity but not the full object.
	PatchWithObject(ctx context.Context, gvk schema.GroupVersionKind, namespace, name string, patch []byte, opts ...PatchOption) (PatchResultV2, error)

	// Delete deletes the specified resource.
	// Returns clear deletion states:
	//   - DeletedV2: Successfully deleted or already gone
	//   - NotFoundV2: Resource never existed
	//   - DeletionInProgressV2: Has deletionTimestamp but still exists (finalizers)
	Delete(ctx context.Context, gvk schema.GroupVersionKind, namespace, name string, opts ...DeleteOption) (DeleteResultV2, error)
}

// helperV2Config holds the configuration for a v2 helper.
type helperV2Config struct {
	client         client.Client
	restConfig     *rest.Config
	logger         log.FieldLogger
	controllerName hivev1.ControllerName
}

// HelperV2Option is a functional option for configuring a HelperV2.
type HelperV2Option func(*helperV2Config)

// WithClient sets the controller-runtime client to use for operations.
// This is the preferred way to configure the helper.
func WithClient(c client.Client) HelperV2Option {
	return func(cfg *helperV2Config) {
		cfg.client = c
	}
}

// WithRESTConfigV2 sets the REST config to create a client.
// Use this when you have a REST config but not a client.
func WithRESTConfigV2(restCfg *rest.Config) HelperV2Option {
	return func(cfg *helperV2Config) {
		cfg.restConfig = restCfg
	}
}

// WithControllerNameV2 sets the controller name for field manager and metrics.
// This is required for proper field ownership tracking.
func WithControllerNameV2(name hivev1.ControllerName) HelperV2Option {
	return func(cfg *helperV2Config) {
		cfg.controllerName = name
	}
}

// helperV2 implements HelperV2 using native Kubernetes client APIs.
type helperV2 struct {
	client         client.Client
	logger         log.FieldLogger
	controllerName hivev1.ControllerName
}

// NewHelperV2 creates a new v2 helper with Server-Side Apply support.
//
// Example usage:
//
//	helper, err := resource.NewHelperV2(
//	    logger,
//	    resource.WithClient(remoteClient),
//	    resource.WithControllerNameV2(hivev1.ClustersyncControllerName),
//	)
//
//	result, err := helper.Apply(ctx, yamlBytes)
//	switch result.State {
//	case resource.CreatedV2:
//	    logger.Info("created resource")
//	case resource.ConfiguredV2:
//	    logger.Info("updated resource")
//	case resource.UnchangedV2:
//	    logger.Info("no changes needed")
//	}
func NewHelperV2(logger log.FieldLogger, opts ...HelperV2Option) (HelperV2, error) {
	cfg := &helperV2Config{
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

	return &helperV2{
		client:         cfg.client,
		logger:         logger,
		controllerName: cfg.controllerName,
	}, nil
}
