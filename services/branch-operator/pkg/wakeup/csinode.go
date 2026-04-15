package wakeup

import (
	"context"
	"fmt"

	apiv1 "github.com/xataio/xata-cnpg/api/v1"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"xata/services/branch-operator/api/v1alpha1"
)

// getCSINodePod returns the CSI node plugin pod running on the same node as
// the cluster's primary pod.
func (r *WakeupReconciler) getCSINodePod(ctx context.Context, cluster *apiv1.Cluster) (*v1.Pod, error) {
	// Get the primary pod name from the Cluster status
	primaryPodName := cluster.Status.TargetPrimary
	if primaryPodName == "" {
		return nil, fmt.Errorf("cluster %q has no target primary", cluster.Name)
	}

	// Fetch the primary pod
	primaryPod := &v1.Pod{}
	err := r.Get(ctx, client.ObjectKey{
		Name:      primaryPodName,
		Namespace: cluster.Namespace,
	}, primaryPod)
	if err != nil {
		return nil, fmt.Errorf("get primary pod %q: %w", primaryPodName, err)
	}

	// Get the node the primary pod is running on
	nodeName := primaryPod.Spec.NodeName
	if nodeName == "" {
		return nil, fmt.Errorf("primary pod %q is not scheduled to a node", primaryPodName)
	}

	// List CSI node plugin pods
	var podList v1.PodList
	err = r.List(ctx, &podList,
		client.InNamespace(r.CSINodeNamespace),
		client.MatchingLabels{
			"app.kubernetes.io/component": "csi-node",
			"app.kubernetes.io/name":      "xatastor-csi",
		},
	)
	if err != nil {
		return nil, fmt.Errorf("list CSI node pods: %w", err)
	}

	// Find the CSI node pod running on the same node as the primary pod
	for i := range podList.Items {
		if podList.Items[i].Spec.NodeName == nodeName {
			return &podList.Items[i], nil
		}
	}

	// No CSI node pod found on the same node as the primary pod - return a
	// terminal error
	return nil, &ConditionError{
		ConditionReason: v1alpha1.CSINodePodNotFoundReason,
		Err:             fmt.Errorf("no CSI node pod found on node %q", nodeName),
		Terminal:        true,
	}
}
