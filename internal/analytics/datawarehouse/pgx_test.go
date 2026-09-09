package datawarehouse

import (
	"context"
	"testing"
	"time"

	"xata/internal/pgtestutil"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

func TestNewPGXRejectsHeldAdvisoryLock(t *testing.T) {
	ctx := context.Background()
	postgresURL := startPostgres(t, ctx)
	lockID := int64(42)
	holder, err := pgx.Connect(ctx, postgresURL)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, holder.Close(context.Background())) })
	_, err = holder.Exec(ctx, "select pg_advisory_lock($1)", lockID)
	require.NoError(t, err)

	_, err = NewPGX(ctx, PGXConfig{PostgresURL: postgresURL, AdvisoryLockID: &lockID})

	require.ErrorContains(t, err, "another warehouse export is already running")
}

func TestPGXReconnectWaitsForAdvisoryLock(t *testing.T) {
	ctx := context.Background()
	postgresURL := startPostgres(t, ctx)
	lockID := int64(42)
	holder, err := pgx.Connect(ctx, postgresURL)
	require.NoError(t, err)
	_, err = holder.Exec(ctx, "select pg_advisory_lock($1)", lockID)
	require.NoError(t, err)

	closedConn, err := pgx.Connect(ctx, postgresURL)
	require.NoError(t, err)
	require.NoError(t, closedConn.Close(ctx))
	dwh := &pgxDW{postgresURL: postgresURL, conn: closedConn, lockID: &lockID}
	t.Cleanup(dwh.Close)
	holderClosed := make(chan error, 1)
	go func() {
		time.Sleep(100 * time.Millisecond)
		holderClosed <- holder.Close(context.Background())
	}()

	var got int
	err = dwh.RunInTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, "select 1").Scan(&got)
	})

	require.NoError(t, <-holderClosed)
	require.NoError(t, err)
	require.Equal(t, 1, got)
}

func startPostgres(t *testing.T, ctx context.Context) string {
	t.Helper()
	return pgtestutil.DSN(ctx, t)
}
