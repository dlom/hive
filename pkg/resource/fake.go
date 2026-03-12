package resource

import (
	"context"
	"math/rand"
	"time"

	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// fakeHelper is a dummy implementation of Helper for testing and simulation.
// Used when communicating with a cluster flagged as fake for simulated scale testing.
type fakeHelper struct {
	logger log.FieldLogger
}

// NewFakeHelper returns a new fake helper that does not actually communicate with the cluster.
func NewFakeHelper(logger log.FieldLogger) Helper {
	return &fakeHelper{
		logger: logger,
	}
}

func (h *fakeHelper) Apply(ctx context.Context, obj interface{}, opts ...ApplyOption) (ApplyResult, error) {
	// Check context cancellation
	select {
	case <-ctx.Done():
		return ApplyResult{}, ctx.Err()
	default:
	}

	// Simulate apply delay
	h.fakeApplySleep()

	// Simulate successful apply - assume configured (resource updated)
	return ApplyResult{
		State: Configured,
		GVK: schema.GroupVersionKind{
			Group:   "",
			Version: "v1",
			Kind:    "FakeResource",
		},
	}, nil
}

func (h *fakeHelper) Patch(ctx context.Context, obj interface{}, patch []byte, opts ...PatchOption) (PatchResult, error) {
	// Check context cancellation
	select {
	case <-ctx.Done():
		return PatchResult{}, ctx.Err()
	default:
	}

	// Simulate patch delay (shorter than apply)
	h.fakePatchSleep()

	// Simulate successful patch
	return PatchResult{
		State: Patched,
	}, nil
}

func (h *fakeHelper) PatchWithObject(ctx context.Context, gvk schema.GroupVersionKind, namespace, name string, patch []byte, opts ...PatchOption) (PatchResult, error) {
	// Delegate to Patch method
	// In a fake implementation, we don't need to actually construct the full object
	return h.Patch(ctx, nil, patch, opts...)
}

func (h *fakeHelper) Delete(ctx context.Context, gvk schema.GroupVersionKind, namespace, name string, opts ...DeleteOption) (DeleteResult, error) {
	// Check context cancellation
	select {
	case <-ctx.Done():
		return DeleteResult{}, ctx.Err()
	default:
	}

	// Simulate delete delay
	h.fakeDeleteSleep()

	// Simulate successful deletion
	return DeleteResult{
		State: Deleted,
	}, nil
}

func (h *fakeHelper) fakeApplySleep() {
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

func (h *fakeHelper) fakePatchSleep() {
	// Patch is typically faster than apply
	in := []int{15, 15, 15, 20, 20, 25, 30, 40, 50, 100} // milliseconds
	randomIndex := rand.Intn(len(in))
	wait := time.Duration(in[randomIndex] * 1000000) // nanoseconds
	h.logger.WithField("sleepMillis", wait.Milliseconds()).Debug("sleeping to simulate a patch")
	time.Sleep(wait)
}

func (h *fakeHelper) fakeDeleteSleep() {
	// Delete is typically fast
	in := []int{10, 10, 15, 15, 20, 20, 25, 30, 40, 80} // milliseconds
	randomIndex := rand.Intn(len(in))
	wait := time.Duration(in[randomIndex] * 1000000) // nanoseconds
	h.logger.WithField("sleepMillis", wait.Milliseconds()).Debug("sleeping to simulate a delete")
	time.Sleep(wait)
}
