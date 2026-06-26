package wakeup

import (
	"errors"

	corev1 "k8s.io/api/core/v1"

	"xata/services/branch-operator/api/v1alpha1"
)

// recordFailureEvent records an event on the WakeupRequest resource if there
// was an error during reconciliation. It determines the event type and reason
// based on the error, and avoids recording duplicate events for the same
// error; an event is only recorded if the error message differs from the last
// error recorded in status
func (r *WakeupReconciler) recordFailureEvent(wur *v1alpha1.WakeupRequest, err error) {
	// Don't record an event if there was no error
	if err == nil {
		return
	}

	// Avoid recording duplicate events for the same error; only record an event
	// if the error message has changed since the last recorded error
	if err.Error() == wur.Status.LastError {
		return
	}

	// Default to a warning event with the generic reconciliation failure reason
	eventType := corev1.EventTypeWarning
	reason := v1alpha1.WakeupReconciliationFailedReason

	// If the error is a ConditionError extract the condition reason and
	// terminality
	if condErr, ok := errors.AsType[*ConditionError](err); ok {
		reason = condErr.ConditionReason
		if !condErr.Terminal {
			eventType = corev1.EventTypeNormal
		}
	}

	// Record the event
	r.Recorder.Eventf(wur, nil, eventType, reason, "Reconcile", "%s", err)
}
