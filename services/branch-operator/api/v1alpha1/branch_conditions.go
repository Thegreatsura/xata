package v1alpha1

// Condition types for Branch `status.conditions` fields
const (
	BranchReadyConditionType       = "Ready"
	XVolInfoAvailableConditionType = "XVolInfoAvailable"
)

// Reason strings for Branch conditions
const (
	// Reason strings for the `Ready` True condition
	ResourcesReadyReason = "ResourcesReady"

	// Reason strings for the `Ready` False condition
	ParentBranchNotFoundReason     = "ParentBranchNotFound"
	ParentBranchHasNoXVolReason    = "ParentBranchHasNoXVol"
	ParentClusterNotFoundReason    = "ParentClusterNotFound"
	ParentBranchHasNoClusterReason = "ParentBranchHasNoCluster"
	ParentClusterUnhealthyReason   = "ParentClusterUnhealthy"
	ParentClusterPVCNotFoundReason = "ParentClusterPVCNotFound"
	ReconciliationFailedReason     = "ReconciliationFailed"

	// Reason strings for the `Ready` Unknown condition
	ReconciliationPausedReason = "ReconciliationPaused"

	// Reason strings for the `XVolInfoAvailable` True condition
	XVolInfoCollectedReason = "XVolInfoCollected"

	// Reason strings for the `XVolInfoAvailable` False condition
	XVolNotFoundReason           = "XVolNotFound"
	XVolCRDNotInstalledReason    = "XVolCRDNotInstalled"
	BranchHasNoClusterReason     = "BranchHasNoCluster"
	ClusterPVCNotAvailableReason = "ClusterPVCNotAvailable"
	PVNotBoundReason             = "PVNotBound"

	// Shared reason strings for multiple conditions
	AwaitingReconciliationReason = "AwaitingReconciliation"
)

// BranchConditionMessages maps condition reasons to human-readable messages
var BranchConditionMessages = map[string]string{
	// Messages for the BranchReady condition
	ResourcesReadyReason:           "All resources created",
	ParentBranchNotFoundReason:     "The parent branch was not found",
	ParentBranchHasNoXVolReason:    "The parent branch has no XVol",
	ParentClusterNotFoundReason:    "The parent cluster for the branch was not found",
	ParentBranchHasNoClusterReason: "The parent branch has no cluster",
	ParentClusterUnhealthyReason:   "The parent cluster for the branch is not healthy",
	ParentClusterPVCNotFoundReason: "The PVC for the parent cluster was not found",
	AwaitingReconciliationReason:   "The branch is awaiting reconciliation",
	ReconciliationPausedReason:     "Reconciliation has been paused",
	ReconciliationFailedReason:     "An error occurred during reconciliation",

	// Messages for the XVolInfoAvailable condition
	XVolInfoCollectedReason:      "XVol information has been collected",
	XVolNotFoundReason:           "No XVol found for the primary volume",
	XVolCRDNotInstalledReason:    "The XVol CRD is not installed",
	BranchHasNoClusterReason:     "The branch has no associated cluster",
	ClusterPVCNotAvailableReason: "The cluster has no primary PVC available",
	PVNotBoundReason:             "The PVC is not bound to a PV",
}
