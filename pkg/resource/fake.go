package resource

import (
	"context"
	"math/rand"
	"time"

	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

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
