package remoteclient

import (
	"context"
	"fmt"

	"github.com/pkg/errors"

	configv1 "github.com/openshift/api/config/v1"
	routev1 "github.com/openshift/api/route/v1"
	testfake "github.com/openshift/hive/pkg/test/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// fakeBuilder builds fake clients for fake clusters. Used to simulate communication with a cluster
// that doesn't actually exist in scale testing.
type fakeBuilder struct {
	clusterVersion string
}

// BuildWithContext returns a fake controller-runtime test client populated with the resources we expect to query for a
// fake cluster. Context is respected for cancellation but not used for I/O (fake client is local).
func (b *fakeBuilder) BuildWithContext(ctx context.Context) (client.Client, error) {
	// Check if context is already canceled
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	fakeObjects := []runtime.Object{
		&routev1.Route{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "console",
				Namespace: "openshift-console",
			},
			Spec: routev1.RouteSpec{
				Host: "https://example.com/veryfakewebconsole",
			},
		},

		&configv1.ClusterVersion{
			ObjectMeta: metav1.ObjectMeta{
				Name: "version",
			},
			Status: configv1.ClusterVersionStatus{
				Desired: configv1.Release{
					Version: b.clusterVersion,
				},
			},
		},
	}

	// As of 4.6 there are approximately 30 ClusterOperators. Return dummy data
	// so we help simulate the storage of ClusterState.
	for i := 0; i < 30; i++ {
		fakeObjects = append(fakeObjects, &configv1.ClusterOperator{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("fake-operator-%d", i),
			},
			Status: configv1.ClusterOperatorStatus{
				Conditions: []configv1.ClusterOperatorStatusCondition{
					{
						Type:               configv1.OperatorAvailable,
						Status:             configv1.ConditionTrue,
						Reason:             "AsExpected",
						Message:            "everything's cool",
						LastTransitionTime: metav1.Now(),
					},
					{
						Type:               configv1.OperatorDegraded,
						Status:             configv1.ConditionFalse,
						Reason:             "AsExpected",
						Message:            "everything's cool",
						LastTransitionTime: metav1.Now(),
					},
					{
						Type:               configv1.OperatorUpgradeable,
						Status:             configv1.ConditionTrue,
						Reason:             "AsExpected",
						Message:            "everything's cool",
						LastTransitionTime: metav1.Now(),
					},
					{
						Type:               configv1.OperatorProgressing,
						Status:             configv1.ConditionFalse,
						Reason:             "AsExpected",
						Message:            "everything's cool",
						LastTransitionTime: metav1.Now(),
					},
				},
			},
		})
	}

	return testfake.NewFakeClientBuilder().WithRuntimeObjects(fakeObjects...).Build(), nil
}

// BuildKubeClientWithContext implements Builder.BuildKubeClientWithContext().
func (b *fakeBuilder) BuildKubeClientWithContext(ctx context.Context) (kubeclient.Interface, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	return nil, errors.New("BuildKubeClientWithContext not implemented for fake cluster client builder")
}

// RESTConfigWithContext implements Builder.RESTConfigWithContext().
func (b *fakeBuilder) RESTConfigWithContext(ctx context.Context) (*rest.Config, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	return nil, errors.New("RESTConfigWithContext not implemented for fake cluster client builder")
}

// UsePrimaryAPIURL implements Builder.UsePrimaryAPIURL().
// For fake builders, there's no URL override, so just return self.
func (b *fakeBuilder) UsePrimaryAPIURL() Builder {
	return b
}

// UseSecondaryAPIURL implements Builder.UseSecondaryAPIURL().
// For fake builders, there's no URL override, so just return self.
func (b *fakeBuilder) UseSecondaryAPIURL() Builder {
	return b
}
