// Package orgstatus propagates an organization's enabled/disabled state to
// every branch it owns.
//
// The desired state is written to Postgres and applied by a background
// worker rather than inline on the caller's request, so a sync interrupted
// partway through is retried rather than left half done. A lease keeps two
// workers from syncing the same organization at once, and a version guard on
// the row ensures a status change that arrives mid-sync is not lost.
package orgstatus

import (
	"context"
	"errors"
	"fmt"

	clustersv1 "xata/gen/proto/clusters/v1"
	"xata/services/projects/cells"
	"xata/services/projects/store"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/status"
)

// Sync brings every branch in the organization into line with disabled.
//
// A branch that fails is logged and skipped so the rest of the organization
// still makes progress, but the failure is returned so the caller leaves the
// organization marked unsynced and tries again instead of recording the
// fleet as converged while some branches were left untouched.
func Sync(ctx context.Context, projectsStore store.ProjectsStore, cellConns cells.Cells, organizationID string, disabled bool) error {
	projects, err := projectsStore.ListProjects(ctx, organizationID)
	if err != nil {
		return fmt.Errorf("list projects: %w", err)
	}

	connMap := make(map[string]cells.CellClient)
	defer func() {
		for k, conn := range connMap {
			if err := conn.Close(); err != nil {
				log.Ctx(ctx).Err(err).Msgf("Failed to close cell [%s] connection", k)
			}
		}
	}()

	var (
		seen, updated int
		failed        []error
	)

	for p := range projects {
		branches, err := projectsStore.ListBranches(ctx, organizationID, projects[p].ID)
		if err != nil {
			return fmt.Errorf("list branches for project %s: %w", projects[p].ID, err)
		}
		for b := range branches {
			// Once the context ends, every remaining branch fails and every
			// failure logs. Stop here rather than walk the rest of the
			// organization to produce one error line per branch.
			if abortErr := abortIfDone(ctx, organizationID, seen, updated); abortErr != nil {
				return abortErr
			}

			branch := branches[b]
			seen++

			if _, ok := connMap[branch.CellID]; !ok {
				conn, err := cellConns.GetCellConnection(ctx, organizationID, branch.CellID)
				if err != nil {
					if abortErr := abortIfDone(ctx, organizationID, seen, updated); abortErr != nil {
						return abortErr
					}
					log.Ctx(ctx).Err(err).Msgf("Failed to get cell [%s] connection for branch [%s]", branch.CellID, branch.ID)
					failed = append(failed, fmt.Errorf("cell %s connection for branch %s: %w", branch.CellID, branch.ID, err))
					continue
				}
				connMap[branch.CellID] = conn
			}

			client := connMap[branch.CellID]
			cluster, err := client.DescribePostgresCluster(ctx, &clustersv1.DescribePostgresClusterRequest{
				Id: branch.ID,
			})
			if err != nil {
				if abortErr := abortIfDone(ctx, organizationID, seen, updated); abortErr != nil {
					return abortErr
				}
				log.Ctx(ctx).Err(err).Msgf("Failed to describe cluster [%s]", branch.ID)
				failed = append(failed, fmt.Errorf("describe cluster %s: %w", branch.ID, err))
				continue
			}

			shouldUpdate := false
			hibernate := disabled
			// Only update if there is a change
			configuration := clustersv1.UpdateClusterConfiguration{}
			if hibernate != (cluster.Status.StatusType == clustersv1.ClusterStatus_STATUS_TYPE_HIBERNATED) {
				shouldUpdate = true
				configuration.Hibernate = new(hibernate)
			}

			// Toggle S2Z only for branches that already had it configured.
			// DescribePostgresCluster always returns a non-nil ScaleToZero proto
			// (defaulting to {Enabled:false, InactivityPeriodMinutes:0} for
			// branches without CRD config), so the nil check is purely defensive.
			// InactivityPeriodMinutes > 0 is the real guard: the CRD enforces
			// minimum=1, so 0 means the branch was never configured for S2Z.
			if cluster.Configuration.ScaleToZero != nil {
				desiredEnabled := !hibernate && cluster.Configuration.ScaleToZero.InactivityPeriodMinutes > 0
				if cluster.Configuration.ScaleToZero.Enabled != desiredEnabled {
					shouldUpdate = true
					configuration.ScaleToZero = &clustersv1.ScaleToZero{
						Enabled:                 desiredEnabled,
						InactivityPeriodMinutes: cluster.Configuration.ScaleToZero.InactivityPeriodMinutes,
					}
				}
			}

			if shouldUpdate {
				log.Ctx(ctx).Info().Msgf("Updating cluster [%s] configuration", branch.ID)
				_, err = client.UpdatePostgresCluster(ctx, &clustersv1.UpdatePostgresClusterRequest{
					Id:                  branch.ID,
					UpdateConfiguration: &configuration,
				})
				if err != nil {
					if abortErr := abortIfDone(ctx, organizationID, seen, updated); abortErr != nil {
						return abortErr
					}
					log.Ctx(ctx).Err(err).Msgf("Failed to update Postgres cluster for branch [%s]", branch.ID)
					failed = append(failed, fmt.Errorf("update cluster %s: %w", branch.ID, err))
					continue
				}
				updated++
			}
		}
	}

	if len(failed) > 0 {
		return fmt.Errorf("organization [%s]: %d of %d branches failed (%d updated): %w",
			organizationID, len(failed), seen, updated, errors.Join(failed...))
	}

	log.Ctx(ctx).Info().
		Str("organization", organizationID).
		Bool("disabled", disabled).
		Int("branches", seen).
		Int("updated", updated).
		Msg("Organization status synced")
	return nil
}

// abortIfDone reports that the sync stopped early because ctx ended, or nil
// while ctx is still live. The gRPC code comes from ctx.Err so a caller still
// sees Canceled or DeadlineExceeded, and the message names the organization and
// the progress made, neither of which a bare "context canceled" tells an
// operator.
func abortIfDone(ctx context.Context, organizationID string, seen, updated int) error {
	if ctx.Err() == nil {
		return nil
	}
	return status.Errorf(status.FromContextError(ctx.Err()).Code(),
		"update organization [%s] status aborted after %d branches (%d updated): %v",
		organizationID, seen, updated, context.Cause(ctx))
}
