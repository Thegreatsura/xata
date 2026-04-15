package store

import (
	"context"
	"time"

	"xata/internal/api/key"
)

//go:generate go run github.com/vektra/mockery/v3 --with-expecter --name AuthStore

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
	CreateAPIKey(ctx context.Context, targetType KeyTargetType, targetID string, key *APIKeyCreate) (key.Key, *APIKey, error)
	DeleteAPIKeys(ctx context.Context, targetType KeyTargetType, targetID string, keyIDs []string) error
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
