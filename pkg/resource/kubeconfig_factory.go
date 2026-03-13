package resource

import (
	"sync"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

func (r *helper) getKubeconfigFactory(namespace string) (cmdutil.Factory, error) {
	config, err := clientcmd.Load(r.kubeconfig)
	if err != nil {
		r.logger.WithError(err).Error("an error occurred loading the kubeconfig")
		return nil, err
	}
	overrides := &clientcmd.ConfigOverrides{}
	if len(namespace) > 0 {
		overrides.Context.Namespace = namespace
	}
	clientConfig := clientcmd.NewNonInteractiveClientConfig(*config, "", overrides, nil)
	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, err
	}
	if r.metricsEnabled {
		controllerutils.AddControllerMetricsTransportWrapper(restConfig, r.controllerName, r.remote)
	}

	r.logger.WithField("cache-dir", r.cacheDir).Debug("creating cmdutil.Factory from client config and cache directory")
	f := cmdutil.NewFactory(&kubeconfigClientGetter{
		clientConfig:   clientConfig,
		cacheDir:       r.cacheDir,
		controllerName: r.controllerName,
		metricsEnabled: r.metricsEnabled,
		restConfig:     restConfig,
	})
	return f, nil
}

type kubeconfigClientGetter struct {
	clientConfig   clientcmd.ClientConfig
	cacheDir       string
	controllerName hivev1.ControllerName
	metricsEnabled bool
	restConfig     *rest.Config
	// Internal caching to prevent resource leaks
	discoveryClient discovery.CachedDiscoveryInterface
	restMapper      meta.RESTMapper
	mu              sync.Mutex
}

// ToRESTConfig returns restconfig
func (r *kubeconfigClientGetter) ToRESTConfig() (*rest.Config, error) {
	return r.restConfig, nil
}

// ToDiscoveryClient returns discovery client
func (r *kubeconfigClientGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.discoveryClient != nil {
		return r.discoveryClient, nil
	}

	config, err := r.ToRESTConfig()
	if err != nil {
		return nil, err
	}
	r.discoveryClient, err = getDiscoveryClient(config, r.cacheDir)
	return r.discoveryClient, err
}

// ToRESTMapper returns a restmapper
func (r *kubeconfigClientGetter) ToRESTMapper() (meta.RESTMapper, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.restMapper != nil {
		return r.restMapper, nil
	}

	discoveryClient, err := r.ToDiscoveryClient()
	if err != nil {
		return nil, err
	}

	mapper := restmapper.NewDeferredDiscoveryRESTMapper(discoveryClient)
	r.restMapper = restmapper.NewShortcutExpander(
		mapper, discoveryClient,
		// TODO: Plumb logger through kubeconfigClientGetter and log warnings here
		func(string) {})
	return r.restMapper, nil
}

// ToRawKubeConfigLoader return kubeconfig loader as-is
func (r *kubeconfigClientGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	return r.clientConfig
}
