package store

import (
	"context"
	"time"

	"xata/internal/api/key"
)

//go:generate go run github.com/vektra/mockery/v2 --with-expecter --name AuthStore

// AuthStore stores information about authentication
type AuthStore interface {
	// Setup runs DB migrations for the store
	Setup(ctx context.Context) error

	// Close closes the store
	Close(ctx context.Context) error

	// ValidateAPIKey retrieves the claims associated with an API Key
	ValidateAPIKey(ctx context.Context, apiKey key.Key) (*APIKey, error)

	// API Key operations
	ListAPIKeys(ctx context.Context, targetType KeyTargetType, targetID string) ([]APIKey, error)
	GetAPIKey(ctx context.Context, id string) (*APIKey, error)
	CreateAPIKey(ctx context.Context, targetType KeyTargetType, targetID string, key *APIKeyCreate) (key.Key, *APIKey, error)
	DeleteAPIKeys(ctx context.Context, targetType KeyTargetType, targetID string, keyIDs []string) error

	// Org limit operations
	GetOrgLimits(ctx context.Context, orgID string) (map[OrgLimitKey]any, error)
	SetOrgLimit(ctx context.Context, orgID string, key OrgLimitKey, value any) error
	DeleteOrgLimit(ctx context.Context, orgID string, key OrgLimitKey) error

	// UpsertVercelInstallation inserts or updates an installation by id;
	// AccessToken is stored as-is (already encrypted by the caller).
	UpsertVercelInstallation(ctx context.Context, installation *VercelInstallation) error
	// GetVercelInstallation returns active and deleting installations; the
	// deleting row is kept visible so billing can finalize after uninstall.
	// Callers must still inspect Status, since a returned installation may
	// be deleting rather than active.
	GetVercelInstallation(ctx context.Context, installationID string) (*VercelInstallation, error)
	// TriggerVercelInstallationDeletion flags an installation as deleting on
	// uninstall. It stays retrievable in this state so billing can finalize
	TriggerVercelInstallationDeletion(ctx context.Context, installationID string) error

	// Vercel Marketplace resources. XataProjectID/XataBranchID are logical
	// references to the projects service's database (no FK): the project/branch is
	// provisioned via the projects gRPC first, so these methods do not validate its
	// existence. Every method is scoped by installationID.

	// CreateVercelResource inserts a resource and returns the stored row. The input
	// is not modified; a nil Metadata is stored as an empty object.
	CreateVercelResource(ctx context.Context, resource *VercelResource) (*VercelResource, error)
	// CreateVercelResourceAndBranches inserts a resource and all of its branch rows
	// in one transaction, so a partial write can't leave a resource missing a scope.
	CreateVercelResourceAndBranches(ctx context.Context, resource *VercelResource, branches []VercelResourceBranch) (*VercelResource, error)
	// TryAcquireVercelResourceLock takes a NON-blocking advisory lock scoped to a
	// resource id, serializing same-key provisions. On contention it returns
	// ErrVercelResourceBusy (→ 429). The returned closure releases the lock and must
	// be called (defer) even on error.
	TryAcquireVercelResourceLock(ctx context.Context, resourceID string) (func() error, error)
	// GetVercelResource returns active and deleting resources (deleted rows read as
	// absent). Callers must inspect Status.
	GetVercelResource(ctx context.Context, installationID, resourceID string) (*VercelResource, error)
	// TriggerVercelResourceDeletion flags an active resource as deleting.
	TriggerVercelResourceDeletion(ctx context.Context, installationID, resourceID string) error
	// AddVercelResourceBranch adds a branch (any scope) to an active resource and
	// returns the stored row with its generated id. The input is not modified.
	AddVercelResourceBranch(ctx context.Context, installationID string, branch *VercelResourceBranch) (*VercelResourceBranch, error)
	// ListVercelResourceBranches returns a resource's non-deleted branches.
	ListVercelResourceBranches(ctx context.Context, installationID, resourceID string) ([]VercelResourceBranch, error)
	// TriggerVercelResourceBranchDeletion flags an active branch (by resource and
	// scope) as deleting.
	TriggerVercelResourceBranchDeletion(ctx context.Context, installationID, resourceID, scope string) error
}

// OrgLimitKey identifies an organization-level limit stored in the auth service.
type OrgLimitKey string

const (
	OrgLimitMaxMembers OrgLimitKey = "max_members"
	OrgLimitMaxInvites OrgLimitKey = "max_invites"
)

func (k OrgLimitKey) IsValid() bool {
	switch k {
	case OrgLimitMaxMembers, OrgLimitMaxInvites:
		return true
	}
	return false
}

// KeyTargetType represents the type of entity the API key is associated with
type KeyTargetType string

const (
	KeyTargetOrganization KeyTargetType = "organization"
	KeyTargetUser         KeyTargetType = "user"

	// MaxAPIKeysPerTarget is the maximum number of API keys that can exist for a user or organization
	MaxAPIKeysPerTarget = 50

	// MaxScopesPerAPIKey is the maximum number of scopes that can be assigned to an API key
	MaxScopesPerAPIKey = 50

	// MaxProjectsPerAPIKey is the maximum number of projects that can be assigned to an API key
	MaxProjectsPerAPIKey = 50

	// MaxBranchesPerAPIKey is the maximum number of branches that can be assigned to an API key
	MaxBranchesPerAPIKey = 50
)

type APIKey struct {
	ID           string
	Name         string
	KeyHash      string
	KeyPreview   string
	TargetType   KeyTargetType
	TargetID     string
	Expiry       *time.Time
	CreatedAt    time.Time
	LastUsed     *time.Time
	Scopes       []string
	Projects     []string
	Branches     []string
	CreatedBy    *string
	CreatedByKey *string
}

type APIKeyCreate struct {
	Name         string
	Expiry       *time.Time
	Scopes       []string
	Projects     []string
	Branches     []string
	CreatedBy    *string
	CreatedByKey *string
}
