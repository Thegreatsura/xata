package datawarehouse

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

const advisoryLockReacquireTimeout = time.Minute

type PGXConfig struct {
	PostgresURL    string
	AdvisoryLockID *int64
	Schema         *SchemaInitializer
}

type pgxDW struct {
	postgresURL string
	conn        *pgx.Conn
	lockID      *int64
	schema      *SchemaInitializer
}

func NewPGX(ctx context.Context, config PGXConfig) (DW, error) {
	conn, err := pgx.Connect(ctx, config.PostgresURL)
	if err != nil {
		return nil, fmt.Errorf("connect warehouse: %w", err)
	}

	if config.AdvisoryLockID != nil {
		var locked bool
		err := conn.QueryRow(ctx, "select pg_try_advisory_lock($1)", *config.AdvisoryLockID).Scan(&locked)
		if err != nil {
			_ = conn.Close(context.Background())
			return nil, fmt.Errorf("acquire advisory lock: %w", err)
		}
		if !locked {
			_ = conn.Close(context.Background())
			return nil, fmt.Errorf("acquire advisory lock: another warehouse export is already running (advisory lock %d is held)", *config.AdvisoryLockID)
		}
	}

	return &pgxDW{
		postgresURL: config.PostgresURL,
		conn:        conn,
		lockID:      config.AdvisoryLockID,
		schema:      config.Schema,
	}, nil
}

func (d *pgxDW) EnsureSchema(ctx context.Context) error {
	if d.schema == nil {
		return nil
	}
	return d.schema.EnsureSchema(ctx, d.conn)
}

func (d *pgxDW) RunInTx(ctx context.Context, fn func(ctx context.Context, tx pgx.Tx) error) error {
	tx, err := d.beginTx(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	if err := fn(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

func (d *pgxDW) beginTx(ctx context.Context) (pgx.Tx, error) {
	tx, err := d.conn.Begin(ctx)
	if err == nil || !d.conn.IsClosed() {
		return tx, err
	}
	log.Ctx(ctx).Warn().Err(err).Msg("warehouse connection closed, reconnecting")

	if err := d.reconnect(ctx); err != nil {
		return nil, fmt.Errorf("reconnect warehouse: %w", err)
	}
	return d.conn.Begin(ctx)
}

func (d *pgxDW) reconnect(ctx context.Context) error {
	_ = d.conn.Close(context.Background())

	conn, err := pgx.Connect(ctx, d.postgresURL)
	if err != nil {
		return fmt.Errorf("connect warehouse: %w", err)
	}
	if d.lockID != nil {
		lockCtx, cancel := context.WithTimeout(ctx, advisoryLockReacquireTimeout)
		_, err := conn.Exec(lockCtx, "select pg_advisory_lock($1)", *d.lockID)
		lockCtxErr := lockCtx.Err()
		cancel()
		if err != nil {
			_ = conn.Close(context.Background())
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			if lockCtxErr != nil {
				return fmt.Errorf("reacquire advisory lock %d after connection loss; another export may be running or the previous PostgreSQL session may still hold the lock: %w", *d.lockID, lockCtxErr)
			}
			return fmt.Errorf("reacquire advisory lock: %w", err)
		}
	}

	d.conn = conn
	log.Ctx(ctx).Info().Msg("warehouse connection reconnected")
	return nil
}

func (d *pgxDW) Close() {
	if err := d.conn.Close(context.Background()); err != nil {
		log.Error().Err(err).Msg("close warehouse connection")
	}
}
