package utils

import (
	"context"
	"time"

	"github.com/pkg/errors"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/internal/clientutil"
)

// InitialURL returns the API URL from the ClusterDeployment's kubeconfig secret.
// This is the URL before any APIURLOverride is applied.
func InitialURL(c client.Client, cd *hivev1.ClusterDeployment) (string, error) {
	if IsFakeCluster(cd) {
		return "https://example.com/veryfakeapi", nil
	}

	cfg, err := unadulteratedRESTConfig(c, cd)
	if err != nil {
		return "", err
	}
	return cfg.Host, nil
}

// Unreachable returns true if the cluster is marked as unreachable based on the UnreachableCondition.
// This does not attempt to connect - it only checks the cached condition state and last probe time.
func Unreachable(cd *hivev1.ClusterDeployment) (unreachable bool, lastCheck time.Time) {
	cond := FindCondition(cd.Status.Conditions, hivev1.UnreachableCondition)
	if cond == nil || cond.Status == corev1.ConditionUnknown {
		unreachable = true
		return
	}
	return cond.Status == corev1.ConditionTrue, cond.LastProbeTime.Time
}

// SetUnreachableCondition sets the UnreachableCondition based on the connection error.
// If connectionError is nil, sets ConditionFalse (cluster is reachable).
// If connectionError is not nil, sets ConditionTrue (cluster is unreachable).
func SetUnreachableCondition(cd *hivev1.ClusterDeployment, connectionError error) (changed bool) {
	status := corev1.ConditionFalse
	reason := "ClusterReachable"
	message := "cluster is reachable"
	// This needs to always update so that the probe time is updated. The probe time is used to determine when to
	// perform the next connectivity check.
	updateCheck := UpdateConditionAlways
	if connectionError != nil {
		status = corev1.ConditionTrue
		reason = "ErrorConnectingToCluster"
		message = ErrorScrub(connectionError)
		updateCheck = UpdateConditionIfReasonOrMessageChange
	}
	cd.Status.Conditions, changed = SetClusterDeploymentConditionWithChangeCheck(
		cd.Status.Conditions,
		hivev1.UnreachableCondition,
		status,
		reason,
		message,
		updateCheck,
	)
	return
}

// unadulteratedRESTConfig returns the REST config directly from the kubeconfig secret
// without any modifications (no URL overrides, no metrics wrappers).
func unadulteratedRESTConfig(c client.Client, cd *hivev1.ClusterDeployment) (*rest.Config, error) {
	kubeconfigSecret := &corev1.Secret{}
	if err := c.Get(
		context.Background(),
		client.ObjectKey{Namespace: cd.Namespace, Name: cd.Spec.ClusterMetadata.AdminKubeconfigSecretRef.Name},
		kubeconfigSecret,
	); err != nil {
		return nil, errors.Wrap(err, "could not get admin kubeconfig secret")
	}
	return clientutil.RestConfigFromSecret(kubeconfigSecret, false)
}
