package fieldmanager

import (
	"testing"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
)

func TestFieldManagerName(t *testing.T) {
	tests := []struct {
		name           string
		controllerName hivev1.ControllerName
		expected       string
	}{
		{
			name:           "clustersync controller",
			controllerName: hivev1.ClustersyncControllerName,
			expected:       "hive-clustersync",
		},
		{
			name:           "clusterdeployment controller",
			controllerName: hivev1.ClusterDeploymentControllerName,
			expected:       "hive-clusterDeployment",
		},
		{
			name:           "unreachable controller",
			controllerName: hivev1.UnreachableControllerName,
			expected:       "hive-unreachable",
		},
		{
			name:           "hibernation controller",
			controllerName: hivev1.HibernationControllerName,
			expected:       "hive-hibernation",
		},
		{
			name:           "controlplanecerts controller",
			controllerName: hivev1.ControlPlaneCertsControllerName,
			expected:       "hive-controlPlaneCerts",
		},
		{
			name:           "remoteingress controller",
			controllerName: hivev1.RemoteIngressControllerName,
			expected:       "hive-remoteingress",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FieldManagerName(tt.controllerName)
			if got != tt.expected {
				t.Errorf("FieldManagerName() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestFieldManagerNameLegacy(t *testing.T) {
	tests := []struct {
		name           string
		controllerName hivev1.ControllerName
		version        int
		expected       string
	}{
		{
			name:           "version 1 - controller utils",
			controllerName: hivev1.ClustersyncControllerName,
			version:        1,
			expected:       "hive1-clustersync",
		},
		{
			name:           "version 2 - remoteclient",
			controllerName: hivev1.ClustersyncControllerName,
			version:        2,
			expected:       "hive2-clustersync",
		},
		{
			name:           "version 4 - resource helper Create",
			controllerName: hivev1.ClustersyncControllerName,
			version:        4,
			expected:       "hive4-clustersync",
		},
		{
			name:           "version 5 - resource helper CreateOrUpdate",
			controllerName: hivev1.ClustersyncControllerName,
			version:        5,
			expected:       "hive5-clustersync",
		},
		{
			name:           "version 6 - resource helper Apply",
			controllerName: hivev1.ClustersyncControllerName,
			version:        6,
			expected:       "hive6-clustersync",
		},
		{
			name:           "version 7 - resource helper Patch",
			controllerName: hivev1.ClustersyncControllerName,
			version:        7,
			expected:       "hive7-clustersync",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FieldManagerNameLegacy(tt.controllerName, tt.version)
			if got != tt.expected {
				t.Errorf("FieldManagerNameLegacy() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestFieldManagerNameFormat(t *testing.T) {
	// Verify the format is consistent and doesn't contain unexpected characters
	controllerName := hivev1.ControllerName("test-controller")
	result := FieldManagerName(controllerName)

	expectedPrefix := "hive-"
	if len(result) <= len(expectedPrefix) {
		t.Fatalf("FieldManagerName() returned %q, which is too short", result)
	}

	if result[:len(expectedPrefix)] != expectedPrefix {
		t.Errorf("FieldManagerName() = %q, should start with %q", result, expectedPrefix)
	}

	if result[len(expectedPrefix):] != string(controllerName) {
		t.Errorf("FieldManagerName() = %q, controller name portion should be %q", result, string(controllerName))
	}
}

func TestFieldManagerNameConsistency(t *testing.T) {
	// Verify that the same controller name always produces the same field manager name
	controllerName := hivev1.ClustersyncControllerName

	result1 := FieldManagerName(controllerName)
	result2 := FieldManagerName(controllerName)

	if result1 != result2 {
		t.Errorf("FieldManagerName() not consistent: got %q and %q", result1, result2)
	}
}
