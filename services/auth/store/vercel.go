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
