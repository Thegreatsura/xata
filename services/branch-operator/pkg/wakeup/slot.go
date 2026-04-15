package wakeup

import (
	"context"
	"fmt"
	"strings"

	apiv1 "github.com/xataio/xata-cnpg/api/v1"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"xata/services/branch-operator/api/v1alpha1"
)

// getPersistentVolume returns the PV backing the Cluster's primary instance by
// resolving the Cluster's healthy PVC and following its bound PV reference.
func (r *WakeupReconciler) getPersistentVolume(ctx context.Context, cluster *apiv1.Cluster) (*v1.PersistentVolume, error) {
	// Ensure the Cluster has at least one healthy PVC
	if len(cluster.Status.HealthyPVC) == 0 {
		return nil, slotConditionError(fmt.Errorf("no healthy PVCs found for cluster %q", cluster.Name))
	}

	// Get the PVC
	pvcName := cluster.Status.HealthyPVC[0]
	pvc := &v1.PersistentVolumeClaim{}
	err := r.Get(ctx, client.ObjectKey{Name: pvcName, Namespace: cluster.Namespace}, pvc)
	if err != nil {
		return nil, fmt.Errorf("get pvc %q: %w", pvcName, err)
	}

	// Read the PV name from the PVC
	pvName := pvc.Spec.VolumeName
	if pvName == "" {
		return nil, slotConditionError(fmt.Errorf("PVC %q is not bound to a PV", pvcName))
	}

	// Get the PV
	pv := &v1.PersistentVolume{}
	err = r.Get(ctx, client.ObjectKey{Name: pvName}, pv)
	if err != nil {
		return nil, fmt.Errorf("get pv %q: %w", pvName, err)
	}

	return pv, nil
}

// getSlotID extracts the CSI volume handle from the given PV. The volume
// handle is used as the slot ID for the cluster's storage.
func getSlotID(pv *v1.PersistentVolume) (string, error) {
	// Ensure the PV has a CSI volume source
	if pv.Spec.CSI == nil {
		return "", slotConditionError(fmt.Errorf("PV %q has no CSI volume source", pv.Name))
	}

	// Extract the CSI volume handle from the PV spec
	volumeHandle := pv.Spec.CSI.VolumeHandle
	if volumeHandle == "" {
		return "", slotConditionError(fmt.Errorf("PV %q has no CSI volume handle", pv.Name))
	}

	// Strip the "slot/" prefix from the volume handle to get the slot ID
	slotID, found := strings.CutPrefix(volumeHandle, "slot/")
	if !found {
		return "", slotConditionError(fmt.Errorf("PV %q has bad volume handle format %q", pv.Name, volumeHandle))
	}

	return slotID, nil
}

// slotConditionError constructs a terminal ConditionError for errors
// encountered when resolving the slot ID from the Cluster's primary PVC.
func slotConditionError(err error) *ConditionError {
	return &ConditionError{
		ConditionReason: v1alpha1.SlotIDNotAvailableReason,
		Err:             err,
		Terminal:        true,
	}
}
