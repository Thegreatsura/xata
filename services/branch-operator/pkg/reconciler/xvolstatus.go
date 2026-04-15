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

// updateXVolStatus resolves the XVol backing the branch's primary instance and
// records its name in the Branch status. On any failure the PrimaryXVolName
// field is left untouched and the XVolInfoAvailable condition is set to False
// with a reason indicating why. Transient errors (failed API server calls) are
// returned to the caller so the reconciler requeues, while expected states (no
// cluster, CRD missing, PV unbound) return nil after setting the condition.
func (r *BranchReconciler) updateXVolStatus(ctx context.Context, branch *v1alpha1.Branch) error {
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

	// Look up the XVol named after the PV
	xvol := &unstructured.Unstructured{}
	xvol.SetGroupVersionKind(xvolGVK)
	err = r.Get(ctx, client.ObjectKey{Name: pvName}, xvol)
	if err != nil {
		if meta.IsNoMatchError(err) {
			return r.setXVolInfoConditionToFalse(ctx, branch, v1alpha1.XVolCRDNotInstalledReason)
		}
		if apierrors.IsNotFound(err) {
			return r.setXVolInfoConditionToFalse(ctx, branch, v1alpha1.XVolNotFoundReason)
		}
		return fmt.Errorf("get xvol %q: %w", pvName, err)
	}

	// Record the XVol name in the Branch status and update the status condition
	// to True
	branch.Status.PrimaryXVolName = xvol.GetName()
	return r.setXVolInfoConditionToTrue(ctx, branch, v1alpha1.XVolInfoCollectedReason)
}
