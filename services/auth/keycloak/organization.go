package keycloak

import "xata/services/auth/api/spec"

const (
	OrganizationDisabledByAdminKey             = "disabledByAdmin"
	OrganizationBillingStatusKey               = "billingStatus"
	OrganizationAdminReasonKey                 = "adminReason"
	OrganizationBillingReasonKey               = "billingReason"
	OrganizationLastUpdatedKey                 = "lastUpdated"
	OrganizationCreatedAtKey                   = "createdAt"
	OrganizationResourcesCleanedAtKey          = "resourcesCleanedAt"
	OrganizationBillingStatusNoPaymentMethod   = "no_payment_method"
	OrganizationBillingStatusDeletionRequested = "deletion_requested"

	OrganizationDeletedAtKey = "deletedAt"

	OrganizationUsageTierKey = "usageTier"

	OrganizationMarketplaceKey   = "marketplace"
	OrganizationAWSCustomerIDKey = "awsCustomerId"
	OrganizationAWSProductIDKey  = "awsProductId"
	OrganizationAWSAccountIDKey  = "awsAccountId"
)

type Domain struct {
	Name string `json:"name"`
}

// MaxOrganizationMembers is the maximum number of users allowed in an organization
const MaxOrganizationMembers = 100

type KeycloakOrganization struct {
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	Alias       string              `json:"alias"`
	Description string              `json:"description"`
	Domains     []Domain            `json:"domains"`
	Attributes  map[string][]string `json:"attributes,omitempty"`
	RedirectURL string              `json:"redirectUrl,omitempty"`
}

type OrganizationCreate struct {
	Name        string
	Marketplace MarketplaceAttributes
	UsageTier   spec.OrganizationStatusUsageTier
}

type OrganizationUpdate struct {
	Name               *string `json:"name"`
	BillingStatus      *string `json:"billingStatus,omitempty"`
	BillingReason      *string `json:"billingReason,omitempty"`
	AdminReason        *string `json:"adminReason,omitempty"`
	DisabledByAdmin    *bool   `json:"disabledByAdmin,omitempty"`
	ResourcesCleanedAt *string `json:"resourcesCleanedAt,omitempty"`
	UsageTier          *string `json:"usageTier,omitempty"`
}

type OrganizationInvitation struct {
	ID             string  `json:"id"`
	OrganizationID string  `json:"organizationId"`
	Email          string  `json:"email"`
	FirstName      *string `json:"firstName,omitempty"`
	LastName       *string `json:"lastName,omitempty"`
	CreatedAt      int64   `json:"sentDate"`
	ExpiresAt      int64   `json:"expiresAt"`
	Status         string  `json:"status"`
	InviteLink     string  `json:"inviteLink"`
}

type ListInvitationsParams struct {
	Status    *string
	Email     *string
	FirstName *string
	LastName  *string
	Search    *string
	First     *int
	Max       *int
}
