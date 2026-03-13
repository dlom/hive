package resource

import (
	"context"
	"math/rand"
	"time"

	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// fakeHelper is a no-op Helper for scale testing fake clusters.
type fakeHelper struct {
	logger log.FieldLogger
}

func NewFakeHelper(logger log.FieldLogger) Helper {
	return &fakeHelper{
		logger: logger,
	}
}

func (h *fakeHelper) Apply(ctx context.Context, obj interface{}) (ApplyState, error) {
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	default:
	}

	h.fakeApplySleep()
	return Configured, nil
}

func (h *fakeHelper) Patch(ctx context.Context, obj interface{}, patch []byte, opts ...PatchOption) (PatchState, error) {
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	default:
	}

	h.fakePatchSleep()
	return Patched, nil
}

func (h *fakeHelper) PatchWithObject(ctx context.Context, gvk schema.GroupVersionKind, namespace, name string, patch []byte, opts ...PatchOption) (PatchState, error) {
	return h.Patch(ctx, nil, patch, opts...)
}

func (h *fakeHelper) Delete(ctx context.Context, gvk schema.GroupVersionKind, namespace, name string) (DeleteState, error) {
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	default:
	}

	h.fakeDeleteSleep()
	return Deleted, nil
}

func (h *fakeHelper) fakeApplySleep() {
	latencies := []int{27, 27, 27, 27, 27, 45, 45, 45, 53, 230}
	time.Sleep(time.Duration(latencies[rand.Intn(len(latencies))]) * time.Millisecond)
}

func (h *fakeHelper) fakePatchSleep() {
	latencies := []int{15, 15, 15, 20, 20, 25, 30, 40, 50, 100}
	time.Sleep(time.Duration(latencies[rand.Intn(len(latencies))]) * time.Millisecond)
}

func (h *fakeHelper) fakeDeleteSleep() {
	latencies := []int{10, 10, 15, 15, 20, 20, 25, 30, 40, 80}
	time.Sleep(time.Duration(latencies[rand.Intn(len(latencies))]) * time.Millisecond)
}
