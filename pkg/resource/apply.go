package resource

import (
	"bytes"
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/hive/internal/clientutil"
	"github.com/openshift/hive/pkg/util/scheme"
)

// Apply applies the given resource using Server-Side Apply.
// This replaces the kubectl-based apply with native Kubernetes APIs, eliminating:
//   - 3+ second OpenAPI schema fetching overhead
//   - kubectl library dependency
//   - os.Args global mutation
//
// Server-Side Apply benefits:
//   - Field-level conflict detection and resolution
//   - Automatic field ownership tracking
//   - No client-side three-way merge
//   - No OpenAPI schema required
func (h *helperImpl) Apply(ctx context.Context, obj interface{}, opts ...ApplyOption) (ApplyResult, error) {
	startTime := time.Now()

	// Parse options
	options := applyOptions{
		fieldManager: clientutil.FieldManagerName(h.controllerName),
		force:        false,
		dryRun:       false,
	}
	for _, opt := range opts {
		opt(&options)
	}

	// Convert input to unstructured
	unstructuredObj, gvk, err := h.toUnstructured(obj)
	if err != nil {
		return ApplyResult{}, h.wrapError(err, "parse-object", gvk, "", "")
	}

	// Check if object exists (for state determination)
	existingObj := &unstructured.Unstructured{}
	existingObj.SetGroupVersionKind(gvk)
	objectKey := client.ObjectKeyFromObject(unstructuredObj)

	exists := false
	if err := h.client.Get(ctx, objectKey, existingObj); err != nil {
		if !apierrors.IsNotFound(err) {
			return ApplyResult{}, h.wrapError(err, "get-existing", gvk, objectKey.Namespace, objectKey.Name)
		}
		// Not found - will be created
	} else {
		exists = true
	}

	// Prepare patch options
	patchOpts := []client.PatchOption{
		client.ForceOwnership, // Always use force ownership for Server-Side Apply
		client.FieldOwner(options.fieldManager),
	}

	if options.force {
		// Force takes ownership of fields from other managers
		force := true
		patchOpts = append(patchOpts, &client.PatchOptions{
			Force: &force,
		})
	}

	if options.dryRun {
		patchOpts = append(patchOpts, client.DryRunAll)
	}

	// Apply using Server-Side Apply
	if err := h.client.Patch(ctx, unstructuredObj, client.Apply, patchOpts...); err != nil {
		// Record operation metrics
		h.recordOperation("apply", gvk, "failure", time.Since(startTime).Seconds())

		return ApplyResult{}, h.wrapError(err, "apply", gvk, objectKey.Namespace, objectKey.Name)
	}

	// Determine result state
	state := Configured
	if !exists {
		state = Created
	} else {
		// Check if anything actually changed by comparing generation or resourceVersion
		// For now, we assume Configured if it existed
		// A more sophisticated check would compare the objects
		state = Configured
	}

	// Record success metrics
	h.recordOperation("apply", gvk, "success", time.Since(startTime).Seconds())

	return ApplyResult{
		State:  state,
		Object: unstructuredObj,
		GVK:    gvk,
	}, nil
}

// toUnstructured converts various input types to *unstructured.Unstructured.
// Accepts:
//   - []byte (YAML or JSON)
//   - runtime.Object
//   - *unstructured.Unstructured
func (h *helperImpl) toUnstructured(obj interface{}) (*unstructured.Unstructured, schema.GroupVersionKind, error) {
	switch v := obj.(type) {
	case []byte:
		// Parse YAML/JSON to unstructured
		return h.parseBytes(v)

	case *unstructured.Unstructured:
		// Already unstructured
		return v, v.GroupVersionKind(), nil

	case runtime.Object:
		// Convert runtime.Object to unstructured
		return h.runtimeObjectToUnstructured(v)

	default:
		return nil, schema.GroupVersionKind{}, fmt.Errorf("unsupported object type: %T", obj)
	}
}

// parseBytes parses YAML or JSON bytes to unstructured.
func (h *helperImpl) parseBytes(data []byte) (*unstructured.Unstructured, schema.GroupVersionKind, error) {
	// Decode YAML/JSON
	obj := &unstructured.Unstructured{}
	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), 4096)

	if err := decoder.Decode(obj); err != nil {
		return nil, schema.GroupVersionKind{}, fmt.Errorf("failed to decode YAML/JSON: %w", err)
	}

	gvk := obj.GroupVersionKind()
	if gvk.Empty() {
		return nil, schema.GroupVersionKind{}, fmt.Errorf("object missing apiVersion or kind")
	}

	return obj, gvk, nil
}

// runtimeObjectToUnstructured converts a runtime.Object to unstructured.
func (h *helperImpl) runtimeObjectToUnstructured(obj runtime.Object) (*unstructured.Unstructured, schema.GroupVersionKind, error) {
	// Get GVK
	gvks, _, err := scheme.GetScheme().ObjectKinds(obj)
	if err != nil {
		return nil, schema.GroupVersionKind{}, fmt.Errorf("failed to get object kind: %w", err)
	}
	if len(gvks) == 0 {
		return nil, schema.GroupVersionKind{}, fmt.Errorf("object has no kind")
	}
	gvk := gvks[0]

	// Convert to unstructured
	unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, gvk, fmt.Errorf("failed to convert to unstructured: %w", err)
	}

	result := &unstructured.Unstructured{Object: unstructuredObj}
	result.SetGroupVersionKind(gvk)

	return result, gvk, nil
}

// wrapError wraps an error with cluster and resource context.
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

// recordOperation records operation metrics.
func (h *helperImpl) recordOperation(operation string, gvk schema.GroupVersionKind, result string, duration float64) {
	if h.controllerName == "" {
		return
	}

	clientutil.RecordOperation(
		string(h.controllerName),
		operation,
		gvk.String(),
		result,
		duration,
	)
}
