package remoteclient

//go:generate mockgen -source=./remoteclient.go -destination=./mock/remoteclient_generated.go -package=mock

import (
	"context"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/internal/clientutil"
	"github.com/openshift/hive/pkg/controller/utils"
)

// Builder is used to build API clients to the remote cluster with context support and client caching.
type Builder interface {
	// BuildWithContext creates a controller-runtime client with context support.
	// If caching is enabled, returns a cached client on subsequent calls.
	BuildWithContext(ctx context.Context) (client.Client, error)

	// BuildKubeClientWithContext creates a typed Kubernetes client with context support.
	BuildKubeClientWithContext(ctx context.Context) (kubeclient.Interface, error)

	// RESTConfigWithContext returns the REST config with context support.
	RESTConfigWithContext(ctx context.Context) (*rest.Config, error)

	// UsePrimaryAPIURL returns a new builder configured to use the primary API URL.
	// If there is an API URL override, that is the primary. Otherwise, the kubeconfig URL is primary.
	UsePrimaryAPIURL() Builder

	// UseSecondaryAPIURL returns a new builder configured to use the secondary API URL.
	// If there is an API URL override, the kubeconfig URL is secondary.
	UseSecondaryAPIURL() Builder
}

// ConnectToRemoteCluster connects to a remote cluster using the specified builder.
// If the ClusterDeployment is marked as unreachable, then no connection will be made.
// If there are problems connecting, then the specified clusterdeployment will be marked as unreachable.
func ConnectToRemoteCluster(
	cd *hivev1.ClusterDeployment,
	remoteClientBuilder Builder,
	localClient client.Client,
	logger log.FieldLogger,
) (remoteClient client.Client, unreachable, requeue bool) {
	var rawRemoteClient any
	rawRemoteClient, unreachable, requeue = connectToRemoteCluster(
		cd,
		remoteClientBuilder,
		localClient,
		logger,
		func(builder Builder) (any, error) { return builder.BuildWithContext(context.Background()) },
	)
	if unreachable {
		return
	}
	remoteClient = rawRemoteClient.(client.Client)
	return
}

func connectToRemoteCluster(
	cd *hivev1.ClusterDeployment,
	remoteClientBuilder Builder,
	localClient client.Client,
	logger log.FieldLogger,
	buildFunc func(builder Builder) (any, error),
) (remoteClient any, unreachable, requeue bool) {
	if u, _ := Unreachable(cd); u {
		logger.Debug("skipping cluster with unreachable condition")
		unreachable = true
		return
	}
	var err error
	remoteClient, err = buildFunc(remoteClientBuilder)
	if err == nil {
		return
	}
	unreachable = true
	logger.WithError(err).Info("remote cluster is unreachable")
	SetUnreachableCondition(cd, err)
	if err := localClient.Status().Update(context.Background(), cd); err != nil {
		logger.WithError(err).Log(utils.LogLevel(err), "could not update clusterdeployment with unreachable condition")
		requeue = true
	}
	return
}

// InitialURL returns the initial API URL for the ClusterDeployment.
func InitialURL(c client.Client, cd *hivev1.ClusterDeployment) (string, error) {

	if utils.IsFakeCluster(cd) {
		return "https://example.com/veryfakeapi", nil
	}

	cfg, err := unadulteratedRESTConfig(c, cd)
	if err != nil {
		return "", err
	}
	return cfg.Host, nil
}

// Unreachable returns true if Hive has not been able to reach the remote cluster.
// Note that this function will not attempt to reach the remote cluster. It only checks the current conditions on
// the ClusterDeployment to determine if the remote cluster is reachable.
func Unreachable(cd *hivev1.ClusterDeployment) (unreachable bool, lastCheck time.Time) {
	cond := utils.FindCondition(cd.Status.Conditions, hivev1.UnreachableCondition)
	if cond == nil || cond.Status == corev1.ConditionUnknown {
		unreachable = true
		return
	}
	return cond.Status == corev1.ConditionTrue, cond.LastProbeTime.Time
}

// IsPrimaryURLActive returns true if the remote cluster is reachable via the primary API URL.
func IsPrimaryURLActive(cd *hivev1.ClusterDeployment) bool {
	if cd.Spec.ControlPlaneConfig.APIURLOverride == "" {
		return true
	}
	cond := utils.FindCondition(cd.Status.Conditions, hivev1.ActiveAPIURLOverrideCondition)
	return cond != nil && cond.Status == corev1.ConditionTrue
}

// SetUnreachableCondition sets the Unreachable condition on the ClusterDeployment based on the specified error
// encountered when attempting to connect to the remote cluster.
func SetUnreachableCondition(cd *hivev1.ClusterDeployment, connectionError error) (changed bool) {
	status := corev1.ConditionFalse
	reason := "ClusterReachable"
	message := "cluster is reachable"
	// This needs to always update so that the probe time is updated. The probe time is used to determine when to
	// perform the next connectivity check.
	updateCheck := utils.UpdateConditionAlways
	if connectionError != nil {
		status = corev1.ConditionTrue
		reason = "ErrorConnectingToCluster"
		message = utils.ErrorScrub(connectionError)
		updateCheck = utils.UpdateConditionIfReasonOrMessageChange
	}
	cd.Status.Conditions, changed = utils.SetClusterDeploymentConditionWithChangeCheck(
		cd.Status.Conditions,
		hivev1.UnreachableCondition,
		status,
		reason,
		message,
		updateCheck,
	)
	return
}

func unadulteratedRESTConfig(c client.Client, cd *hivev1.ClusterDeployment) (*rest.Config, error) {
	kubeconfigSecret := &corev1.Secret{}
	if err := c.Get(
		context.Background(),
		// HIVE-2485 ✓
		client.ObjectKey{Namespace: cd.Namespace, Name: cd.Spec.ClusterMetadata.AdminKubeconfigSecretRef.Name},
		kubeconfigSecret,
	); err != nil {
		return nil, errors.Wrap(err, "could not get admin kubeconfig secret")
	}
	return clientutil.RestConfigFromSecret(kubeconfigSecret, false)
}
