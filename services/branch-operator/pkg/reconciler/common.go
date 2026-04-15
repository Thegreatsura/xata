package reconciler

import (
	"maps"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"xata/services/branch-operator/api/v1alpha1"
)

const (
	kubeTrue  = "true"
	kubeFalse = "false"
)

// ensureLabels adds the canonical set of labels for resources managed by the
// operator to the given ObjectMeta, merging in any user-provided labels from
// InheritedMetadata.
func ensureLabels(m *metav1.ObjectMeta, inheritedMetadata *v1alpha1.InheritedMetadata) {
	if m.Labels == nil {
		m.Labels = make(map[string]string)
	}

	// Apply user labels
	if inheritedMetadata != nil && inheritedMetadata.Labels != nil {
		maps.Copy(m.Labels, inheritedMetadata.Labels)
	}

	// Apply operator labels
	m.Labels["app.kubernetes.io/managed-by"] = OperatorName
}
