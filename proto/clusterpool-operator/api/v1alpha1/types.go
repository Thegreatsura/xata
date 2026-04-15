package v1alpha1

import (
	cnpgv1 "github.com/xataio/xata-cnpg/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterPoolSpec defines the desired state of a ClusterPool
type ClusterPoolSpec struct {
	// Clusters is the target number of clusters to maintain in the pool.
	// When a cluster is removed from the pool the controller will
	// create another cluster to replace it.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Required
	Clusters int32 `json:"clusters"`

	// ClusterSpec is an embedded CNPG cluster spec. All clusters in the
	// pool will use this spec.
	// +kubebuilder:validation:Required
	ClusterSpec cnpgv1.ClusterSpec `json:"clusterSpec"`
}

// ClusterPoolStatus defines the observed state of a ClusterPool
type ClusterPoolStatus struct {
	// Clusters is the current number of clusters owned by the pool
	// +optional
	Clusters int32 `json:"clusters,omitempty"`

	// ObservedGeneration reflects the generation of the most recently observed ClusterPool spec
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the latest available observations of the ClusterPool's state
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// LastError contains the last error message encountered during
	// reconciliation, if any.
	// +optional
	LastError string `json:"lastError,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:subresource:scale:specpath=.spec.clusters,statuspath=.status.clusters
// +kubebuilder:resource:shortName=cp
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Desired",type="integer",JSONPath=".spec.clusters"
// +kubebuilder:printcolumn:name="Current",type="integer",JSONPath=".status.clusters"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
type ClusterPool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterPoolSpec   `json:"spec,omitempty"`
	Status ClusterPoolStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type ClusterPoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterPool `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterPool{}, &ClusterPoolList{})
}
