package sqlstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"maps"

	"xata/internal/idgen"
	"xata/services/auth/store"
)

// Index names from migration 011_vercel_resources.json, used to classify unique
// violations. xata_project_id / xata_branch_id are logical references to the
// projects database (no FK), so there is no project/branch existence check here —
// the caller provisions the project/branch via the projects gRPC first.
const (
	UniqueConstraintVercelResourcePK               = "vercel_resources_pkey"
	UniqueConstraintVercelResourceProject          = "unique_active_vercel_resource_project"
	UniqueConstraintVercelResourceBranchScope      = "unique_active_vercel_resource_branch_scope"
	UniqueConstraintVercelResourceBranchXataBranch = "unique_active_vercel_resource_branch_xata_branch"
)

// rowQueryer is the subset of *sql.DB / *sql.Tx used by the resource inserts, so
// the same insert logic runs standalone or inside a transaction.
type rowQueryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func (s *sqlAuthStore) CreateVercelResource(ctx context.Context, resource *store.VercelResource) (*store.VercelResource, error) {
	return s.insertVercelResource(ctx, s.sql, resource)
}

func (s *sqlAuthStore) insertVercelResource(ctx context.Context, q rowQueryer, resource *store.VercelResource) (*store.VercelResource, error) {
	created := &store.VercelResource{
		ResourceID:     resource.ResourceID,
		InstallationID: resource.InstallationID,
		ProductSlug:    resource.ProductSlug,
		BillingPlanID:  resource.BillingPlanID,
		XataProjectID:  resource.XataProjectID,
		Name:           resource.Name,
		// Clone so the returned row does not alias the caller's map.
		Metadata: maps.Clone(resource.Metadata),
	}
	if created.Metadata == nil {
		created.Metadata = map[string]any{}
	}
	metadataJSON, err := json.Marshal(created.Metadata)
	if err != nil {
		return nil, fmt.Errorf("marshal resource metadata: %w", err)
	}
	err = q.QueryRowContext(ctx, `
		INSERT INTO vercel_resources (
			resource_id, installation_id, product_slug, billing_plan_id,
			xata_project_id, name, metadata
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb)
		RETURNING status, created_at, updated_at
	`,
		created.ResourceID, created.InstallationID, created.ProductSlug, created.BillingPlanID,
		created.XataProjectID, created.Name, string(metadataJSON),
	).Scan(&created.Status, &created.CreatedAt, &created.UpdatedAt)
	if IsConstraintError(err, UniqueConstraintVercelResourcePK) {
		return nil, store.ErrVercelResourceAlreadyExists{ResourceID: created.ResourceID}
	}
	if IsConstraintError(err, UniqueConstraintVercelResourceProject) {
		return nil, store.ErrVercelResourceProjectLinked{XataProjectID: created.XataProjectID}
	}
	if err != nil {
		return nil, fmt.Errorf("create vercel resource: %w", err)
	}
	return created, nil
}

func (s *sqlAuthStore) GetVercelResource(ctx context.Context, installationID, resourceID string) (*store.VercelResource, error) {
	var (
		resource     store.VercelResource
		metadataJSON []byte
		deletedAt    sql.NullTime
	)
	err := s.sql.QueryRowContext(ctx, `
		SELECT resource_id, installation_id, product_slug, billing_plan_id,
			xata_project_id, name, metadata, status,
			created_at, updated_at, deleted_at
		FROM vercel_resources
		WHERE resource_id = $1 AND installation_id = $3 AND status != $2
	`, resourceID, store.VercelResourceDeleted, installationID).Scan(
		&resource.ResourceID, &resource.InstallationID, &resource.ProductSlug, &resource.BillingPlanID,
		&resource.XataProjectID, &resource.Name, &metadataJSON, &resource.Status,
		&resource.CreatedAt, &resource.UpdatedAt, &deletedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, store.ErrVercelResourceNotFound{ResourceID: resourceID}
	}
	if err != nil {
		return nil, fmt.Errorf("get vercel resource: %w", err)
	}
	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &resource.Metadata); err != nil {
			return nil, fmt.Errorf("unmarshal resource metadata: %w", err)
		}
	}
	if deletedAt.Valid {
		resource.DeletedAt = &deletedAt.Time
	}
	return &resource, nil
}

func (s *sqlAuthStore) TriggerVercelResourceDeletion(ctx context.Context, installationID, resourceID string) error {
	var (
		prev      sql.NullString
		didUpdate bool
	)
	err := s.sql.QueryRowContext(ctx, `
		WITH current AS (
			SELECT status FROM vercel_resources WHERE resource_id = $1 AND installation_id = $4
		), updated AS (
			UPDATE vercel_resources SET status = $2, updated_at = now()
			WHERE resource_id = $1 AND installation_id = $4 AND status = $3
			RETURNING 1
		)
		SELECT (SELECT status FROM current), EXISTS (SELECT 1 FROM updated)
	`, resourceID, store.VercelResourceDeleting, store.VercelResourceActive, installationID).Scan(&prev, &didUpdate)
	if err != nil {
		return fmt.Errorf("trigger vercel resource deletion: %w", err)
	}
	return classifyVercelDeletion(prev, didUpdate,
		store.ErrVercelResourceNotFound{ResourceID: resourceID},
		store.ErrVercelResourceNotActive{ResourceID: resourceID},
	)
}

// classifyVercelDeletion turns the pre-update status and whether the delete
// UPDATE matched a row into the right error. A matched update is success;
// a missing or terminally deleted row is notFound; anything still present is
// notActive (deletion already in progress).
func classifyVercelDeletion(prev sql.NullString, didUpdate bool, notFound, notActive error) error {
	switch {
	case didUpdate:
		return nil
	case !prev.Valid || store.VercelResourceStatus(prev.String) == store.VercelResourceDeleted:
		return notFound
	default:
		return notActive
	}
}

func (s *sqlAuthStore) AddVercelResourceBranch(ctx context.Context, installationID string, branch *store.VercelResourceBranch) (*store.VercelResourceBranch, error) {
	return s.insertVercelResourceBranch(ctx, s.sql, installationID, branch)
}

func (s *sqlAuthStore) insertVercelResourceBranch(ctx context.Context, q rowQueryer, installationID string, branch *store.VercelResourceBranch) (*store.VercelResourceBranch, error) {
	created := &store.VercelResourceBranch{
		ResourceID:   branch.ResourceID,
		Scope:        branch.Scope,
		XataBranchID: branch.XataBranchID,
	}
	// Only add a branch when the owning resource is active. The resource row is
	// taken FOR SHARE so a concurrent TriggerVercelResourceDeletion serializes
	// against this insert. xata_branch_id is a logical ref (no FK), so there is no
	// cross-store branch existence check.
	err := q.QueryRowContext(ctx, `
		WITH resource AS (
			SELECT resource_id FROM vercel_resources
			WHERE resource_id = $2 AND installation_id = $5 AND status = $4
			FOR SHARE
		)
		INSERT INTO vercel_resource_branches (id, resource_id, scope, xata_branch_id)
		SELECT $1, resource.resource_id, $3, $6 FROM resource
		RETURNING id, status, created_at, updated_at
	`,
		idgen.Generate(), created.ResourceID, created.Scope, store.VercelResourceActive, installationID, created.XataBranchID,
	).Scan(&created.ID, &created.Status, &created.CreatedAt, &created.UpdatedAt)
	if IsConstraintError(err, UniqueConstraintVercelResourceBranchScope) {
		return nil, store.ErrVercelResourceBranchExists{ResourceID: created.ResourceID, Scope: created.Scope}
	}
	if IsConstraintError(err, UniqueConstraintVercelResourceBranchXataBranch) {
		return nil, store.ErrVercelResourceXataBranchLinked{ResourceID: created.ResourceID, XataBranchID: created.XataBranchID}
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil, s.classifyAddBranchFailure(ctx, q, installationID, created.ResourceID)
	}
	if err != nil {
		return nil, fmt.Errorf("add vercel resource branch: %w", err)
	}
	return created, nil
}

// classifyAddBranchFailure explains why the branch insert produced no row: the
// owning resource is missing/deleted (not-found) or deleting (not-active).
func (s *sqlAuthStore) classifyAddBranchFailure(ctx context.Context, q rowQueryer, installationID, resourceID string) error {
	var status store.VercelResourceStatus
	err := q.QueryRowContext(ctx,
		`SELECT status FROM vercel_resources WHERE resource_id = $1 AND installation_id = $2`, resourceID, installationID,
	).Scan(&status)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return store.ErrVercelResourceNotFound{ResourceID: resourceID}
	case err != nil:
		return fmt.Errorf("classify add vercel branch failure: %w", err)
	case status == store.VercelResourceDeleting:
		return store.ErrVercelResourceNotActive{ResourceID: resourceID}
	default:
		// Deleted (or any other non-active state) reads as absent.
		return store.ErrVercelResourceNotFound{ResourceID: resourceID}
	}
}

func (s *sqlAuthStore) CreateVercelResourceAndBranches(ctx context.Context, resource *store.VercelResource, branches []store.VercelResourceBranch) (*store.VercelResource, error) {
	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op after Commit

	created, err := s.insertVercelResource(ctx, tx, resource)
	if err != nil {
		return nil, err
	}
	for i := range branches {
		if _, err := s.insertVercelResourceBranch(ctx, tx, resource.InstallationID, &branches[i]); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit vercel resource and branches: %w", err)
	}
	return created, nil
}

// TryAcquireVercelResourceLock takes a NON-blocking session advisory lock on a
// pinned connection, serializing same-key provisions. On contention it returns
// ErrVercelResourceBusy rather than blocking. The returned closure releases the
// lock and closes the connection and must be called (defer) even on error.
func (s *sqlAuthStore) TryAcquireVercelResourceLock(ctx context.Context, resourceID string) (func() error, error) {
	conn, err := s.sql.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire connection: %w", err)
	}
	key := "vercel-resource:" + resourceID
	var acquired bool
	if err := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock(hashtext($1))`, key).Scan(&acquired); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("try advisory lock: %w", err)
	}
	if !acquired {
		_ = conn.Close()
		return nil, store.ErrVercelResourceBusy{ResourceID: resourceID}
	}
	release := func() error {
		// Unlock on the same connection, then return it to the pool. Use a fresh
		// context so release still runs if the caller's context is done.
		defer conn.Close()
		_, err := conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock(hashtext($1))`, key)
		if err != nil {
			return fmt.Errorf("release advisory lock: %w", err)
		}
		return nil
	}
	return release, nil
}

func (s *sqlAuthStore) ListVercelResourceBranches(ctx context.Context, installationID, resourceID string) ([]store.VercelResourceBranch, error) {
	rows, err := s.sql.QueryContext(ctx, `
		SELECT b.id, b.resource_id, b.scope, b.xata_branch_id, b.status, b.created_at, b.updated_at, b.deleted_at
		FROM vercel_resource_branches b
		JOIN vercel_resources r ON r.resource_id = b.resource_id AND r.installation_id = $3
		WHERE b.resource_id = $1 AND b.status != $2
		ORDER BY b.created_at, b.id
	`, resourceID, store.VercelResourceDeleted, installationID)
	if err != nil {
		return nil, fmt.Errorf("list vercel resource branches: %w", err)
	}
	defer rows.Close()

	branches := []store.VercelResourceBranch{}
	for rows.Next() {
		var (
			b         store.VercelResourceBranch
			deletedAt sql.NullTime
		)
		if err := rows.Scan(&b.ID, &b.ResourceID, &b.Scope, &b.XataBranchID, &b.Status, &b.CreatedAt, &b.UpdatedAt, &deletedAt); err != nil {
			return nil, fmt.Errorf("scan vercel resource branch: %w", err)
		}
		if deletedAt.Valid {
			b.DeletedAt = &deletedAt.Time
		}
		branches = append(branches, b)
	}
	return branches, rows.Err()
}

func (s *sqlAuthStore) TriggerVercelResourceBranchDeletion(ctx context.Context, installationID, resourceID, scope string) error {
	var (
		prev      sql.NullString
		didUpdate bool
	)
	err := s.sql.QueryRowContext(ctx, `
		WITH current AS (
			SELECT b.status FROM vercel_resource_branches b
			JOIN vercel_resources r ON r.resource_id = b.resource_id AND r.installation_id = $6
			WHERE b.resource_id = $1 AND b.scope = $2 AND b.status != $5
		), updated AS (
			UPDATE vercel_resource_branches b SET status = $3, updated_at = now()
			FROM vercel_resources r
			WHERE b.resource_id = $1 AND b.scope = $2 AND b.status = $4
				AND r.resource_id = b.resource_id AND r.installation_id = $6
			RETURNING 1
		)
		SELECT (SELECT status FROM current), EXISTS (SELECT 1 FROM updated)
	`, resourceID, scope, store.VercelResourceDeleting, store.VercelResourceActive, store.VercelResourceDeleted, installationID).Scan(&prev, &didUpdate)
	if err != nil {
		return fmt.Errorf("trigger vercel resource branch deletion: %w", err)
	}
	return classifyVercelDeletion(prev, didUpdate,
		store.ErrVercelResourceBranchNotFound{ResourceID: resourceID, Scope: scope},
		store.ErrVercelResourceBranchNotActive{ResourceID: resourceID, Scope: scope},
	)
}
