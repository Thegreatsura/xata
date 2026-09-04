package sqlstore

import (
	"context"
	"sync"
	"testing"
	"time"

	"xata/services/projects/store"

	"github.com/stretchr/testify/require"
)

// TestSQLStoreOrganizationStatuses covers the claim and version semantics that
// keep a half-applied organization status owed rather than silently lost. These
// need real Postgres: SKIP LOCKED and the version guard are database behaviour,
// not Go behaviour.
func TestSQLStoreOrganizationStatuses(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	sqlStore := setupSQLStore(ctx, t, maxDepth)
	// The store stamps next_retry_at with its own clock when a row is written,
	// so claims are made from a point comfortably after that.
	now := time.Now().UTC()
	claimAfter := now.Add(time.Hour)

	t.Run("upsert records desired state and leaves it unsynced", func(t *testing.T) {
		got, err := sqlStore.UpsertOrganizationStatus(ctx, "org-upsert", true)
		require.NoError(t, err)
		require.Equal(t, "org-upsert", got.OrganizationID)
		require.True(t, got.Disabled)
		require.Nil(t, got.SyncedAt, "a new desired state is owed a sync")
		require.Equal(t, int64(1), got.Version)
	})

	t.Run("version only advances when the desired state actually changes", func(t *testing.T) {
		first, err := sqlStore.UpsertOrganizationStatus(ctx, "org-version", true)
		require.NoError(t, err)

		same, err := sqlStore.UpsertOrganizationStatus(ctx, "org-version", true)
		require.NoError(t, err)
		require.Equal(t, first.Version, same.Version, "a repeated webhook must not invalidate an in-flight sync")

		flipped, err := sqlStore.UpsertOrganizationStatus(ctx, "org-version", false)
		require.NoError(t, err)
		require.Equal(t, first.Version+1, flipped.Version)
	})

	t.Run("marking synced is refused when the desired state moved", func(t *testing.T) {
		claimed, err := sqlStore.UpsertOrganizationStatus(ctx, "org-guard", true)
		require.NoError(t, err)

		// The status flips while the sync is still running.
		_, err = sqlStore.UpsertOrganizationStatus(ctx, "org-guard", false)
		require.NoError(t, err)

		marked, err := sqlStore.MarkOrganizationStatusSynced(ctx, "org-guard", claimed.Version, now)
		require.NoError(t, err)
		require.False(t, marked, "a sync of a stale version must not mark the row done")

		// Still claimable, which is what makes the flip converge.
		pending := claimAll(t, ctx, sqlStore, claimAfter, time.Minute)
		require.True(t, containsOrg(pending, "org-guard"))
	})

	t.Run("claiming leases the row so it is not handed out twice", func(t *testing.T) {
		_, err := sqlStore.UpsertOrganizationStatus(ctx, "org-lease", true)
		require.NoError(t, err)

		first := claimAll(t, ctx, sqlStore, claimAfter, time.Hour)
		require.True(t, containsOrg(first, "org-lease"))

		second := claimAll(t, ctx, sqlStore, claimAfter, time.Hour)
		require.False(t, containsOrg(second, "org-lease"), "the lease must hold until it expires")

		// Once the lease expires the row is claimable again, so a worker that
		// died mid-sync does not strand the organization.
		afterLease := claimAll(t, ctx, sqlStore, claimAfter.Add(3*time.Hour), time.Hour)
		require.True(t, containsOrg(afterLease, "org-lease"))
	})

	t.Run("upsert during an active lease does not hand the row to a second worker", func(t *testing.T) {
		// Regression for the interleaving case: worker A is part way through
		// hibernating an organization when a wind up arrives. If that write
		// made the row claimable, worker B would start un-hibernating the
		// same branches while A was still hibernating them, and whichever
		// wrote last on a branch would win regardless of which state was
		// newer.
		first, err := sqlStore.UpsertOrganizationStatus(ctx, "org-inflight", true)
		require.NoError(t, err)
		claimAt := claimAfter.Add(12 * time.Hour)

		claimed := claimAll(t, ctx, sqlStore, claimAt, time.Hour)
		require.True(t, containsOrg(claimed, "org-inflight"))

		// The flip lands mid-sync.
		flipped, err := sqlStore.UpsertOrganizationStatus(ctx, "org-inflight", false)
		require.NoError(t, err)
		require.Equal(t, first.Version+1, flipped.Version)
		require.NotNil(t, flipped.LeaseUntil, "an upsert must not release a lease it does not hold")

		again := claimAll(t, ctx, sqlStore, claimAt.Add(time.Minute), time.Hour)
		require.False(t, containsOrg(again, "org-inflight"), "the row is still leased to the first worker")

		// Worker A finishes its stale sync. The lease is released, the row is
		// not marked synced, and the newer state is claimable immediately
		// rather than after the lease would have expired.
		marked, err := sqlStore.MarkOrganizationStatusSynced(ctx, "org-inflight", first.Version, now)
		require.NoError(t, err)
		require.False(t, marked)

		next := claimAll(t, ctx, sqlStore, claimAt.Add(2*time.Minute), time.Hour)
		require.True(t, containsOrg(next, "org-inflight"))
		for _, s := range next {
			if s.OrganizationID == "org-inflight" {
				require.False(t, s.Disabled, "the second claim applies the newer state")
				require.Equal(t, flipped.Version, s.Version)
			}
		}
	})

	t.Run("a stale failure does not delay a newer desired state", func(t *testing.T) {
		first, err := sqlStore.UpsertOrganizationStatus(ctx, "org-stale-fail", true)
		require.NoError(t, err)
		claimAt := claimAfter.Add(36 * time.Hour)

		claimed := claimAll(t, ctx, sqlStore, claimAt, time.Hour)
		require.True(t, containsOrg(claimed, "org-stale-fail"))

		// The state flips, then the old attempt fails and asks for a long
		// backoff. That backoff belongs to the old version and must not push
		// out the schedule the flip set.
		_, err = sqlStore.UpsertOrganizationStatus(ctx, "org-stale-fail", false)
		require.NoError(t, err)
		require.NoError(t, sqlStore.MarkOrganizationStatusFailed(ctx, "org-stale-fail", first.Version, "stale attempt", claimAt.Add(10*time.Hour)))

		next := claimAll(t, ctx, sqlStore, claimAt.Add(time.Minute), time.Hour)
		require.True(t, containsOrg(next, "org-stale-fail"), "the lease is released and the newer state is due now")
		for _, s := range next {
			if s.OrganizationID == "org-stale-fail" {
				require.Empty(t, s.LastError, "the stale attempt's error is not recorded against the new version")
			}
		}
	})

	t.Run("synced rows are not claimed again", func(t *testing.T) {
		status, err := sqlStore.UpsertOrganizationStatus(ctx, "org-done", true)
		require.NoError(t, err)
		marked, err := sqlStore.MarkOrganizationStatusSynced(ctx, "org-done", status.Version, now)
		require.NoError(t, err)
		require.True(t, marked)

		pending := claimAll(t, ctx, sqlStore, claimAfter.Add(6*time.Hour), time.Minute)
		require.False(t, containsOrg(pending, "org-done"))
	})

	t.Run("concurrent workers claim disjoint sets", func(t *testing.T) {
		const orgs = 12
		for i := range orgs {
			_, err := sqlStore.UpsertOrganizationStatus(ctx, "org-race-"+string(rune('a'+i)), true)
			require.NoError(t, err)
		}
		claimAt := claimAfter.Add(24 * time.Hour)

		// Results come back over a channel rather than being asserted inside the
		// goroutines, because a failed assertion in a goroutine panics.
		type claimResult struct {
			ids []string
			err error
		}
		results := make(chan claimResult, 2)
		var wg sync.WaitGroup
		for range 2 {
			wg.Go(func() {
				claimed, err := claimUntilEmpty(ctx, sqlStore, claimAt, time.Hour)
				ids := make([]string, 0, len(claimed))
				for _, c := range claimed {
					ids = append(ids, c.OrganizationID)
				}
				results <- claimResult{ids: ids, err: err}
			})
		}
		wg.Wait()
		close(results)

		seen := map[string]int{}
		for r := range results {
			require.NoError(t, r.err)
			for _, id := range r.ids {
				seen[id]++
			}
		}
		for id, count := range seen {
			require.Equal(t, 1, count, "organization %s was claimed by more than one worker", id)
		}
	})

	t.Run("failure records the reason and defers the retry", func(t *testing.T) {
		status, err := sqlStore.UpsertOrganizationStatus(ctx, "org-fail", true)
		require.NoError(t, err)
		retryAt := claimAfter.Add(72 * time.Hour)
		require.NoError(t, sqlStore.MarkOrganizationStatusFailed(ctx, "org-fail", status.Version, "cell unreachable", retryAt))

		notYet := claimAll(t, ctx, sqlStore, claimAfter.Add(48*time.Hour), time.Minute)
		require.False(t, containsOrg(notYet, "org-fail"))

		due := claimAll(t, ctx, sqlStore, retryAt.Add(time.Minute), time.Minute)
		require.True(t, containsOrg(due, "org-fail"))
		for _, s := range due {
			if s.OrganizationID == "org-fail" {
				require.Equal(t, "cell unreachable", s.LastError)
			}
		}
	})

	t.Run("delete removes the row and is safe to repeat", func(t *testing.T) {
		_, err := sqlStore.UpsertOrganizationStatus(ctx, "org-delete", true)
		require.NoError(t, err)

		require.NoError(t, sqlStore.DeleteOrganizationStatus(ctx, "org-delete"))

		pending := claimAll(t, ctx, sqlStore, claimAfter, time.Minute)
		require.False(t, containsOrg(pending, "org-delete"))

		// The cleanup job may run again for an organization already gone.
		require.NoError(t, sqlStore.DeleteOrganizationStatus(ctx, "org-delete"))
	})

	t.Run("backlog is countable for alerting", func(t *testing.T) {
		count, _, err := sqlStore.CountUnsyncedOrganizationStatuses(ctx)
		require.NoError(t, err)
		require.Positive(t, count)
	})
}

// claimUntilEmpty claims one row at a time until nothing is due, which is how
// the worker drains the queue. Tests use it to see everything claimable at now.
func claimUntilEmpty(ctx context.Context, s *sqlProjectStore, now time.Time, lease time.Duration) ([]store.OrganizationStatus, error) {
	var claimed []store.OrganizationStatus
	for {
		next, err := s.ClaimOrganizationStatusForSync(ctx, now, lease)
		if err != nil {
			return nil, err
		}
		if next == nil {
			return claimed, nil
		}
		claimed = append(claimed, *next)
	}
}

func claimAll(t *testing.T, ctx context.Context, s *sqlProjectStore, now time.Time, lease time.Duration) []store.OrganizationStatus {
	t.Helper()
	claimed, err := claimUntilEmpty(ctx, s, now, lease)
	require.NoError(t, err)
	return claimed
}

func containsOrg(statuses []store.OrganizationStatus, id string) bool {
	for _, s := range statuses {
		if s.OrganizationID == id {
			return true
		}
	}
	return false
}
