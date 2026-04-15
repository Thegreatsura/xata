package datawarehouse

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

type fakeRow struct {
	id     string
	queued bool
}

func (r *fakeRow) RowID() string { return r.id }

func (r *fakeRow) QueueUpsert(b *pgx.Batch) {
	r.queued = true
	b.Queue("SELECT 1")
}

type fakeDW struct {
	runInTxFn func(ctx context.Context, fn func(context.Context, pgx.Tx) error) error
}

func (f *fakeDW) EnsureSchema(context.Context) error { return nil }
func (f *fakeDW) Close()                             {}

func (f *fakeDW) RunInTx(ctx context.Context, fn func(context.Context, pgx.Tx) error) error {
	return f.runInTxFn(ctx, fn)
}

type fakeTx struct {
	pgx.Tx
	sendBatchCalls int
	batchSizes     []int
	execErr        error
}

func (f *fakeTx) SendBatch(_ context.Context, b *pgx.Batch) pgx.BatchResults {
	f.sendBatchCalls++
	f.batchSizes = append(f.batchSizes, b.Len())
	return &fakeBatchResults{remaining: b.Len(), execErr: f.execErr}
}

type fakeBatchResults struct {
	pgx.BatchResults
	remaining int
	execErr   error
}

func (f *fakeBatchResults) Exec() (pgconn.CommandTag, error) {
	if f.execErr != nil {
		return pgconn.CommandTag{}, f.execErr
	}
	f.remaining--
	return pgconn.NewCommandTag("INSERT 0 1"), nil
}

func (f *fakeBatchResults) Close() error {
	return nil
}

func TestBulkUpsertRows(t *testing.T) {
	tests := map[string]struct {
		rows    []*fakeRow
		txErr   error
		execErr error
		wantErr string
	}{
		"empty rows": {
			rows: []*fakeRow{},
		},
		"single row": {
			rows: []*fakeRow{{id: "r1"}},
		},
		"multiple rows": {
			rows: []*fakeRow{{id: "r1"}, {id: "r2"}, {id: "r3"}},
		},
		"RunInTx error propagates": {
			rows:    []*fakeRow{{id: "r1"}},
			txErr:   errors.New("tx failed"),
			wantErr: "tx failed",
		},
		"exec error propagates": {
			rows:    []*fakeRow{{id: "r1"}},
			execErr: errors.New("insert failed"),
			wantErr: "execute batch starting at 0: upsert test statement 0: insert failed",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			tx := &fakeTx{execErr: tt.execErr}
			dwh := &fakeDW{
				runInTxFn: func(ctx context.Context, fn func(context.Context, pgx.Tx) error) error {
					if tt.txErr != nil {
						return tt.txErr
					}
					return fn(ctx, tx)
				},
			}

			err := BulkUpsertRows(context.Background(), dwh, tt.rows, "test")
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			for _, row := range tt.rows {
				require.True(t, row.queued, "row %s was not queued", row.id)
			}
			if len(tt.rows) > 0 {
				require.Equal(t, 1, tx.sendBatchCalls)
				require.Equal(t, []int{len(tt.rows)}, tx.batchSizes)
			}
		})
	}
}

func TestBulkUpsertRowsBatching(t *testing.T) {
	tests := map[string]struct {
		numRows        int
		wantBatches    int
		wantBatchSizes []int
	}{
		"fewer than batch size": {
			numRows:        3,
			wantBatches:    1,
			wantBatchSizes: []int{3},
		},
		"exactly batch size": {
			numRows:        upsertBatchSize,
			wantBatches:    1,
			wantBatchSizes: []int{upsertBatchSize},
		},
		"one more than batch size": {
			numRows:        upsertBatchSize + 1,
			wantBatches:    2,
			wantBatchSizes: []int{upsertBatchSize, 1},
		},
		"two full batches": {
			numRows:        upsertBatchSize * 2,
			wantBatches:    2,
			wantBatchSizes: []int{upsertBatchSize, upsertBatchSize},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			rows := make([]*fakeRow, tt.numRows)
			for i := range rows {
				rows[i] = &fakeRow{id: "r"}
			}

			tx := &fakeTx{}
			dwh := &fakeDW{
				runInTxFn: func(ctx context.Context, fn func(context.Context, pgx.Tx) error) error {
					return fn(ctx, tx)
				},
			}

			err := BulkUpsertRows(context.Background(), dwh, rows, "test")
			require.NoError(t, err)
			require.Equal(t, tt.wantBatches, tx.sendBatchCalls)
			require.Equal(t, tt.wantBatchSizes, tx.batchSizes)
			for _, row := range rows {
				require.True(t, row.queued)
			}
		})
	}
}
