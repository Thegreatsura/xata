package reconciler

import (
	"context"
	"fmt"

	apiv1 "github.com/xataio/xata-cnpg/api/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"xata/services/branch-operator/api/v1alpha1"
)

// updateXVolStatus resolves the XVol associated with the Branch and reports
// its availability via the XVolInfoAvailable condition. PrimaryXVolName is
// immutable once set: on the first successful resolution it is recorded and
// subsequent reconciles verify that the recorded XVol still exists rather than
// re-deriving the name.
//
// For parent branches, the initial resolution walks Cluster → PVC → PV → XVol
// (XVols are named after the PV they back). Child branches (XVolClone
// restore) have PrimaryXVolName set by reconcileXVolClone before any cluster
// exists, so they always take the fast path here.
func (r *BranchReconciler) updateXVolStatus(ctx context.Context, branch *v1alpha1.Branch) error {
	// PrimaryXVolName is immutable once set, so just verify the recorded XVol
	// still exists.
	if branch.Status.PrimaryXVolName != "" {
		return r.recordXVolStatus(ctx, branch, branch.Status.PrimaryXVolName)
	}

	// The XVol info is unavailable if the Branch has no associated Cluster
	if !branch.HasClusterName() {
		return r.setXVolInfoConditionToFalse(ctx, branch, v1alpha1.BranchHasNoClusterReason)
	}

	// Get the Cluster associated with the Branch
	cluster := &apiv1.Cluster{}
	err := r.Get(ctx, client.ObjectKey{
		Name:      branch.ClusterName(),
		Namespace: r.ClustersNamespace,
	}, cluster)
	if err != nil {
		return fmt.Errorf("get cluster: %w", err)
	}

	// Get the primary PVC name for the Cluster
	pvcName, err := getClusterPVC(cluster)
	if err != nil {
		return r.setXVolInfoConditionToFalse(ctx, branch, v1alpha1.ClusterPVCNotAvailableReason)
	}

	// Get the PVC for the Cluster
	pvc := &v1.PersistentVolumeClaim{}
	err = r.Get(ctx, client.ObjectKey{
		Name:      pvcName,
		Namespace: r.ClustersNamespace,
	}, pvc)
	if err != nil {
		return fmt.Errorf("get pvc %q: %w", pvcName, err)
	}

	// If the PVC does not have a bound PV there is nothing to look up
	pvName := pvc.Spec.VolumeName
	if pvName == "" {
		return r.setXVolInfoConditionToFalse(ctx, branch, v1alpha1.PVNotBoundReason)
	}

	// Record the XVol name and verify its existence
	return r.recordXVolStatus(ctx, branch, pvName)
}

// recordXVolStatus looks up an XVol by name and sets the XVolInfoAvailable
// condition based on the result. On success the name is recorded in
// PrimaryXVolName.
func (r *BranchReconciler) recordXVolStatus(ctx context.Context, branch *v1alpha1.Branch, name string) error {
	xvol := &unstructured.Unstructured{}
	xvol.SetGroupVersionKind(xvolGVK)

	// Try to get the named XVol. If it doesn't exist or the API is not found,
	// set the condition to False with an appropriate reason
	err := r.Get(ctx, client.ObjectKey{Name: name}, xvol)
	if meta.IsNoMatchError(err) {
		return r.setXVolInfoConditionToFalse(ctx, branch, v1alpha1.XVolCRDNotInstalledReason)
	}
	if apierrors.IsNotFound(err) {
		return r.setXVolInfoConditionToFalse(ctx, branch, v1alpha1.XVolNotFoundReason)
	}
	if err != nil {
		return err
	}

	// The XVol exists, record the name and set the condition to True
	branch.Status.PrimaryXVolName = name
	return r.setXVolInfoConditionToTrue(ctx, branch, v1alpha1.XVolInfoCollectedReason)
}
