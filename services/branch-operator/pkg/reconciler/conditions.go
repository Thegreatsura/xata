package reconciler

import (
	"context"
	"errors"
	"fmt"

	"xata/services/branch-operator/api/v1alpha1"
	v1alpha1ac "xata/services/branch-operator/applyconfiguration/api/v1alpha1"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metav1ac "k8s.io/client-go/applyconfigurations/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ConditionError indicates that a specific condition on a Branch should be set
// in response to the error.
type ConditionError struct {
	ConditionType   string
	ConditionReason string
	Err             error
}

func (e *ConditionError) Error() string {
	return fmt.Sprintf("%s - %s: %s", e.ConditionType, e.ConditionReason, e.Err.Error())
}

func (e *ConditionError) Unwrap() error {
	return e.Err
}

// applyStatus applies Branch status using Server-Side Apply. A Branch's status
// is built up in-memory during reconciliation, and then applied to the API
// server once at the end of reconciliation
func (r *BranchReconciler) applyStatus(ctx context.Context, br *v1alpha1.Branch) error {
	status := v1alpha1ac.BranchStatus().
		WithObservedGeneration(br.Status.ObservedGeneration).
		WithLastError(br.Status.LastError).
		WithConditions(conditionsAC(br.Status.Conditions)...)

	if br.Status.PrimaryXVolName != "" {
		status = status.WithPrimaryXVolName(br.Status.PrimaryXVolName)
	}

	if br.Status.FirstRecoverabilityPoint != "" {
		status = status.WithFirstRecoverabilityPoint(br.Status.FirstRecoverabilityPoint)
	}
	if br.Status.LastSuccessfulBackup != "" {
		status = status.WithLastSuccessfulBackup(br.Status.LastSuccessfulBackup)
	}
	if br.Status.LastFailedBackup != "" {
		status = status.WithLastFailedBackup(br.Status.LastFailedBackup)
	}
	if br.Status.LastRecoverabilityPoint != "" {
		status = status.WithLastRecoverabilityPoint(br.Status.LastRecoverabilityPoint)
	}

	ac := v1alpha1ac.Branch(br.Name, "").WithStatus(status)

	return r.Status().Apply(ctx, ac, client.FieldOwner(OperatorName), client.ForceOwnership)
}

// conditionsAC converts the given conditions to apply configurations.
func conditionsAC(conditions []metav1.Condition) []*metav1ac.ConditionApplyConfiguration {
	acs := make([]*metav1ac.ConditionApplyConfiguration, 0, len(conditions))

	for _, cond := range conditions {
		acs = append(acs, metav1ac.Condition().
			WithType(cond.Type).
			WithStatus(cond.Status).
			WithReason(cond.Reason).
			WithMessage(cond.Message).
			WithObservedGeneration(cond.ObservedGeneration).
			WithLastTransitionTime(cond.LastTransitionTime))
	}
	return acs
}

// setStatusConditionFromError sets the Branch Ready condition in memory based on
// the provided error. When the error is nil the condition is left unchanged (the
// caller sets the success condition). When the error is non-nil the condition is
// set to False: the condition type and reason from a ConditionError, or the
// generic Ready=False reconciliation failure otherwise.
func setStatusConditionFromError(br *v1alpha1.Branch, err error) {
	if err == nil {
		return
	}

	conditionType := v1alpha1.BranchReadyConditionType
	reason := v1alpha1.ReconciliationFailedReason
	if condErr, ok := errors.AsType[*ConditionError](err); ok {
		conditionType = condErr.ConditionType
		reason = condErr.ConditionReason
	}

	setStatusCondition(br, conditionType, metav1.ConditionFalse, reason)
}

// setStatusCondition sets the given condition on the Branch's in-memory
// conditions using meta.SetStatusCondition, which populates LastTransitionTime
// (a required field) and advances it only on an actual status transition.
func setStatusCondition(br *v1alpha1.Branch, conditionType string, status metav1.ConditionStatus, reason string) {
	meta.SetStatusCondition(&br.Status.Conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            v1alpha1.BranchConditionMessages[reason],
		ObservedGeneration: br.Generation,
	})
}

// setReadyCondition sets the Branch Ready condition in memory with the given
// status and reason.
func setReadyCondition(br *v1alpha1.Branch, status metav1.ConditionStatus, reason string) {
	setStatusCondition(br, v1alpha1.BranchReadyConditionType, status, reason)
}

// errorMessage returns the error's message, or the empty string if err is nil.
func errorMessage(err error) string {
	if err != nil {
		return err.Error()
	}
	return ""
}
