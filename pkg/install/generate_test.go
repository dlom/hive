package install

import (
	"testing"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	hiveassert "github.com/openshift/hive/pkg/test/assert"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	installerImage = "fakeinstallerimage"
	cliImage       = "fakecliimage"
)

const (
	testHttpProxy  = "localhost:3112"
	testHttpsProxy = "localhost:4432"
	testNoProxy    = "example.com,foo.com,bar.org"
)

func init() {
	log.SetLevel(log.DebugLevel)
}

func TestGenerateDeprovision(t *testing.T) {
	dr := testClusterDeprovision()
	job, err := GenerateUninstallerJobForDeprovision(
		dr, "someseviceaccount",
		testHttpProxy, testHttpsProxy, testNoProxy,
		nil,
		controllerutils.SharedPodConfig{}, log.StandardLogger())
	assert.Nil(t, err)
	assert.NotNil(t, job)
	hiveassert.AssertAllContainersHaveEnvVar(t, &job.Spec.Template.Spec, "HTTP_PROXY", testHttpProxy)
	hiveassert.AssertAllContainersHaveEnvVar(t, &job.Spec.Template.Spec, "HTTPS_PROXY", testHttpsProxy)
	hiveassert.AssertAllContainersHaveEnvVar(t, &job.Spec.Template.Spec, "NO_PROXY", testNoProxy)

	// Verify init container copies hiveutil from the hive image (mirrors install path)
	assert.Len(t, job.Spec.Template.Spec.InitContainers, 1, "expected exactly one init container")
	assert.Equal(t, "hive", job.Spec.Template.Spec.InitContainers[0].Name)

	// Verify main container uses the installer image and runs deprovision
	assert.Len(t, job.Spec.Template.Spec.Containers, 1, "expected exactly one container")
	assert.Equal(t, *dr.Spec.InstallerImage, job.Spec.Template.Spec.Containers[0].Image)
	assert.Equal(t, []string{"/bin/sh", "-c"}, job.Spec.Template.Spec.Containers[0].Command)
	assert.Contains(t, job.Spec.Template.Spec.Containers[0].Args[0], "deprovision")
	assert.Contains(t, job.Spec.Template.Spec.Containers[0].Args[0], "--metadata-json-secret-name")
	assert.Contains(t, job.Spec.Template.Spec.Containers[0].Args[0], "openshift-install")
}

func TestGenerateDeprovisionMissingInstallerImage(t *testing.T) {
	dr := testClusterDeprovision()
	dr.Spec.InstallerImage = nil
	_, err := GenerateUninstallerJobForDeprovision(
		dr, "sa",
		"", "", "",
		nil,
		controllerutils.SharedPodConfig{}, log.StandardLogger())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "InstallerImage is required")
}

func TestGenerateDeprovisionMissingMetadata(t *testing.T) {
	dr := testClusterDeprovision()
	dr.Spec.MetadataJSONSecretRef = nil
	_, err := GenerateUninstallerJobForDeprovision(
		dr, "sa",
		"", "", "",
		nil,
		controllerutils.SharedPodConfig{}, log.StandardLogger())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "MetadataJSONSecretRef is required")
}

func testClusterDeprovision() *hivev1.ClusterDeprovision {
	img := "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:abc123"
	return &hivev1.ClusterDeprovision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: hivev1.ClusterDeprovisionSpec{
			InfraID:   "test-infra-id",
			ClusterID: "test-cluster-id",
			MetadataJSONSecretRef: &corev1.LocalObjectReference{
				Name: "foo-metadata",
			},
			InstallerImage: &img,
			Platform: hivev1.ClusterDeprovisionPlatform{
				AWS: &hivev1.AWSClusterDeprovision{
					Region: "us-east-1",
					CredentialsSecretRef: &corev1.LocalObjectReference{
						Name: "aws-creds",
					},
				},
			},
		},
	}
}

func TestInstallerPodSpec(t *testing.T) {
	tests := []struct {
		name               string
		clusterDeployment  *hivev1.ClusterDeployment
		provisionName      string
		releaseImage       string
		serviceAccountName string
		pvcName            string
		skipGatherLogs     bool
		extraEnvVars       []corev1.EnvVar
		validate           func(*testing.T, *corev1.PodSpec, error)
	}{
		{
			name: "Test Provision Pod Resource Requests",
			clusterDeployment: &hivev1.ClusterDeployment{
				Spec: hivev1.ClusterDeploymentSpec{
					Provisioning: &hivev1.Provisioning{
						InstallConfigSecretRef: &corev1.LocalObjectReference{Name: "foo"},
					},
				},
				Status: hivev1.ClusterDeploymentStatus{
					InstallerImage: &installerImage,
					CLIImage:       &cliImage,
				},
			},
			provisionName:  "testprovision",
			skipGatherLogs: true,
			extraEnvVars: []corev1.EnvVar{
				{
					Name:  "TESTVAR",
					Value: "TESTVAL",
				},
			},
			validate: func(t *testing.T, actualPodSpec *corev1.PodSpec, actualError error) {
				expectedPodMemoryRequest := resource.MustParse("800Mi")
				actualPodMemoryRequest := actualPodSpec.Containers[0].Resources.Requests[corev1.ResourceMemory]

				assert.Equal(t, expectedPodMemoryRequest, actualPodMemoryRequest, "Incorrect pod memory request")

				for _, container := range append(actualPodSpec.Containers, actualPodSpec.InitContainers...) {
					assert.Contains(t, container.Env, corev1.EnvVar{Name: "TESTVAR", Value: "TESTVAL"})
				}
				assert.NoError(t, actualError)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange

			// Act
			actualPodSpec, actualError := InstallerPodSpec(test.clusterDeployment,
				test.provisionName,
				test.releaseImage,
				test.serviceAccountName,
				testHttpProxy,
				testHttpsProxy,
				testNoProxy,
				test.extraEnvVars)

			// Assert
			test.validate(t, actualPodSpec, actualError)
			hiveassert.AssertAllContainersHaveEnvVar(t, actualPodSpec, "HTTP_PROXY", testHttpProxy)
			hiveassert.AssertAllContainersHaveEnvVar(t, actualPodSpec, "HTTPS_PROXY", testHttpsProxy)
			hiveassert.AssertAllContainersHaveEnvVar(t, actualPodSpec, "NO_PROXY", testNoProxy)
		})
	}
}
