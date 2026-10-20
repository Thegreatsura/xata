package store

import "time"

// VercelInstallationStatus is the lifecycle state of a Vercel installation.
type VercelInstallationStatus string

const (
	// VercelInstallationActive is a live installation.
	VercelInstallationActive VercelInstallationStatus = "active"
	// VercelInstallationDeleting is an installation being torn down; kept until
	// Vercel finalizes any outstanding billing.
	VercelInstallationDeleting VercelInstallationStatus = "deleting"
	// VercelInstallationDeleted is a soft-deleted installation.
	VercelInstallationDeleted VercelInstallationStatus = "deleted"
)

// VercelInstallation records a Vercel Marketplace installation and its link to
// a Xata organization. AccessToken is stored encrypted; the store persists and
// returns it as-is and never encrypts or decrypts.
type VercelInstallation struct {
	InstallationID     string
	VercelAccountID    string
	XataOrganizationID string
	// AccessToken is the installation access token, already encrypted by the
	// caller (see saas-services/auth/vercel.TokenCipher). The store never sees
	// the plaintext.
	AccessToken string
	Scopes      []string
	// AcceptedPolicies maps a Vercel policy id to its acceptance timestamp.
	AcceptedPolicies map[string]time.Time
	Status           VercelInstallationStatus
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        *time.Time
}

// VercelResourceStatus is the lifecycle state of a Vercel resource or resource
// branch. deleting is a transient state kept visible while teardown finalizes;
// deleted is the terminal soft-delete (the row is retained but no longer usable).
type VercelResourceStatus string

const (
	VercelResourceActive   VercelResourceStatus = "active"
	VercelResourceDeleting VercelResourceStatus = "deleting"
	VercelResourceDeleted  VercelResourceStatus = "deleted"
)

// Fixed VercelResourceBranch scopes mirroring Vercel's environments. Scope is
// otherwise free text: per-PR ephemeral branches use a dynamic deployment/PR ref
// (namespaced, e.g. "preview:<deploymentId>") that will not collide with these.
const (
	VercelScopeProduction  = "production"
	VercelScopePreview     = "preview"
	VercelScopeDevelopment = "development"
)

// VercelResource records a Vercel Marketplace resource and its link to a Xata
// project. Its branches — including the production/main branch — live in
// VercelResourceBranch, keyed by scope. XataProjectID is a logical reference to
// a project in the projects service's database (a separate store), not a foreign
// key; teardown is explicit via the projects gRPC.
type VercelResource struct {
	ResourceID     string               `json:"resourceId"`
	InstallationID string               `json:"installationId"`
	ProductSlug    string               `json:"productSlug"`
	BillingPlanID  string               `json:"billingPlanId"`
	XataProjectID  string               `json:"xataProjectId"`
	Name           string               `json:"name"`
	Metadata       map[string]any       `json:"metadata"`
	Status         VercelResourceStatus `json:"status"`
	CreatedAt      time.Time            `json:"createdAt"`
	UpdatedAt      time.Time            `json:"updatedAt"`
	DeletedAt      *time.Time           `json:"deletedAt,omitempty"`
}

// VercelResourceBranch is one branch of a Vercel resource, keyed by Scope. Every
// branch is a row here, including the production/main branch (Scope
// "production"). XataBranchID is a logical reference to a branch in the projects
// service's database, not a foreign key.
type VercelResourceBranch struct {
	ID           string               `json:"id"`
	ResourceID   string               `json:"resourceId"`
	Scope        string               `json:"scope"`
	XataBranchID string               `json:"xataBranchId"`
	Status       VercelResourceStatus `json:"status"`
	CreatedAt    time.Time            `json:"createdAt"`
	UpdatedAt    time.Time            `json:"updatedAt"`
	DeletedAt    *time.Time           `json:"deletedAt,omitempty"`
}
