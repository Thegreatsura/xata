package keycloak

import "time"

const (
	OrganizationDisabledByAdminKey    = "disabledByAdmin"
	OrganizationBillingStatusKey      = "billingStatus"
	OrganizationAdminReasonKey        = "adminReason"
	OrganizationBillingReasonKey      = "billingReason"
	OrganizationLastUpdatedKey        = "lastUpdated"
	OrganizationCreatedAtKey          = "createdAt"
	OrganizationResourcesCleanedAtKey = "resourcesCleanedAt"

	OrganizationDeletedAtKey = "deletedAt"

	OrganizationUsageTierKey = "usageTier"

	OrganizationMarketplaceKey   = "marketplace"
	OrganizationAWSCustomerIDKey = "awsCustomerId"
	OrganizationAWSProductIDKey  = "awsProductId"
	OrganizationAWSAccountIDKey  = "awsAccountId"
)

const (
	OrganizationBillingStatusOK                OrganizationBillingStatus = "ok"
	OrganizationBillingStatusNoPaymentMethod   OrganizationBillingStatus = "no_payment_method"
	OrganizationBillingStatusInvoiceOverdue    OrganizationBillingStatus = "invoice_overdue"
	OrganizationBillingStatusDeletionRequested OrganizationBillingStatus = "deletion_requested"
	OrganizationBillingStatusUnknown           OrganizationBillingStatus = "unknown"
)

const (
	OrganizationUsageTierT1 OrganizationUsageTier = "t1"
	OrganizationUsageTierT2 OrganizationUsageTier = "t2"
)

const OrganizationMarketplaceProviderAWS OrganizationMarketplaceProvider = "aws"

const (
	OrganizationStateEnabled  OrganizationState = "enabled"
	OrganizationStateDisabled OrganizationState = "disabled"
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

type OrganizationMember struct {
	ID    string
	Email string
	Name  string
}

type OrganizationCreate struct {
	Name        string
	Marketplace MarketplaceAttributes
	UsageTier   OrganizationUsageTier
}

type OrganizationUpdate struct {
	Name               *string                    `json:"name"`
	BillingStatus      *OrganizationBillingStatus `json:"billingStatus,omitempty"`
	BillingReason      *string                    `json:"billingReason,omitempty"`
	AdminReason        *string                    `json:"adminReason,omitempty"`
	DisabledByAdmin    *bool                      `json:"disabledByAdmin,omitempty"`
	ResourcesCleanedAt *string                    `json:"resourcesCleanedAt,omitempty"`
	UsageTier          *OrganizationUsageTier     `json:"usageTier,omitempty"`
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

type OrganizationBillingStatus string

type OrganizationUsageTier string

type OrganizationMarketplaceProvider string

type OrganizationState string

type Organization struct {
	ID          string
	Name        string
	Marketplace *OrganizationMarketplaceProvider
	Status      OrganizationStatus
}

type OrganizationStatus struct {
	DisabledByAdmin bool
	BillingStatus   OrganizationBillingStatus
	AdminReason     *string
	BillingReason   *string
	LastUpdated     time.Time
	CreatedAt       *time.Time
	UsageTier       OrganizationUsageTier
}

func (s OrganizationStatus) EffectiveState() OrganizationState {
	if !s.DisabledByAdmin && s.BillingStatus == OrganizationBillingStatusOK {
		return OrganizationStateEnabled
	}
	return OrganizationStateDisabled
}
