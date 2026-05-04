package openebs

import (
	"context"
	"fmt"

	"xata/services/clusters/internal/kubernetes"

	openebsv1beta3 "github.com/openebs/openebs-e2e/common/custom_resources/api/types/v1beta3"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//go:generate go run github.com/vektra/mockery/v3 --with-expecter --name Connector

type Connector interface {
	AvailableSpaceBytes(ctx context.Context) (*uint64, error)
}

type DefaultConnector struct {
	KubernetesClient client.Client
	Namespace        string
}

type PoolStatus string

const (
	defaultNamespace            = "openebs"
	PoolStatusOnline PoolStatus = "Online"
)

// NewConnector creates a new OpenEBS connector used to work with OpenEBS
// resources
func NewConnector(kubeConfig, namespace string) (Connector, error) {
	dcp := &kubernetes.DefaultConfigProvider{
		KubeConfigPath: kubeConfig,
		MasterURL:      "",
	}

	cfg, err := kubernetes.GetRestConfig(dcp)
	if err != nil {
		return nil, fmt.Errorf("get restconfig %w", err)
	}

	dc, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("create discovery client: %w", err)
	}

	scheme := runtime.NewScheme()
	client, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("create clientset %w", err)
	}

	return NewConnectorWithClients(client, dc, namespace)
}

// AvailableSpaceBytes returns the total available space across all online
// DiskPool resources
func (dc *DefaultConnector) AvailableSpaceBytes(ctx context.Context) (*uint64, error) {
	list := &openebsv1beta3.DiskPoolList{}
	namespace := dc.Namespace
	if namespace == "" {
		namespace = defaultNamespace
	}

	err := dc.KubernetesClient.List(ctx, list, &client.ListOptions{Namespace: namespace})
	if err != nil {
		return nil, fmt.Errorf("list diskpools: %w", err)
	}

	// Calculate available space across all online pools
	available := uint64(0)
	for _, dp := range list.Items {
		if dp.Status.PoolStatus == string(PoolStatusOnline) {
			available += dp.Status.Available
		}
	}

	return new(available), nil
}

// NewConnectorWithClients creates a new OpenEBS connector using the provided
// Kubernetes client and discovery client.
func NewConnectorWithClients(client client.Client, dc discovery.DiscoveryInterface, namespace string) (Connector, error) {
	if namespace == "" {
		namespace = defaultNamespace
	}

	diskPoolGVR := schema.GroupVersionResource{
		Group:    "openebs.io",
		Version:  "v1beta3",
		Resource: "diskpools",
	}

	// Check if the DiskPoool CRD exists
	exists, err := discovery.IsResourceEnabled(dc, diskPoolGVR)
	if err != nil {
		return nil, fmt.Errorf("check if diskpool resource exists: %w", err)
	}

	// If the DiskPool CRD does not exist, return a NoopConnector
	if !exists {
		return &NoopConnector{}, nil
	}

	// Configure the client's scheme to include OpenEBS types
	scheme := client.Scheme()
	if err = clientgoscheme.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("add to scheme clientgo: %w", err)
	}
	if err = openebsv1beta3.PoolAddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("add to scheme openebs: %w", err)
	}

	return &DefaultConnector{KubernetesClient: client, Namespace: namespace}, nil
}
