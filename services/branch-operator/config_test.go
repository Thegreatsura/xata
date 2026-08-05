package branchoperator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func TestReadConfigUsesStructSetterForTolerations(t *testing.T) {
	t.Setenv("XATA_CLUSTERS_TOLERATIONS", "dedicated=database:NoSchedule")

	service := NewBranchOperatorService()
	require.NoError(t, service.ReadConfig(context.Background()))
	require.Equal(t, []corev1.Toleration{
		{
			Key:      "dedicated",
			Operator: corev1.TolerationOpEqual,
			Value:    "database",
			Effect:   corev1.TaintEffectNoSchedule,
		},
	}, service.config.Tolerations.Value)
}
