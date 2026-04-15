package reconciler

import (
	apiv1 "github.com/xataio/xata-cnpg/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// GenerationOrAnnotationChanged returns a predicate that filters events
// where either the generation or the annotations have changed
func GenerationOrAnnotationChanged() predicate.Predicate {
	return predicate.Or(
		predicate.GenerationChangedPredicate{},
		predicate.AnnotationChangedPredicate{},
	)
}

// ClusterPhaseOrGenerationOrAnnotationChanged is a predicate for Cluster
// resources that filters events where the Cluster's phase, generation, or
// annotations have changed
var ClusterPhaseOrGenerationOrAnnotationChanged = predicate.Or(
	predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			//nolint:forcetypeassert
			oldCluster := e.ObjectOld.(*apiv1.Cluster)
			//nolint:forcetypeassert
			newCluster := e.ObjectNew.(*apiv1.Cluster)

			// React only to status Phase transitions
			return oldCluster.Status.Phase != newCluster.Status.Phase
		},
	},
	predicate.GenerationChangedPredicate{},
	predicate.AnnotationChangedPredicate{},
)
