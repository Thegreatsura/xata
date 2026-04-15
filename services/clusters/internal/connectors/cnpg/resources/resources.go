package resources

import (
	v1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	ClustersServicePrefix = "clusters-"
)

// GlobalCNPGServices creates ClusterIP services for a CNPG cluster, with the
// required Cilium annotations to make the service a global service.
func GlobalCNPGServices(clusterID, namespace string) []v1.Service {
	suffixes := []string{"-ro", "-r", "-rw"}
	services := make([]v1.Service, 0, len(suffixes))

	for _, suffix := range suffixes {
		services = append(services,
			globalCNPGService(clusterID, suffix, namespace),
		)
	}
	return services
}

// globalCNPGService creates a ClusterIP service for a CNPG cluster, with the
// required Cilium annotations to make the service a global service
func globalCNPGService(clusterID, suffix, namespace string) v1.Service {
	return v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "branch-" + clusterID + suffix,
			Namespace: namespace,
			Annotations: map[string]string{
				"service.cilium.io/global": "true",
			},
		},
		Spec: v1.ServiceSpec{
			Type: v1.ServiceTypeClusterIP,
			Ports: []v1.ServicePort{
				{
					Name:       "postgres",
					Port:       5432,
					TargetPort: intstr.FromInt(5432),
					Protocol:   v1.ProtocolTCP,
				},
			},
		},
	}
}

// GlobalPoolerService creates a selectorless service called
// branch-<clusterID>-pooler with the Cilium global annotation. On the primary
// cell this allows Cilium to route traffic to the pooler additional service on
// the secondary cell where the PgBouncer pods run.
func GlobalPoolerService(clusterID, namespace string) v1.Service {
	return v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "branch-" + clusterID + "-pooler",
			Namespace: namespace,
			Annotations: map[string]string{
				"service.cilium.io/global": "true",
			},
		},
		Spec: v1.ServiceSpec{
			Type: v1.ServiceTypeClusterIP,
			Ports: []v1.ServicePort{
				{
					Name:       "pgbouncer",
					Port:       5432,
					TargetPort: intstr.FromInt(5432),
					Protocol:   v1.ProtocolTCP,
				},
			},
		},
	}
}

// GlobalClustersService creates a service called clusters-<clusterID> with no
// selector.
func GlobalClustersService(clusterID, namespace string) v1.Service {
	return clustersService(clusterID, namespace, map[string]string{})
}

func clustersService(clusterID, namespace string, selector map[string]string) v1.Service {
	return v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ClustersServicePrefix + clusterID,
			Namespace: namespace,
			Annotations: map[string]string{
				"service.cilium.io/global": "true",
			},
		},
		Spec: v1.ServiceSpec{
			Type: v1.ServiceTypeClusterIP,
			Ports: []v1.ServicePort{
				{
					Name:       "grpc",
					Port:       5002,
					TargetPort: intstr.FromInt(5002),
					Protocol:   v1.ProtocolTCP,
				},
			},
			Selector: selector,
		},
	}
}
