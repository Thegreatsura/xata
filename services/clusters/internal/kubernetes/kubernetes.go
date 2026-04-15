package kubernetes

import (
	"fmt"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type ConfigProvider interface {
	BuildConfigFromFlags() (*rest.Config, error)
	InClusterConfig() (*rest.Config, error)
}

type DefaultConfigProvider struct {
	KubeConfigPath string
	MasterURL      string
}

func (dp *DefaultConfigProvider) BuildConfigFromFlags() (*rest.Config, error) {
	return clientcmd.BuildConfigFromFlags(dp.MasterURL, dp.KubeConfigPath)
}

func (dp *DefaultConfigProvider) InClusterConfig() (*rest.Config, error) {
	return rest.InClusterConfig()
}

func GetRestConfig(cp ConfigProvider) (*rest.Config, error) {
	restConfig, err := cp.InClusterConfig()
	if err != nil {
		restKubeConfig, errKubeconf := cp.BuildConfigFromFlags()
		if errKubeconf != nil {
			return nil, fmt.Errorf("failed to read in cluster config %w and kubeconfig: %w", err, errKubeconf)
		}
		return restKubeConfig, nil
	}
	return restConfig, nil
}
