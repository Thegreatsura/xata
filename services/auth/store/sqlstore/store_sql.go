package sqlstore

import (
	"bytes"
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"

	"xata/internal/api/key"
	"xata/internal/idgen"
	"xata/internal/o11y"
	"xata/internal/pgroll"
	"xata/services/auth/store"
)

//go:embed migrations/*.json
var migrationsFS embed.FS

// check if sqlAuthStore implements the AuthStore interface
var _ store.AuthStore = (*sqlAuthStore)(nil)

const (
	// Unique constraint name for API keys
	UniqueConstraintKeyName = "unique_api_key_name"
	// Partial unique index enforcing one active installation per Vercel account
	UniqueConstraintVercelInstallationAccount = "unique_active_vercel_installation_account"
)

const (
	maxOpenConns    = 25
	maxIdleConns    = 10
	connMaxLifetime = 30 * time.Minute
	connMaxIdleTime = 5 * time.Minute
)

type sqlAuthStore struct {
	config Config
	sql    *sql.DB
	pgroll *pgroll.PGRoll
}

func NewSQLAuthStore(ctx context.Context, cfg Config) (*sqlAuthStore, error) {
	// set search path to the latest known version
	pgroll, err := pgroll.FromEmbeddedFS(&migrationsFS)
	if err != nil {
		return nil, fmt.Errorf("failed to create pgroll: %w", err)
	}

	// connect to the database (with the latest schema version)
	latest := pgroll.LatestVersionSchema(ctx)
	db, err := sql.Open("postgres", cfg.ConnectionString()+"&search_path="+latest)
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}
	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxIdleConns)
	db.SetConnMaxLifetime(connMaxLifetime)
	db.SetConnMaxIdleTime(connMaxIdleTime)

	return &sqlAuthStore{
		sql:    db,
		config: cfg,
		pgroll: pgroll,
	}, nil
}

// Setup runs DB migrations for the store
func (s *sqlAuthStore) Setup(ctx context.Context) error {
	// TODO move this to its own package (+ CLI tool?)
	logger := o11y.Ctx(ctx).Logger()
	logger.Info().Msg("Running DB migrations")

	err := s.pgroll.ApplyMigrations(ctx, s.config.ConnectionString())
	if err != nil {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}
	return nil
}

func (s *sqlAuthStore) Close(ctx context.Context) error {
	return s.sql.Close()
}

// scanAPIKey scans a single row into an APIKey
func scanAPIKey(row *sql.Row) (*store.APIKey, error) {
	if err := row.Err(); err != nil {
		return nil, fmt.Errorf("query execution error: %w", err)
	}

	apiKey, err := scanAPIKeyFrom(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, &store.ErrAPIKeyNotFound{ID: ""}
		}
		return nil, err
	}

	return apiKey, nil
}

// scanAPIKeyFrom scans using a generic scanner to avoid duplicate logic.
func scanAPIKeyFrom(scanner interface{ Scan(dest ...any) error }) (*store.APIKey, error) {
	var (
		apiKey           store.APIKey
		nullExpiry       sql.NullTime
		nullLastUsed     sql.NullTime
		nullCreatedBy    sql.NullString
		nullCreatedByKey sql.NullString
	)

	if err := scanner.Scan(
		&apiKey.ID,
		&apiKey.Name,
		&apiKey.KeyHash,
		&apiKey.KeyPreview,
		&apiKey.TargetType,
		&apiKey.TargetID,
		&nullExpiry,
		&apiKey.CreatedAt,
		&nullLastUsed,
		pq.Array(&apiKey.Scopes),
		pq.Array(&apiKey.Projects),
		pq.Array(&apiKey.Branches),
		&nullCreatedBy,
		&nullCreatedByKey,
	); err != nil {
		return nil, fmt.Errorf("failed to scan API key: %w", err)
	}

	if nullExpiry.Valid {
		apiKey.Expiry = &nullExpiry.Time
	}

	if nullLastUsed.Valid {
		apiKey.LastUsed = &nullLastUsed.Time
	}

	if nullCreatedBy.Valid {
		apiKey.CreatedBy = &nullCreatedBy.String
	}

	if nullCreatedByKey.Valid {
		apiKey.CreatedByKey = &nullCreatedByKey.String
	}

	return &apiKey, nil
}

// scanAPIKeys scans rows into APIKey structs
func scanAPIKeys(rows *sql.Rows) ([]store.APIKey, error) {
	var apiKeys []store.APIKey

	// Check for any errors that occurred during query execution before iterating
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("query execution error: %w", err)
	}

	for rows.Next() {
		apiKey, err := scanAPIKeyFrom(rows)
		if err != nil {
			return nil, err
		}
		apiKeys = append(apiKeys, *apiKey)
	}

	// Check for any errors encountered during iteration
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return apiKeys, nil
}

// ValidateAPIKey validates the provided API Key.
func (s *sqlAuthStore) ValidateAPIKey(ctx context.Context, apiKey key.Key) (*store.APIKey, error) {
	{
		// Use HMAC lookup for fast validation
		hashedKey := apiKey.HashKey(s.config.HMACSecret)

		row := s.sql.QueryRowContext(ctx, `
		SELECT id, name, key_hash, key_preview, target_type, target_id, expiry, created_at, last_used,
		       scopes, projects, branches, created_by, created_by_key
		FROM api_keys
		WHERE key_hash = $1
	`, hashedKey)

		apiKey, err := scanAPIKey(row)
		if err != nil {
			return nil, &store.ErrInvalidAPIKey{}
		}

		// Check if the API key has expired
		if apiKey.Expiry != nil && apiKey.Expiry.Before(time.Now()) {
			return nil, &store.ErrInvalidAPIKey{}
		}

		// Update last_used timestamp in the background
		go func(apiKeyID string, parentCtx context.Context) {
			updateCtx, cancel := context.WithTimeout(parentCtx, 5*time.Second)
			defer cancel()

			_, err := s.sql.ExecContext(updateCtx, "UPDATE api_keys SET last_used = NOW() WHERE id = $1", apiKeyID)
			if err != nil {
				// Non-critical error, just log it
				logger := o11y.Ctx(parentCtx).Logger()
				logger.Error().Err(err).Str("api_key_id", apiKeyID).Msg("Failed to update last_used timestamp")
			}
		}(apiKey.ID, ctx)

		return apiKey, nil
	}
}

// DeleteAPIKeys deletes API keys by their IDs for a specific target type and target ID.
func (s *sqlAuthStore) DeleteAPIKeys(ctx context.Context, targetType store.KeyTargetType, targetID string, keyIDs []string) error {
	if len(keyIDs) == 0 {
		return nil
	}

	_, err := s.sql.ExecContext(ctx, `
		DELETE FROM api_keys
		WHERE id = ANY($1) AND target_type = $2 AND target_id = $3
	`, pq.Array(keyIDs), targetType, targetID)
	if err != nil {
		return err
	}

	return nil
}

// GetAPIKey retrieves an API key by its ID.
func (s *sqlAuthStore) GetAPIKey(ctx context.Context, id string) (*store.APIKey, error) {
	row := s.sql.QueryRowContext(ctx, `
		SELECT id, name, key_hash, key_preview, target_type, target_id, expiry, created_at, last_used,
		       scopes, projects, branches, created_by, created_by_key
		FROM api_keys
		WHERE id = $1
	`, id)

	apiKey, err := scanAPIKey(row)
	if err != nil {
		if _, ok := errors.AsType[*store.ErrAPIKeyNotFound](err); ok {
			return nil, &store.ErrAPIKeyNotFound{ID: id}
		}
		return nil, fmt.Errorf("get API key: %w", err)
	}
	return apiKey, nil
}

// ListAPIKeys retrieves all API keys for a specific target type and target ID.
func (s *sqlAuthStore) ListAPIKeys(ctx context.Context, targetType store.KeyTargetType, targetID string) ([]store.APIKey, error) {
	rows, err := s.sql.QueryContext(ctx, `
		SELECT id, name, key_hash, key_preview, target_type, target_id, expiry, created_at, last_used,
		       scopes, projects, branches, created_by, created_by_key
		FROM api_keys
		WHERE target_type = $1 AND target_id = $2
	`, targetType, targetID)
	if err != nil {
		return nil, fmt.Errorf("failed to query API keys: %w", err)
	}
	defer rows.Close()

	return scanAPIKeys(rows)
}

// generateAPIKey creates a new API key based on the target type.
func (s *sqlAuthStore) generateAPIKey(targetType store.KeyTargetType) (key.Key, error) {
	switch targetType {
	case store.KeyTargetOrganization:
		return key.NewOrganizationKey()
	case store.KeyTargetUser:
		return key.NewUserKey()
	default:
		return "", &store.ErrUnsupportedTargetType{TargetType: string(targetType)}
	}
}

// CreateAPIKey creates a new API key for a specific target type and target ID.
func (s *sqlAuthStore) CreateAPIKey(ctx context.Context, targetType store.KeyTargetType, targetID string, keyInfo *store.APIKeyCreate) (key.Key, *store.APIKey, error) {
	// Validate expiry time if provided
	if keyInfo.Expiry != nil && keyInfo.Expiry.Before(time.Now()) {
		return "", nil, &store.ErrAPIKeyExpiresInPast{Expiry: keyInfo.Expiry}
	}

	// Validate scopes
	if len(keyInfo.Scopes) > store.MaxScopesPerAPIKey {
		return "", nil, &store.ErrAPIKeyScopesLimitReached{Limit: store.MaxScopesPerAPIKey}
	}
	if len(keyInfo.Scopes) == 0 {
		keyInfo.Scopes = []string{"*"}
	}

	// Validate projects
	if len(keyInfo.Projects) > store.MaxProjectsPerAPIKey {
		return "", nil, &store.ErrAPIKeyProjectsLimitReached{Limit: store.MaxProjectsPerAPIKey}
	}
	if len(keyInfo.Projects) == 0 {
		keyInfo.Projects = []string{"*"}
	}

	// Validate branches
	if len(keyInfo.Branches) > store.MaxBranchesPerAPIKey {
		return "", nil, &store.ErrAPIKeyBranchesLimitReached{Limit: store.MaxBranchesPerAPIKey}
	}
	if len(keyInfo.Branches) == 0 {
		keyInfo.Branches = []string{"*"}
	}

	rawKey, err := s.generateAPIKey(targetType)
	if err != nil {
		return "", nil, err
	}

	hashedKey := rawKey.HashKey(s.config.HMACSecret)

	var expiry sql.NullTime
	if keyInfo.Expiry != nil {
		expiry.Valid = true
		expiry.Time = *keyInfo.Expiry
	}

	tx, err := s.sql.BeginTx(ctx, nil)
	if err != nil {
		return "", nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Enforce maximum number of API keys per target within the transaction
	var count int
	err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM api_keys WHERE target_type = $1 AND target_id = $2`, targetType, targetID).Scan(&count)
	if err != nil {
		return "", nil, fmt.Errorf("failed to count API keys: %w", err)
	}
	if count >= store.MaxAPIKeysPerTarget {
		return "", nil, &store.ErrAPIKeyLimitReached{Limit: store.MaxAPIKeysPerTarget}
	}

	var createdBy, createdByKey *string
	if keyInfo.CreatedBy != nil && *keyInfo.CreatedBy != "" {
		createdBy = keyInfo.CreatedBy
	} else {
		createdBy = nil
	}
	if keyInfo.CreatedByKey != nil && *keyInfo.CreatedByKey != "" {
		createdByKey = keyInfo.CreatedByKey
	} else {
		createdByKey = nil
	}

	row := tx.QueryRowContext(ctx, `
		INSERT INTO api_keys (id, name, key_hash, key_preview, target_type, target_id, expiry, created_at, last_used,
							  scopes, projects, branches, created_by, created_by_key)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NULL, $8, $9, $10, $11, $12)
		RETURNING id, name, key_hash, key_preview, target_type, target_id, expiry, created_at, last_used,
				  scopes, projects, branches, created_by, created_by_key
	`, idgen.Generate(), keyInfo.Name, hashedKey, rawKey.Obfuscate(key.DefaultObfuscateCharsCount),
		targetType, targetID, expiry,
		pq.Array(keyInfo.Scopes), pq.Array(keyInfo.Projects), pq.Array(keyInfo.Branches), createdBy, createdByKey,
	)

	apiKey, err := scanAPIKey(row)
	if err != nil {
		// API key already exists
		if IsConstraintError(err, UniqueConstraintKeyName) {
			return "", nil, &store.ErrAPIKeyAlreadyExists{Name: keyInfo.Name}
		}

		return "", nil, err
	}

	if err := tx.Commit(); err != nil {
		return "", nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return rawKey, apiKey, nil
}

// IsConstraintError checks if a given constraint was not met
func IsConstraintError(err error, constraint string) bool {
	if pqErr, ok := errors.AsType[*pq.Error](err); ok {
		return pqErr.Code == "23505" && pqErr.Constraint == constraint
	}
	return false
}

func decodeLimits[K ~string](raw []byte) (map[K]any, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var m map[K]any
	if err := d.Decode(&m); err != nil {
		return nil, err
	}
	return m, nil
}

// GetOrgLimits returns the stored limit overrides for an organization.
func (s *sqlAuthStore) GetOrgLimits(ctx context.Context, orgID string) (map[store.OrgLimitKey]any, error) {
	var raw []byte
	err := s.sql.QueryRowContext(ctx, `
		SELECT limits
		FROM organization_limits
		WHERE organization_id = $1
	`, orgID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return map[store.OrgLimitKey]any{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query org limits: %w", err)
	}
	limits, err := decodeLimits[store.OrgLimitKey](raw)
	if err != nil {
		return nil, fmt.Errorf("unmarshal org limits: %w", err)
	}
	return limits, nil
}

// SetOrgLimit upserts an override for a single organization limit.
func (s *sqlAuthStore) SetOrgLimit(ctx context.Context, orgID string, key store.OrgLimitKey, value any) error {
	if !key.IsValid() {
		return fmt.Errorf("unknown org limit key %q", key)
	}
	valueJSON, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal limit value: %w", err)
	}
	_, err = s.sql.ExecContext(ctx, `
		INSERT INTO organization_limits (organization_id, limits)
		VALUES ($1, jsonb_build_object($2::text, $3::jsonb))
		ON CONFLICT (organization_id) DO UPDATE
		SET limits = organization_limits.limits || jsonb_build_object($2::text, $3::jsonb)
	`, orgID, key, valueJSON)
	if err != nil {
		return fmt.Errorf("set org limit: %w", err)
	}
	return nil
}

// DeleteOrgLimit removes an override for a single organization limit.
func (s *sqlAuthStore) DeleteOrgLimit(ctx context.Context, orgID string, key store.OrgLimitKey) error {
	_, err := s.sql.ExecContext(ctx, `
		UPDATE organization_limits
		SET limits = limits - $2::text
		WHERE organization_id = $1
	`, orgID, key)
	if err != nil {
		return fmt.Errorf("delete org limit: %w", err)
	}
	return nil
}

// UpsertVercelInstallation inserts or updates a Vercel installation. The access
// token is persisted verbatim; the caller is responsible for encrypting it.
//
// On conflict only mutable fields (access_token, scopes, accepted_policies) are
// refreshed, and only while the existing row is active. The identity link
// (vercel_account_id, xata_organization_id) and created_at are write-once, and
// the lifecycle (status, deleted_at) is never touched here — those transitions
// go through TriggerVercelInstallationDeletion — so a re-sent Upsert can neither
// re-point an installation nor change its status. If the existing row is
// deleting or deleted the update is refused with ErrVercelInstallationNotActive
// rather than silently resurrecting the uninstall — a re-install must be handled
// explicitly by the caller.
func (s *sqlAuthStore) UpsertVercelInstallation(ctx context.Context, installation *store.VercelInstallation) error {
	scopes := installation.Scopes
	if scopes == nil {
		scopes = []string{}
	}
	policies := installation.AcceptedPolicies
	if policies == nil {
		policies = map[string]time.Time{}
	}
	status := installation.Status
	if status == "" {
		status = store.VercelInstallationActive
	}
	policiesJSON, err := json.Marshal(policies)
	if err != nil {
		return fmt.Errorf("marshal accepted policies: %w", err)
	}
	err = s.sql.QueryRowContext(ctx, `
		INSERT INTO vercel_installations (
			installation_id, vercel_account_id, xata_organization_id, access_token,
			scopes, accepted_policies, status, deleted_at
		)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8)
		ON CONFLICT (installation_id) DO UPDATE SET
			access_token      = EXCLUDED.access_token,
			scopes            = EXCLUDED.scopes,
			accepted_policies = EXCLUDED.accepted_policies,
			updated_at        = now()
		WHERE vercel_installations.status = $9
		RETURNING status, created_at, updated_at
	`,
		installation.InstallationID,
		installation.VercelAccountID,
		installation.XataOrganizationID,
		installation.AccessToken,
		pq.Array(scopes),
		string(policiesJSON),
		status,
		installation.DeletedAt,
		store.VercelInstallationActive,
	).Scan(&installation.Status, &installation.CreatedAt, &installation.UpdatedAt)
	// No row returned on conflict means the DO UPDATE was skipped by the WHERE:
	// the installation exists but is not active, so we refuse to resurrect it.
	if errors.Is(err, sql.ErrNoRows) {
		return store.ErrVercelInstallationNotActive{InstallationID: installation.InstallationID}
	}
	if IsConstraintError(err, UniqueConstraintVercelInstallationAccount) {
		return store.ErrVercelAccountAlreadyLinked{
			InstallationID:  installation.InstallationID,
			VercelAccountID: installation.VercelAccountID,
		}
	}
	if err != nil {
		return fmt.Errorf("upsert vercel installation: %w", err)
	}
	// Reflect the coalesced defaults back so callers don't observe nil values
	// after a successful upsert (Status and the timestamps come from RETURNING).
	installation.Scopes = scopes
	installation.AcceptedPolicies = policies
	return nil
}

// GetVercelInstallation returns active and deleting installations; the
// deleting row is kept visible so billing can finalize after uninstall.
// Callers must still inspect Status, since a returned installation may
// be deleting rather than active.
func (s *sqlAuthStore) GetVercelInstallation(ctx context.Context, installationID string) (*store.VercelInstallation, error) {
	var (
		inst         store.VercelInstallation
		policiesJSON []byte
		deletedAt    sql.NullTime
	)
	err := s.sql.QueryRowContext(ctx, `
		SELECT installation_id, vercel_account_id, xata_organization_id, access_token,
			scopes, accepted_policies, status, created_at, updated_at, deleted_at
		FROM vercel_installations
		WHERE installation_id = $1 AND status != $2
	`, installationID, store.VercelInstallationDeleted).Scan(
		&inst.InstallationID,
		&inst.VercelAccountID,
		&inst.XataOrganizationID,
		&inst.AccessToken,
		pq.Array(&inst.Scopes),
		&policiesJSON,
		&inst.Status,
		&inst.CreatedAt,
		&inst.UpdatedAt,
		&deletedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, store.ErrVercelInstallationNotFound{InstallationID: installationID}
	}
	if err != nil {
		return nil, fmt.Errorf("get vercel installation: %w", err)
	}
	if len(policiesJSON) > 0 {
		if err := json.Unmarshal(policiesJSON, &inst.AcceptedPolicies); err != nil {
			return nil, fmt.Errorf("unmarshal accepted policies: %w", err)
		}
	}
	if deletedAt.Valid {
		inst.DeletedAt = &deletedAt.Time
	}
	return &inst, nil
}

// TriggerVercelInstallationDeletion moves an active installation into the
// deleting state, the visible window in which billing can finalize after an
// uninstall. It does not stamp deleted_at — that belongs to the terminal deleted
// transition, which a later PR will add once the teardown requirements are
// settled. It is strict about the source state:
//   - active               -> transitions to deleting
//   - already deleting      -> ErrVercelInstallationAlreadyDeleting (idempotent
//     to the caller, but signalled so teardown is not re-run)
//   - deleted / missing     -> ErrVercelInstallationNotFound (deleted rows are
//     treated as absent, matching GetVercelInstallation)
func (s *sqlAuthStore) TriggerVercelInstallationDeletion(ctx context.Context, installationID string) error {
	res, err := s.sql.ExecContext(ctx, `
		UPDATE vercel_installations
		SET status = $2, updated_at = now()
		WHERE installation_id = $1 AND status = $3
	`, installationID, store.VercelInstallationDeleting, store.VercelInstallationActive)
	if err != nil {
		return fmt.Errorf("trigger vercel installation deletion: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if affected > 0 {
		return nil // transitioned active -> deleting
	}

	// No active row matched: disambiguate why so the caller gets a precise
	// signal (already deleting vs. absent).
	var current store.VercelInstallationStatus
	err = s.sql.QueryRowContext(ctx, `
		SELECT status FROM vercel_installations WHERE installation_id = $1
	`, installationID).Scan(&current)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return store.ErrVercelInstallationNotFound{InstallationID: installationID}
	case err != nil:
		return fmt.Errorf("get vercel installation status: %w", err)
	case current == store.VercelInstallationDeleting:
		return store.ErrVercelInstallationAlreadyDeleting{InstallationID: installationID}
	default:
		// Deleted (or any other non-active state) is treated as absent.
		return store.ErrVercelInstallationNotFound{InstallationID: installationID}
	}
}
