package reconciler_test

import (
	"context"
	"testing"
	"time"

	"xata/services/branch-operator/api/v1alpha1"
	"xata/services/branch-operator/pkg/reconciler"

	barmanPluginApi "github.com/cloudnative-pg/plugin-barman-cloud/api/v1"
	"github.com/stretchr/testify/require"
	apiv1 "github.com/xataio/xata-cnpg/api/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestInheritedMetadataLabels(t *testing.T) {
	t.Parallel()

	t.Run("inherited labels are applied to owned resources", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()

		const projectIDLabelKey = "xata.io/projectID"
		const orgIDLabelKey = "xata.io/organizationID"

		branch := NewBranchBuilder().
			WithBackupRetention("7d").
			WithBackupSchedule("0 0 * * *").
			WithInheritedMetadata(&v1alpha1.InheritedMetadata{
				Labels: map[string]string{
					orgIDLabelKey:     "some-org-id",
					projectIDLabelKey: "some-project-id",
				},
			}).
			Build()

		withBranch(ctx, t, branch, func(t *testing.T, br *v1alpha1.Branch) {
			// Expect the Cluster to be created
			cluster := apiv1.Cluster{}
			requireEventuallyNoErr(t, func() error {
				return getK8SObject(ctx, br.Name, &cluster)
			})

			// Ensure the labels are present on the Cluster
			require.Equal(t, "some-org-id", cluster.Labels[orgIDLabelKey])
			require.Equal(t, "some-project-id", cluster.Labels[projectIDLabelKey])

			// Expect the clusters Service to be created
			svc := corev1.Service{}
			requireEventuallyNoErr(t, func() error {
				svcName := reconciler.ClustersServiceNamePrefix + br.Name

				return getK8SObjectInNamespace(ctx, svcName, XataNamespace, &svc)
			})

			// Ensure the labels are present on the Service
			require.Equal(t, "some-org-id", svc.Labels[orgIDLabelKey])
			require.Equal(t, "some-project-id", svc.Labels[projectIDLabelKey])

			// Expect the NetworkPolicy to be created
			np := networkingv1.NetworkPolicy{}
			requireEventuallyNoErr(t, func() error {
				return getK8SObject(ctx, br.Name, &np)
			})

			// Ensure the labels are present on the NetworkPolicy
			require.Equal(t, "some-org-id", np.Labels[orgIDLabelKey])
			require.Equal(t, "some-project-id", np.Labels[projectIDLabelKey])

			// Expect the ObjectStore to be created
			os := barmanPluginApi.ObjectStore{}
			requireEventuallyNoErr(t, func() error {
				return getK8SObject(ctx, br.Name, &os)
			})

			// Ensure the labels are present on the ObjectStore
			require.Equal(t, "some-org-id", os.Labels[orgIDLabelKey])
			require.Equal(t, "some-project-id", os.Labels[projectIDLabelKey])

			// Expect the ScheduledBackup to be created
			sb := apiv1.ScheduledBackup{}
			requireEventuallyNoErr(t, func() error {
				return getK8SObject(ctx, br.Name, &sb)
			})

			// Ensure the labels are present on the ScheduledBackup
			require.Equal(t, "some-org-id", sb.Labels[orgIDLabelKey])
			require.Equal(t, "some-project-id", sb.Labels[projectIDLabelKey])
		})
	})
}

func TestPauseReconciliationAnnotation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("branch status reflects reconciliation pause annotation", func(t *testing.T) {
		t.Parallel()

		branch := NewBranchBuilder().Build()

		withBranch(ctx, t, branch, func(t *testing.T, br *v1alpha1.Branch) {
			// Add the pause reconciliation annotation
			err := retryOnConflict(ctx, br, func(b *v1alpha1.Branch) {
				if b.Annotations == nil {
					b.Annotations = make(map[string]string)
				}
				b.Annotations[reconciler.ReconciliationPausedAnnotation] = "true"
			})
			require.NoError(t, err)

			// Expect the status of the Branch to be updated. The condition should
			// reflect the fact that reconciliation is paused.
			requireEventuallyTrue(t, func() bool {
				err := getK8SObject(ctx, br.Name, br)
				if err != nil {
					return false
				}
				c := meta.FindStatusCondition(br.Status.Conditions, v1alpha1.BranchReadyConditionType)
				if c == nil {
					return false
				}
				return c.Status == metav1.ConditionUnknown && c.Reason == v1alpha1.ReconciliationPausedReason
			})

			// Remove the pause reconciliation annotation
			err = retryOnConflict(ctx, br, func(b *v1alpha1.Branch) {
				delete(b.Annotations, reconciler.ReconciliationPausedAnnotation)
			})
			require.NoError(t, err)

			// Expect the status of the Branch to be updated. Branch reconciliation
			// is no longer paused, so the status should eventually turn to
			// Ready=True.
			requireEventuallyTrue(t, func() bool {
				err := getK8SObject(ctx, br.Name, br)
				if err != nil {
					return false
				}
				c := meta.FindStatusCondition(br.Status.Conditions, v1alpha1.BranchReadyConditionType)
				if c == nil {
					return false
				}
				return meta.IsStatusConditionTrue(br.Status.Conditions, v1alpha1.BranchReadyConditionType)
			})
		})
	})
}

func TestRestoreImmutability(t *testing.T) {
	t.Parallel()

	t.Run("restore spec can not be added after branch creation", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()

		branch := NewBranchBuilder().Build()

		withBranch(ctx, t, branch, func(t *testing.T, br *v1alpha1.Branch) {
			// Attempt to add a restore spec
			err := retryOnConflict(ctx, br, func(b *v1alpha1.Branch) {
				b.Spec.Restore = &v1alpha1.RestoreSpec{
					Type: v1alpha1.RestoreTypeVolumeSnapshot,
					Name: "some-snapshot",
				}
			})

			// Expect the update to be rejected with a validation error
			require.Error(t, err)
			require.True(t, apierrors.IsInvalid(err))
		})
	})

	t.Run("restore spec can not be removed after branch creation", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()

		branch := NewBranchBuilder().
			WithRestore(v1alpha1.RestoreTypeVolumeSnapshot, "some-snapshot").
			Build()

		withBranch(ctx, t, branch, func(t *testing.T, br *v1alpha1.Branch) {
			// Attempt to remove the restore spec
			err := retryOnConflict(ctx, br, func(b *v1alpha1.Branch) {
				b.Spec.Restore = nil
			})

			// Expect the update to be rejected with a validation error
			require.Error(t, err)
			require.True(t, apierrors.IsInvalid(err))
		})
	})

	t.Run("restore spec type can not be modified", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()

		branch := NewBranchBuilder().
			WithRestore(v1alpha1.RestoreTypeVolumeSnapshot, "some-snapshot").
			Build()

		withBranch(ctx, t, branch, func(t *testing.T, br *v1alpha1.Branch) {
			// Attempt to modify the restore type
			err := retryOnConflict(ctx, br, func(b *v1alpha1.Branch) {
				b.Spec.Restore.Type = v1alpha1.RestoreTypeObjectStore
			})

			// Expect the update to be rejected with a validation error
			require.Error(t, err)
			require.True(t, apierrors.IsInvalid(err))
		})
	})

	t.Run("restore spec name can not be modified", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()

		branch := NewBranchBuilder().
			WithRestore(v1alpha1.RestoreTypeVolumeSnapshot, "some-snapshot").
			Build()

		withBranch(ctx, t, branch, func(t *testing.T, br *v1alpha1.Branch) {
			// Attempt to modify the restore name
			err := retryOnConflict(ctx, br, func(b *v1alpha1.Branch) {
				b.Spec.Restore.Name = "different-snapshot"
			})

			// Expect the update to be rejected with a validation error
			require.Error(t, err)
			require.True(t, apierrors.IsInvalid(err))
		})
	})

	t.Run("restore spec timestamp can not be modified", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()

		timestamp := time.Now().Add(-1 * time.Hour)
		branch := NewBranchBuilder().
			WithRestoreTimestamp(v1alpha1.RestoreTypeObjectStore, "some-object-store", timestamp).
			Build()

		withBranch(ctx, t, branch, func(t *testing.T, br *v1alpha1.Branch) {
			// Attempt to modify the restore timestamp
			newTimestamp := time.Now()
			err := retryOnConflict(ctx, br, func(b *v1alpha1.Branch) {
				b.Spec.Restore.Timestamp = &metav1.Time{Time: newTimestamp}
			})

			// Expect the update to be rejected with a validation error
			require.Error(t, err)
			require.True(t, apierrors.IsInvalid(err))
		})
	})
}
