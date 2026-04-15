package resources

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	v1 "k8s.io/api/core/v1"
)

func TestGlobalCNPGService(t *testing.T) {
	tests := []struct {
		name      string
		id        string
		namespace string
		want      []v1.Service
	}{
		{
			name:      "basic global services",
			id:        "test-cluster",
			namespace: "default",
			want: []v1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "branch-test-cluster-rw",
						Namespace: "default",
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
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "branch-test-cluster-r",
						Namespace: "default",
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
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "branch-test-cluster-ro",
						Namespace: "default",
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
				},
			},
		},
		{
			name:      "global services in different namespace",
			id:        "test-cluster-2",
			namespace: "another-namespace",
			want: []v1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "branch-test-cluster-2-rw",
						Namespace: "another-namespace",
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
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "branch-test-cluster-2-r",
						Namespace: "another-namespace",
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
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "branch-test-cluster-2-ro",
						Namespace: "another-namespace",
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
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GlobalCNPGServices(tt.id, tt.namespace)
			assert.ElementsMatch(t, tt.want, got)
		})
	}
}

func TestGlobalPoolerService(t *testing.T) {
	tests := []struct {
		name      string
		id        string
		namespace string
		want      v1.Service
	}{
		{
			name:      "basic global pooler service",
			id:        "test-cluster",
			namespace: "default",
			want: v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "branch-test-cluster-pooler",
					Namespace: "default",
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
			},
		},
		{
			name:      "global pooler service in different namespace",
			id:        "test-cluster-2",
			namespace: "another-namespace",
			want: v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "branch-test-cluster-2-pooler",
					Namespace: "another-namespace",
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
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GlobalPoolerService(tt.id, tt.namespace)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GlobalPoolerService() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGlobalClustersService(t *testing.T) {
	tests := []struct {
		name      string
		id        string
		namespace string
		want      v1.Service
	}{
		{
			name:      "basic global clusters service",
			id:        "branchid123",
			namespace: "default",
			want: v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ClustersServicePrefix + "branchid123",
					Namespace: "default",
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
					Selector: map[string]string{},
				},
			},
		},
		{
			name:      "global clusters service in different namespace",
			id:        "branchid456",
			namespace: "another-namespace",
			want: v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ClustersServicePrefix + "branchid456",
					Namespace: "another-namespace",
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
					Selector: map[string]string{},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GlobalClustersService(tt.id, tt.namespace)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GlobalClustersService() = %v, want %v", got, tt.want)
			}
		})
	}
}
