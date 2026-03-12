package resource

import (
	"context"
	"math/rand"
	"time"

	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

// fakeHelper is a dummy implementation of the resource Helper that will never attempt to communicate with the server.
// Used when communicating with a cluster that is flagged as fake for simulated scale testing.
type fakeHelper struct {
	logger log.FieldLogger
}

// NewFakeHelper returns a new fake helper object that does not actually communicate with the cluster.
func NewFakeHelper(logger log.FieldLogger) Helper {
	r := &fakeHelper{
		logger: logger,
	}
	return r
}

func (r *fakeHelper) Apply(obj []byte) (ApplyResult, error) {
	// TODO: would be good to simulate some of the serialization here if possible so we hit CPU/RAM nearly as much as
	// we would in the real world.
	r.fakeApplySleep()
	return ConfiguredApplyResult, nil
}

func (r *fakeHelper) ApplyRuntimeObject(obj runtime.Object, scheme *runtime.Scheme) (ApplyResult, error) {
	r.fakeApplySleep()
	return ConfiguredApplyResult, nil
}

func (r *fakeHelper) fakeApplySleep() {
	// real world data indicates that for our slowest non-delete request type (POST):
	// histogram_quantile(0.9, (sum without(controller,endpoint,instance,job,namespace,pod,resource,service,status)(rate(hive_kube_client_request_seconds_bucket{remote="true",controller="clustersync"}[2h]))))
	//
	// 50% of requests are under 0.027s
	// 80% of requests are under 0.045s
	// 90% of requests are under 0.053s
	// 99% of requests are under 0.230s
	in := []int{27, 27, 27, 27, 27, 45, 45, 45, 53, 230} // milliseconds
	randomIndex := rand.Intn(len(in))
	wait := time.Duration(in[randomIndex] * 1000000) // nanoseconds to match the duration unit
	r.logger.WithField("sleepMillis", wait.Milliseconds()).Debug("sleeping to simulate an apply")
	time.Sleep(wait)
}

func (r *fakeHelper) CreateOrUpdate(obj []byte) (ApplyResult, error) {
	return ConfiguredApplyResult, nil
}

func (r *fakeHelper) CreateOrUpdateRuntimeObject(obj runtime.Object, scheme *runtime.Scheme) (ApplyResult, error) {
	return ConfiguredApplyResult, nil
}

func (r *fakeHelper) Create(obj []byte) (ApplyResult, error) {
	return ConfiguredApplyResult, nil
}

func (r *fakeHelper) CreateRuntimeObject(obj runtime.Object, scheme *runtime.Scheme) (ApplyResult, error) {
	return ConfiguredApplyResult, nil
}

func (r *fakeHelper) Info(obj []byte) (*Info, error) {
	// TODO: Do we need to fake this better?
	return &Info{}, nil
}

func (fakeHelper) Patch(name types.NamespacedName, kind, apiVersion string, patch []byte, patchType string) error {
	return nil
}

func (fakeHelper) Delete(apiVersion, kind, namespace, name string) error {
	return nil
}

// fakeHelperV2 is a dummy implementation of HelperV2 for testing and simulation.
// Used when communicating with a cluster flagged as fake for simulated scale testing.
type fakeHelperV2 struct {
	logger log.FieldLogger
}

// NewFakeHelperV2 returns a new fake v2 helper that does not actually communicate with the cluster.
func NewFakeHelperV2(logger log.FieldLogger) HelperV2 {
	return &fakeHelperV2{
		logger: logger,
	}
}

func (h *fakeHelperV2) Apply(ctx context.Context, obj interface{}, opts ...ApplyOption) (ApplyResultV2, error) {
	// Check context cancellation
	select {
	case <-ctx.Done():
		return ApplyResultV2{}, ctx.Err()
	default:
	}

	// Simulate apply delay
	h.fakeApplySleep()

	// Simulate successful apply - assume configured (resource updated)
	return ApplyResultV2{
		State: ConfiguredV2,
		GVK: schema.GroupVersionKind{
			Group:   "",
			Version: "v1",
			Kind:    "FakeResource",
		},
	}, nil
}

func (h *fakeHelperV2) Patch(ctx context.Context, obj interface{}, patch []byte, opts ...PatchOption) (PatchResultV2, error) {
	// Check context cancellation
	select {
	case <-ctx.Done():
		return PatchResultV2{}, ctx.Err()
	default:
	}

	// Simulate patch delay (shorter than apply)
	h.fakePatchSleep()

	// Simulate successful patch
	return PatchResultV2{
		State: PatchedV2,
	}, nil
}

func (h *fakeHelperV2) PatchWithObject(ctx context.Context, gvk schema.GroupVersionKind, namespace, name string, patch []byte, opts ...PatchOption) (PatchResultV2, error) {
	// Delegate to Patch method
	// In a fake implementation, we don't need to actually construct the full object
	return h.Patch(ctx, nil, patch, opts...)
}

func (h *fakeHelperV2) Delete(ctx context.Context, gvk schema.GroupVersionKind, namespace, name string, opts ...DeleteOption) (DeleteResultV2, error) {
	// Check context cancellation
	select {
	case <-ctx.Done():
		return DeleteResultV2{}, ctx.Err()
	default:
	}

	// Simulate delete delay
	h.fakeDeleteSleep()

	// Simulate successful deletion
	return DeleteResultV2{
		State: DeletedV2,
	}, nil
}

func (h *fakeHelperV2) fakeApplySleep() {
	// Real world data for apply operations
	// 50% of requests are under 0.027s
	// 80% of requests are under 0.045s
	// 90% of requests are under 0.053s
	// 99% of requests are under 0.230s
	in := []int{27, 27, 27, 27, 27, 45, 45, 45, 53, 230} // milliseconds
	randomIndex := rand.Intn(len(in))
	wait := time.Duration(in[randomIndex] * 1000000) // nanoseconds to match the duration unit
	h.logger.WithField("sleepMillis", wait.Milliseconds()).Debug("sleeping to simulate an apply")
	time.Sleep(wait)
}

func (h *fakeHelperV2) fakePatchSleep() {
	// Patch is typically faster than apply
	in := []int{15, 15, 15, 20, 20, 25, 30, 40, 50, 100} // milliseconds
	randomIndex := rand.Intn(len(in))
	wait := time.Duration(in[randomIndex] * 1000000) // nanoseconds
	h.logger.WithField("sleepMillis", wait.Milliseconds()).Debug("sleeping to simulate a patch")
	time.Sleep(wait)
}

func (h *fakeHelperV2) fakeDeleteSleep() {
	// Delete is typically fast
	in := []int{10, 10, 15, 15, 20, 20, 25, 30, 40, 80} // milliseconds
	randomIndex := rand.Intn(len(in))
	wait := time.Duration(in[randomIndex] * 1000000) // nanoseconds
	h.logger.WithField("sleepMillis", wait.Milliseconds()).Debug("sleeping to simulate a delete")
	time.Sleep(wait)
}
