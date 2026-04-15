package wakeup

import (
	"context"
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"xata/services/branch-operator/api/v1alpha1"
)

// ConditionError indicates that the Succeeded condition on a WakeupRequest
// should be updated in response to the error. Terminal errors set
// Succeeded=False; non-terminal errors set Succeeded=Unknown.
type ConditionError struct {
	ConditionReason string
	Err             error
	Terminal        bool
}

func (e *ConditionError) Error() string {
	return fmt.Sprintf("%s: %s", e.ConditionReason, e.Err.Error())
}

func (e *ConditionError) Unwrap() error {
	return e.Err
}

// ignoreTerminal returns nil if the error is a terminal ConditionError,
// Non-terminal errors are returned as-is
func ignoreTerminal(err error) error {
	if condErr, ok := errors.AsType[*ConditionError](err); ok && condErr.Terminal {
		return nil
	}
	return err
}

// ensureStatusConditions initializes the status conditions on the
// WakeupRequest if they are not already set.
func (r *WakeupReconciler) ensureStatusConditions(ctx context.Context, wr *v1alpha1.WakeupRequest) error {
	if len(wr.Status.Conditions) != 0 {
		return nil
	}

	wr.Status.Conditions = make([]metav1.Condition, 0)
	meta.SetStatusCondition(&wr.Status.Conditions, metav1.Condition{
		Type:               v1alpha1.WakeupSucceededConditionType,
		Status:             metav1.ConditionUnknown,
		ObservedGeneration: wr.Generation,
		Reason:             v1alpha1.WakeupAwaitingReconciliationReason,
		Message:            v1alpha1.WakeupConditionMessages[v1alpha1.WakeupAwaitingReconciliationReason],
	})

	return r.Status().Update(ctx, wr)
}

// setStatusConditionFromError updates the Succeeded condition on the
// WakeupRequest based on the provided error. Terminal ConditionErrors set
// Succeeded=False; non-terminal ConditionErrors and generic errors set
// Succeeded=Unknown.
func (r *WakeupReconciler) setStatusConditionFromError(ctx context.Context, wr *v1alpha1.WakeupRequest, err error) error {
	if err == nil {
		return nil
	}

	if condErr, ok := errors.AsType[*ConditionError](err); ok {
		status := metav1.ConditionUnknown
		if condErr.Terminal {
			status = metav1.ConditionFalse
		}
		return r.setSucceededCondition(ctx, wr, status, condErr.ConditionReason)
	}

	return r.setSucceededCondition(ctx, wr, metav1.ConditionUnknown, v1alpha1.WakeupReconciliationFailedReason)
}

// setSucceededCondition sets the Succeeded condition on the WakeupRequest
func (r *WakeupReconciler) setSucceededCondition(ctx context.Context, wr *v1alpha1.WakeupRequest, status metav1.ConditionStatus, reason string) error {
	meta.SetStatusCondition(&wr.Status.Conditions, metav1.Condition{
		Type:               v1alpha1.WakeupSucceededConditionType,
		Status:             status,
		Reason:             reason,
		Message:            v1alpha1.WakeupConditionMessages[reason],
		ObservedGeneration: wr.Generation,
	})

	return r.Status().Update(ctx, wr)
}

// isWakeupSucceeded checks if the WakeupRequest has Succeeded=True
func (r *WakeupReconciler) isWakeupSucceeded(wr *v1alpha1.WakeupRequest) bool {
	cond := meta.FindStatusCondition(wr.Status.Conditions, v1alpha1.WakeupSucceededConditionType)
	return cond != nil && cond.Status == metav1.ConditionTrue
}

// isWakeupFailed checks if the WakeupRequest has Succeeded=False (terminal)
func (r *WakeupReconciler) isWakeupFailed(wr *v1alpha1.WakeupRequest) bool {
	cond := meta.FindStatusCondition(wr.Status.Conditions, v1alpha1.WakeupSucceededConditionType)
	return cond != nil && cond.Status == metav1.ConditionFalse
}

// setLastErrorStatus sets the LastError field in the WakeupRequest status
func (r *WakeupReconciler) setLastErrorStatus(ctx context.Context, wr *v1alpha1.WakeupRequest, err error) error {
	var msg string
	if err != nil {
		msg = err.Error()
	}

	wr.Status.LastError = msg
	return r.Status().Update(ctx, wr)
}
