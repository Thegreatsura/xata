package v1alpha1

const (
	ClusterPoolReadyConditionType = "Ready"
)

const (
	PoolHealthyReason            = "PoolHealthy"
	ReconciliationFailedReason   = "ReconciliationFailed"
	AwaitingReconciliationReason = "AwaitingReconciliation"
)

var ClusterPoolConditionMessages = map[string]string{
	PoolHealthyReason:            "The cluster pool is healthy",
	ReconciliationFailedReason:   "An error occurred during reconciliation",
	AwaitingReconciliationReason: "The cluster pool is awaiting reconciliation",
}
