package datawarehouse

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// DW is a common interface for interacting with the product
// analytics database.
//
// When creating a new data warehouse connector make sure to create a new
// exporter role in the DWH. Naming convention: {schema_name}_exporter.
// If you are inserting data into the schema named orb, the name of
// the exporter role should be orb_exporter.
// Make sure that the exporter role has create and usage access on the schema, and
// only on that schema.
//
// Connection string must be passed using environment variables. Store it in
// a secret pa-{source_name}-secret.yaml.
type DW interface {
	EnsureSchema(ctx context.Context) error
	RunInTx(ctx context.Context, fn func(ctx context.Context, tx pgx.Tx) error) error
	Close()
}

// UpsertRow is implemented by row types that can be bulk-upserted into the
// data warehouse.
type UpsertRow interface {
	QueueUpsert(batch *pgx.Batch)
}

const upsertBatchSize = 500

// BulkUpsertRows upserts rows into the data warehouse within a single
// transaction managed by dwh.RunInTx.
func BulkUpsertRows[T UpsertRow](ctx context.Context, dwh DW, rows []T, rowType string) error {
	return dwh.RunInTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return ExecBatches(ctx, tx, rows, rowType)
	})
}

// ExecBatches upserts rows in batches of upsertBatchSize within the given
// transaction. It can be used directly when the caller manages its own
// transaction (e.g. to wrap multiple entity types in a single tx).
func ExecBatches[T UpsertRow](ctx context.Context, tx pgx.Tx, rows []T, rowType string) error {
	totalRows := len(rows)
	totalBatches := (totalRows + upsertBatchSize - 1) / upsertBatchSize
	logger := log.Ctx(ctx)

	for start := 0; start < totalRows; start += upsertBatchSize {
		end := min(start+upsertBatchSize, totalRows)
		if err := execBatch(ctx, tx, rows[start:end], rowType); err != nil {
			return fmt.Errorf("execute batch starting at %d: %w", start, err)
		}

		batchNumber := start/upsertBatchSize + 1
		logger.Debug().
			Str("row_type", rowType).
			Int("batch", batchNumber).
			Int("batches_total", totalBatches).
			Int("rows_in_batch", end-start).
			Int("rows_total", totalRows).
			Msg("upsert batch")
	}
	return nil
}

func execBatch[T UpsertRow](ctx context.Context, tx pgx.Tx, rows []T, rowType string) error {
	var batch pgx.Batch
	for _, row := range rows {
		row.QueueUpsert(&batch)
	}

	batchResults := tx.SendBatch(ctx, &batch)
	for i := range batch.Len() {
		if _, err := batchResults.Exec(); err != nil {
			_ = batchResults.Close()
			return fmt.Errorf("upsert %s statement %d: %w", rowType, i, err)
		}
	}
	if err := batchResults.Close(); err != nil {
		return fmt.Errorf("close batch: %w", err)
	}
	return nil
}

// SchemaInitializer holds idempotent DDL that is executed on every run
// to ensure the schema is up to date.
type SchemaInitializer struct {
	// InitSQL is idempotent DDL (CREATE ... IF NOT EXISTS) executed on every run.
	InitSQL string
}

// EnsureSchema executes InitSQL. The DDL must be idempotent.
func (s *SchemaInitializer) EnsureSchema(ctx context.Context, conn *pgxpool.Conn) error {
	log.Ctx(ctx).Info().Msg("ensuring schema")
	if _, err := conn.Exec(ctx, s.InitSQL); err != nil {
		return fmt.Errorf("ensure schema: %w", err)
	}
	return nil
}
