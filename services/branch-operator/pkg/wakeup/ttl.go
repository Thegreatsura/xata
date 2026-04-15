package wakeup

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"xata/services/branch-operator/api/v1alpha1"
)

// deleteAfterTTL deletes a succeeded WakeupRequest if it has been Succeeded
// for longer than the configured TTL. Otherwise, it requeues the WakeupRequest
// to be reconciled again after the remaining TTL duration
func (r *WakeupReconciler) deleteAfterTTL(ctx context.Context, wr *v1alpha1.WakeupRequest) (ctrl.Result, error) {
	// Get the Succeeded condition from the WakeupRequest status
	cond := meta.FindStatusCondition(wr.Status.Conditions, v1alpha1.WakeupSucceededConditionType)
	if cond == nil {
		return ctrl.Result{}, nil
	}

	// Calculate the time since the condition transitioned to True
	elapsed := time.Since(cond.LastTransitionTime.Time)
	remaining := r.WakeupRequestTTL - elapsed

	// Requeue if the TTL has not yet expired
	if remaining > 0 {
		return ctrl.Result{RequeueAfter: remaining}, nil
	}

	// TTL has expired, delete the WakeupRequest
	if err := r.Delete(ctx, wr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	return ctrl.Result{}, nil
}
