package store

import (
	"fmt"
	"net/http"
	"time"
)

// ErrAPIKeyNotFound is returned when an API key cannot be found
type ErrAPIKeyNotFound struct {
	ID string
}

func (e ErrAPIKeyNotFound) Error() string {
	return fmt.Sprintf("API key [%s] not found", e.ID)
}

func (e ErrAPIKeyNotFound) StatusCode() int {
	return http.StatusNotFound
}

// ErrVercelInstallationNotFound is returned when a Vercel installation cannot be found
type ErrVercelInstallationNotFound struct {
	InstallationID string
}

func (e ErrVercelInstallationNotFound) Error() string {
	return fmt.Sprintf("vercel installation [%s] not found", e.InstallationID)
}

func (e ErrVercelInstallationNotFound) StatusCode() int {
	return http.StatusNotFound
}

// ErrVercelInstallationNotActive is returned when an upsert conflicts with an
// installation that is no longer active (deleting or deleted). The store refuses
// to silently resurrect it — the caller must decide how to handle a re-install
// (e.g. provision a fresh org) explicitly.
type ErrVercelInstallationNotActive struct {
	InstallationID string
}

func (e ErrVercelInstallationNotActive) Error() string {
	return fmt.Sprintf("vercel installation [%s] is not active (deleting or deleted)", e.InstallationID)
}

func (e ErrVercelInstallationNotActive) StatusCode() int {
	return http.StatusConflict
}

// ErrVercelInstallationAlreadyDeleting is returned when deletion is triggered on
// an installation whose deletion is already in progress. Callers should treat a
// duplicate uninstall as idempotent (not re-run teardown), not as a hard failure.
type ErrVercelInstallationAlreadyDeleting struct {
	InstallationID string
}

func (e ErrVercelInstallationAlreadyDeleting) Error() string {
	return fmt.Sprintf("vercel installation [%s] is already deleting", e.InstallationID)
}

func (e ErrVercelInstallationAlreadyDeleting) StatusCode() int {
	return http.StatusConflict
}

// ErrVercelAccountAlreadyLinked is returned when a Vercel account already has an
// active installation. A Vercel team maps to at most one active installation,
// enforced by a partial unique index; a deleting or deleted installation does
// not reserve the account, so a re-install over a finalizing uninstall succeeds.
type ErrVercelAccountAlreadyLinked struct {
	InstallationID  string
	VercelAccountID string
}

func (e ErrVercelAccountAlreadyLinked) Error() string {
	return fmt.Sprintf("vercel account [%s] already has an active installation", e.VercelAccountID)
}

func (e ErrVercelAccountAlreadyLinked) StatusCode() int {
	return http.StatusConflict
}

// ErrInvalidAPIKey is returned when an API key is invalid
type ErrInvalidAPIKey struct{}

func (e ErrInvalidAPIKey) Error() string {
	return "invalid API key"
}

func (e ErrInvalidAPIKey) StatusCode() int {
	return http.StatusUnauthorized
}

// ErrInvalidJWTToken is returned when a JWT token is invalid
type ErrInvalidJWTToken struct{}

func (e ErrInvalidJWTToken) Error() string {
	return "invalid JWT token"
}

func (e ErrInvalidJWTToken) StatusCode() int {
	return http.StatusUnauthorized
}

// ErrInvalidAPIKeyTargetType is returned when an API key has an invalid target type
type ErrInvalidAPIKeyTargetType struct {
	TargetType string
}

func (e ErrInvalidAPIKeyTargetType) Error() string {
	return fmt.Sprintf("invalid API key target type: %s", e.TargetType)
}

func (e ErrInvalidAPIKeyTargetType) StatusCode() int {
	return http.StatusBadRequest
}

// ErrUnsupportedTargetType is returned when an unsupported target type is specified
type ErrUnsupportedTargetType struct {
	TargetType string
}

func (e ErrUnsupportedTargetType) Error() string {
	return fmt.Sprintf("unsupported target type: %s", e.TargetType)
}

func (e ErrUnsupportedTargetType) StatusCode() int {
	return http.StatusBadRequest
}

// ErrFailedToDecodeJWT is returned when JWT token cannot be decoded
type ErrFailedToDecodeJWT struct {
	Err error
}

func (e ErrFailedToDecodeJWT) Error() string {
	return fmt.Sprintf("failed to decode JWT token: %v", e.Err)
}

func (e ErrFailedToDecodeJWT) StatusCode() int {
	return http.StatusUnauthorized
}

// ErrAPIKeyAlreadyExists is returned when an API key already exists
type ErrAPIKeyAlreadyExists struct {
	Name string
}

func (e ErrAPIKeyAlreadyExists) Error() string {
	return fmt.Sprintf("API key with name '%s' already exists", e.Name)
}

func (e ErrAPIKeyAlreadyExists) StatusCode() int {
	return http.StatusConflict
}

// ErrAPIKeyExpiresInPast is returned when an API key's expiration date is in the past
type ErrAPIKeyExpiresInPast struct {
	Expiry *time.Time
}

func (e ErrAPIKeyExpiresInPast) Error() string {
	return fmt.Sprintf("API key expiration date is in the past: %s", e.Expiry.Format(time.RFC3339))
}

func (e ErrAPIKeyExpiresInPast) StatusCode() int {
	return http.StatusBadRequest
}

// ErrAPIKeyLimitReached is returned when a user or organization has reached the maximum number of API keys
type ErrAPIKeyLimitReached struct {
	Limit int
}

func (e ErrAPIKeyLimitReached) Error() string {
	return fmt.Sprintf("API key limit of %d reached", e.Limit)
}

func (e ErrAPIKeyLimitReached) StatusCode() int {
	return http.StatusBadRequest
}

// ErrAPIKeyScopesLimitReached is returned when an API key has too many scopes
type ErrAPIKeyScopesLimitReached struct {
	Limit int
}

func (e ErrAPIKeyScopesLimitReached) Error() string {
	return fmt.Sprintf("API key scopes limit of %d reached", e.Limit)
}

func (e ErrAPIKeyScopesLimitReached) StatusCode() int {
	return http.StatusBadRequest
}

// ErrAPIKeyProjectsLimitReached is returned when an API key has too many projects
type ErrAPIKeyProjectsLimitReached struct {
	Limit int
}

func (e ErrAPIKeyProjectsLimitReached) Error() string {
	return fmt.Sprintf("API key projects limit of %d reached", e.Limit)
}

func (e ErrAPIKeyProjectsLimitReached) StatusCode() int {
	return http.StatusBadRequest
}

// ErrAPIKeyBranchesLimitReached is returned when an API key has too many branches
type ErrAPIKeyBranchesLimitReached struct {
	Limit int
}

func (e ErrAPIKeyBranchesLimitReached) Error() string {
	return fmt.Sprintf("API key branches limit of %d reached", e.Limit)
}

func (e ErrAPIKeyBranchesLimitReached) StatusCode() int {
	return http.StatusBadRequest
}

// --- Vercel Marketplace resources ---

type ErrVercelResourceNotFound struct {
	ResourceID string
}

func (e ErrVercelResourceNotFound) Error() string {
	return fmt.Sprintf("vercel resource [%s] not found", e.ResourceID)
}

func (e ErrVercelResourceNotFound) StatusCode() int { return http.StatusNotFound }

type ErrVercelResourceAlreadyExists struct {
	ResourceID string
}

func (e ErrVercelResourceAlreadyExists) Error() string {
	return fmt.Sprintf("vercel resource [%s] already exists", e.ResourceID)
}

func (e ErrVercelResourceAlreadyExists) StatusCode() int { return http.StatusConflict }

// ErrVercelResourceBusy is returned when another provision holds the advisory
// lock for a resource id (a concurrent same-key request is in flight). The caller
// should retry, so it maps to 429.
type ErrVercelResourceBusy struct {
	ResourceID string
}

func (e ErrVercelResourceBusy) Error() string {
	return fmt.Sprintf("vercel resource [%s] is busy; retry later", e.ResourceID)
}

func (e ErrVercelResourceBusy) StatusCode() int { return http.StatusTooManyRequests }

// ErrVercelResourceProjectLinked is returned when a project already backs a
// non-deleted resource (the partial unique index on xata_project_id).
type ErrVercelResourceProjectLinked struct {
	XataProjectID string
}

func (e ErrVercelResourceProjectLinked) Error() string {
	return fmt.Sprintf("xata project [%s] already has an active vercel resource", e.XataProjectID)
}

func (e ErrVercelResourceProjectLinked) StatusCode() int { return http.StatusConflict }

// ErrVercelResourceNotActive is returned when an operation requires an active
// resource but its deletion is already in progress (deleted resources are
// reported as ErrVercelResourceNotFound instead).
type ErrVercelResourceNotActive struct {
	ResourceID string
}

func (e ErrVercelResourceNotActive) Error() string {
	return fmt.Sprintf("vercel resource [%s] is not active (deleting)", e.ResourceID)
}

func (e ErrVercelResourceNotActive) StatusCode() int { return http.StatusConflict }

type ErrVercelResourceBranchNotFound struct {
	ResourceID string
	Scope      string
}

func (e ErrVercelResourceBranchNotFound) Error() string {
	return fmt.Sprintf("vercel resource branch [%s/%s] not found", e.ResourceID, e.Scope)
}

func (e ErrVercelResourceBranchNotFound) StatusCode() int { return http.StatusNotFound }

// ErrVercelResourceBranchExists is returned when a scope already backs a
// non-deleted branch on the resource.
type ErrVercelResourceBranchExists struct {
	ResourceID string
	Scope      string
}

func (e ErrVercelResourceBranchExists) Error() string {
	return fmt.Sprintf("vercel resource [%s] already has an active branch for scope [%s]", e.ResourceID, e.Scope)
}

func (e ErrVercelResourceBranchExists) StatusCode() int { return http.StatusConflict }

// ErrVercelResourceBranchNotActive is returned when an operation requires an
// active resource branch but its deletion is already in progress (deleted
// branches are reported as ErrVercelResourceBranchNotFound instead).
type ErrVercelResourceBranchNotActive struct {
	ResourceID string
	Scope      string
}

func (e ErrVercelResourceBranchNotActive) Error() string {
	return fmt.Sprintf("vercel resource branch [%s/%s] is not active (deleting)", e.ResourceID, e.Scope)
}

func (e ErrVercelResourceBranchNotActive) StatusCode() int { return http.StatusConflict }

// ErrVercelResourceXataBranchLinked is returned when a Xata branch already backs
// another non-deleted scope; a Xata branch backs at most one scope.
type ErrVercelResourceXataBranchLinked struct {
	ResourceID   string
	XataBranchID string
}

func (e ErrVercelResourceXataBranchLinked) Error() string {
	return fmt.Sprintf("vercel resource [%s]: xata branch [%s] already backs another scope", e.ResourceID, e.XataBranchID)
}

func (e ErrVercelResourceXataBranchLinked) StatusCode() int { return http.StatusConflict }
