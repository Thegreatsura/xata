package keycloak

import "context"

//go:generate go run github.com/vektra/mockery/v3 --with-expecter --name KeyCloak

// KeyCloak is the interface for interacting with the KeyCloak service.
// You can keep Keycloak decoupled from the OpenAPI spec by defining models in models/*.go
type KeyCloak interface {
	// CreateOrganization creates a new organization in the given realm.
	CreateOrganization(c context.Context, realm string, params OrganizationCreate) (Organization, error)
	// GetOrganization returns the organization by name in the given realm.
	GetOrganization(c context.Context, realm, name string) (Organization, error)
	// ListOrganizations returns a list of organizations the user is a member of in the given realm.
	ListOrganizations(c context.Context, realm, userID string) ([]Organization, error)
	// AddMember adds a user to the organization in the given realm.
	AddMember(c context.Context, realm string, organizationID string, userID string) error
	// RemoveMember removes a user from the organization in the given realm.
	RemoveMember(c context.Context, realm string, organizationID string, userID string) error
	// ListMembers lists all members of the organization in the given realm.
	ListMembers(c context.Context, realm string, organizationID string) ([]OrganizationMember, error)
	// CreateInvitation sends an invitation for a user to join the organization.
	CreateInvitation(c context.Context, realm string, organizationID string, email string) error
	// ListInvitations retrieves all invitations for an organization with optional filtering.
	ListInvitations(c context.Context, realm string, organizationID string, params ListInvitationsParams) ([]OrganizationInvitation, error)
	// GetInvitation retrieves a specific invitation by ID.
	GetInvitation(c context.Context, realm string, organizationID string, invitationID string) (OrganizationInvitation, error)
	// ResendInvitation resends a pending invitation with a fresh expiration.
	ResendInvitation(c context.Context, realm string, organizationID string, invitationID string) error
	// DeleteInvitation permanently deletes an invitation record.
	DeleteInvitation(c context.Context, realm string, organizationID string, invitationID string) error
	// UpdateOrganization updates an organization's attributes in the given realm.
	UpdateOrganization(c context.Context, realm, organizationID string, update OrganizationUpdate) (Organization, error)
	// DeleteOrganization marks an organization as deleted by setting the deletedAt attribute.
	DeleteOrganization(ctx context.Context, realm, organizationID string) error
	// GetUserRepresentation returns the user representation for the given user ID in the given realm.
	GetUserRepresentation(c context.Context, realm string, userID string) (User, error)
	// ListDisabledOrganizations returns organizations where disabledByAdmin=true OR billingStatus!=ok.
	// When returnCleanedUpOrgs is false, orgs with a resourcesCleanedAt attribute are excluded.
	ListDisabledOrganizations(ctx context.Context, realm string, returnCleanedUpOrgs bool) ([]Organization, error)
	// UpdateUserAttributes merges the given attributes into the user's existing attributes.
	UpdateUserAttributes(ctx context.Context, realm, userID string, update UserAttributesUpdate) error
	// GetIdentityProviderToken retrieves the external identity provider token (e.g. the
	// GitHub user access token) stored in Keycloak for the user identified by userToken.
	// The userToken must be a valid Keycloak access token with the broker read-token role.
	GetIdentityProviderToken(ctx context.Context, realm, provider, userToken string) (string, error)
}
