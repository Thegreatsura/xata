package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// PasswordSyncMode defines how CNPG password synchronization is handled
// during the wakeup process
type PasswordSyncMode string

const (
	PasswordSyncModeWait PasswordSyncMode = "Wait"
	PasswordSyncModeSkip PasswordSyncMode = "Skip"
)

// WakeupRequestSpec defines the desired state of a WakeupRequest
type WakeupRequestSpec struct {
	// BranchName is the name of the Branch resource to wake up
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="branchName is immutable"
	BranchName string `json:"branchName"`

	// XVolName is the name of the XVol resource to use for waking up the Branch.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="xVolName is immutable"
	XVolName string `json:"xVolName"`

	// PasswordSync defines how to handle CNPG password synchronization during
	// the wakeup process. If set to "Wait", the wakeup process will wait for CNPG
	// to apply the password for the 'xata' role to the cluster before assigning
	// the cluster to the Branch. If set to "Skip", the wakeup process will not
	// wait for CNPG to apply the password
	// +kubebuilder:validation:Enum=Wait;Skip
	// +kubebuilder:default=Wait
	// +optional
	PasswordSync PasswordSyncMode `json:"passwordSync,omitempty"`
}

// WakeupRequestStatus defines the observed state of a WakeupRequest
type WakeupRequestStatus struct {
	// ObservedGeneration reflects the generation of the most recently observed spec
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions represent the latest available observations of the WakeupRequest's state
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// LastError contains the last error message encountered during reconciliation
	// +optional
	LastError string `json:"lastError,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=wur
// +kubebuilder:printcolumn:name="Branch",type="string",JSONPath=".spec.branchName"
// +kubebuilder:printcolumn:name="Succeeded",type="string",JSONPath=".status.conditions[?(@.type=='Succeeded')].status"
// +kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.conditions[?(@.type=='Succeeded')].message"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// WakeupRequest is the Schema for the wakeuprequests API
type WakeupRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WakeupRequestSpec   `json:"spec,omitempty"`
	Status WakeupRequestStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// WakeupRequestList contains a list of WakeupRequest
type WakeupRequestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WakeupRequest `json:"items"`
}

func init() {
	SchemeBuilder.Register(&WakeupRequest{}, &WakeupRequestList{})
}
