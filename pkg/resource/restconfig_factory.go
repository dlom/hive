package resource

import (
	"sync"

	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

func (r *helper) getRESTConfigFactory(namespace string) (cmdutil.Factory, error) {
	cfg := r.restConfig
	if r.metricsEnabled {
		cfg = rest.CopyConfig(r.restConfig)
		controllerutils.AddControllerMetricsTransportWrapper(cfg, r.controllerName, false)
	}
	r.logger.WithField("cache-dir", r.cacheDir).Debug("creating cmdutil.Factory from REST client config and cache directory")
	f := cmdutil.NewFactory(&restConfigClientGetter{restConfig: cfg, cacheDir: r.cacheDir, namespace: namespace})
	return f, nil
}

type restConfigClientGetter struct {
	restConfig *rest.Config
	cacheDir   string
	namespace  string
	// Internal caching to prevent resource leaks
	discoveryClient discovery.CachedDiscoveryInterface
	restMapper      meta.RESTMapper
	mu              sync.Mutex
}

// ToRESTConfig returns restconfig
func (r *restConfigClientGetter) ToRESTConfig() (*rest.Config, error) {
	return r.restConfig, nil
}

// ToDiscoveryClient returns discovery client
func (r *restConfigClientGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.discoveryClient != nil {
		return r.discoveryClient, nil
	}

	config := rest.CopyConfig(r.restConfig)
	var err error
	r.discoveryClient, err = getDiscoveryClient(config, r.cacheDir)
	return r.discoveryClient, err
}

// ToRESTMapper returns a restmapper
func (r *restConfigClientGetter) ToRESTMapper() (meta.RESTMapper, error) {
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
		// TODO: Plumb logger through restconfigClientGetter and log warnings here
		func(string) {})
	return r.restMapper, nil
}

// ToRawKubeConfigLoader return kubeconfig loader as-is
func (r *restConfigClientGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	cfg := GenerateClientConfigFromRESTConfig("default", r.restConfig)
	overrides := &clientcmd.ConfigOverrides{}
	if len(r.namespace) > 0 {
		overrides.Context.Namespace = r.namespace
	}
	return clientcmd.NewNonInteractiveClientConfig(*cfg, "", overrides, nil)
}
