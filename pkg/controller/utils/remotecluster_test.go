package utils

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	testcd "github.com/openshift/hive/pkg/test/clusterdeployment"
	testfake "github.com/openshift/hive/pkg/test/fake"
)

const (
	testKubeconfigSecretName = "test-kubeconfig"
	apiURL                   = "https://api.hive-cluster.example.com:6443"
)

func Test_InitialURL(t *testing.T) {
	cd := testClusterDeployment()
	kubeconfigSecret := testKubeconfigSecret(t)
	c := fakeClient(cd, kubeconfigSecret)
	expected := apiURL
	actual, err := InitialURL(c, cd)
	assert.NoError(t, err, "unexpected error getting API URL")
	assert.Equal(t, expected, actual, "unexpected API URL")
}

func Test_Unreachable(t *testing.T) {
	probeTime := time.Unix(123456789, 0)
	cases := []struct {
		name                string
		cd                  *hivev1.ClusterDeployment
		expectedUnreachable bool
		expectedLastCheck   time.Time
	}{
		{
			name: "unreachable still unknown",
			cd: testcd.Build(testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Status: corev1.ConditionUnknown,
				Type:   hivev1.UnreachableCondition,
			})),
			expectedUnreachable: true,
		},
		{
			name:                "unreachable true",
			cd:                  testcd.Build(withUnreachableCondition(corev1.ConditionTrue, probeTime)),
			expectedUnreachable: true,
			expectedLastCheck:   probeTime,
		},
		{
			name:                "unreachable false",
			cd:                  testcd.Build(withUnreachableCondition(corev1.ConditionFalse, probeTime)),
			expectedUnreachable: false,
			expectedLastCheck:   probeTime,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actualUnreachable, actualLastCheck := Unreachable(tc.cd)
			assert.Equal(t, tc.expectedUnreachable, actualUnreachable, "unexpected unreachable")
			assert.Equal(t, tc.expectedLastCheck, actualLastCheck, "unexpected last check")
		})
	}
}

func fakeClient(objects ...runtime.Object) client.Client {
	return testfake.NewFakeClientBuilder().WithRuntimeObjects(objects...).Build()
}

func testClusterDeployment() *hivev1.ClusterDeployment {
	return &hivev1.ClusterDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster-deployment",
			Namespace: testNamespace,
		},
		Spec: hivev1.ClusterDeploymentSpec{
			ClusterMetadata: &hivev1.ClusterMetadata{
				AdminKubeconfigSecretRef: corev1.LocalObjectReference{Name: testKubeconfigSecretName},
			},
		},
	}
}

func withUnreachableCondition(status corev1.ConditionStatus, probeTime time.Time) testcd.Option {
	return testcd.WithCondition(
		hivev1.ClusterDeploymentCondition{
			Type:          hivev1.UnreachableCondition,
			Status:        status,
			LastProbeTime: metav1.NewTime(probeTime),
		},
	)
}

func testKubeconfigSecret(t *testing.T) *corev1.Secret {
	// Use the kubeconfig from remoteclient testdata
	kubeconfigFile := filepath.Join("..", "..", "remoteclient", "testdata", "kubeconfig.sample")
	kubeconfig, err := os.ReadFile(kubeconfigFile)
	if err != nil {
		t.Fatal(err)
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNamespace,
			Name:      testKubeconfigSecretName,
		},
		Data: map[string][]byte{constants.KubeconfigSecretKey: kubeconfig},
	}
}
