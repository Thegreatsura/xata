package keycloak

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"xata/internal/idgen"
	"xata/services/auth/config"

	"github.com/Nerzal/gocloak/v13"
	"github.com/go-resty/resty/v2"
)

// tokenSafetyMargin is subtracted from the admin token's advertised lifetime so
// we never hand out a token that expires mid-request.
const tokenSafetyMargin = 10 * time.Second

// Implements kc.go
type restKC struct {
	client     *gocloak.GoCloak
	authConfig config.AuthConfig

	// tokenMu guards the cached admin token. restKC is a shared singleton hit
	// concurrently by every auth request, so all cache access is serialized.
	tokenMu     sync.Mutex
	cachedToken *gocloak.JWT
	tokenExpiry time.Time // wall-clock time the cached token stops being usable
}

func NewRestKC(client *gocloak.GoCloak, authConfig config.AuthConfig) KeyCloak {
	return &restKC{
		client:     client,
		authConfig: authConfig,
	}
}

func (r *restKC) CreateOrganization(ctx context.Context, realm string, params OrganizationCreate) (Organization, error) {
	if params.UsageTier == "" {
		return Organization{}, fmt.Errorf("usage tier is required")
	}
	if params.BillingCollectionMethod == "" {
		return Organization{}, fmt.Errorf("billing collection method is required")
	}
	if !params.BillingCollectionMethod.Valid() {
		return Organization{}, fmt.Errorf("unsupported billing collection method %q", params.BillingCollectionMethod)
	}
	if params.BillingCollectionMethod == OrganizationBillingCollectionMethodMarketplace && params.Marketplace == nil {
		return Organization{}, fmt.Errorf("marketplace billing collection method requires a marketplace organization")
	}
	if params.Marketplace != nil {
		if err := params.Marketplace.Validate(); err != nil {
			return Organization{}, fmt.Errorf("validate marketplace: %w", err)
		}
	}

	orgsURL, err := r.buildRealmURL(realm, "organizations")
	if err != nil {
		return Organization{}, err
	}

	id := idgen.GenerateOrganizationID()
	createOrg := r.buildCreateOrganizationPayload(id, params)

	resp, err := r.makeAuthenticatedRequest(ctx, "POST", orgsURL, nil, createOrg)
	if err != nil {
		return Organization{}, fmt.Errorf("failed to create organization: %w", err)
	}

	if !r.isSuccessStatus(resp.StatusCode(), http.StatusCreated, http.StatusOK) {
		return Organization{}, fmt.Errorf("failed to create organization: %s, status %d", params.Name, resp.StatusCode())
	}

	return r.convertToOrganization(createOrg), nil
}

func (r *restKC) buildCreateOrganizationPayload(id string, params OrganizationCreate) KeycloakOrganization {
	now := time.Now().UTC().Format(time.RFC3339)
	attrs := map[string][]string{
		OrganizationDisplayNameKey:             {params.Name},
		OrganizationDisabledByAdminKey:         {"false"},
		OrganizationBillingStatusKey:           {string(params.BillingStatus)},
		OrganizationBillingReasonKey:           {params.BillingReason},
		OrganizationUsageTierKey:               {string(params.UsageTier)},
		OrganizationLastUpdatedKey:             {now},
		OrganizationCreatedAtKey:               {now},
		OrganizationBillingCollectionMethodKey: {string(params.BillingCollectionMethod)},
	}
	if params.Marketplace != nil {
		maps.Copy(attrs, params.Marketplace.BuildKeycloakAttributes())
	}
	return KeycloakOrganization{
		Name:        id,
		Alias:       id,
		Attributes:  attrs,
		RedirectURL: fmt.Sprintf(r.authConfig.FrontendURL+"/organizations/%s", id),
	}
}

func (r *restKC) GetOrganization(ctx context.Context, realm, alias string, options GetOrganizationOptions) (Organization, error) {
	keycloakOrganization, err := r.searchOrganization(ctx, realm, alias)
	if err != nil {
		return Organization{}, fmt.Errorf("failed to get organization: %w", err)
	}
	deletedAt, deleted := FirstAttr(keycloakOrganization.Attributes, OrganizationDeletedAtKey)
	if deleted && !options.IncludeDeleted {
		return Organization{}, ErrOrganizationNotFound{ID: alias}
	}
	organization := r.convertToOrganization(keycloakOrganization)
	if deleted && organization.Status.DeletedAt == nil {
		_, err := time.Parse(time.RFC3339, deletedAt)
		return Organization{}, fmt.Errorf("parse deletedAt for organization %s: %w", alias, err)
	}
	return organization, nil
}

func (r *restKC) ListOrganizations(ctx context.Context, realm, userID string) ([]Organization, error) {
	if userID == "" {
		return nil, fmt.Errorf("userID is empty")
	}

	orgsURL, err := r.buildRealmURL(realm, "organizations/members", userID, "organizations")
	if err != nil {
		return nil, err
	}

	queryParams := map[string]string{
		"briefRepresentation": "false",
	}

	resp, err := r.makeAuthenticatedRequest(ctx, "GET", orgsURL, queryParams, nil)
	if err != nil {
		return nil, fmt.Errorf("keycloak request: %w", err)
	}

	if !r.isSuccessStatus(resp.StatusCode(), http.StatusOK, http.StatusConflict) {
		return nil, fmt.Errorf("unexpected status code: %s, status code: %d", resp.String(), resp.StatusCode())
	}

	var keycloakOrganizations []KeycloakOrganization
	err = json.Unmarshal(resp.Body(), &keycloakOrganizations)
	if err != nil {
		return nil, fmt.Errorf("unmarshal organization list: %w", err)
	}

	orgs := make([]Organization, 0, len(keycloakOrganizations))
	for _, org := range keycloakOrganizations {
		if _, ok := FirstAttr(org.Attributes, OrganizationDeletedAtKey); ok {
			continue
		}
		orgs = append(orgs, r.convertToOrganization(org))
	}

	return orgs, nil
}

func (r *restKC) AddMember(ctx context.Context, realm string, organizationID string, userID string) error {
	organization, err := r.searchOrganization(ctx, realm, organizationID)
	if err != nil {
		return fmt.Errorf("failed to get organization: %w", err)
	}

	members, err := r.ListMembers(ctx, realm, organizationID)
	if err != nil {
		return fmt.Errorf("failed to list members: %w", err)
	}
	if len(members) >= MaxOrganizationMembers {
		return ErrOrganizationMemberLimitReached{OrganizationID: organizationID}
	}

	addMemberURL, err := r.buildRealmURL(realm, "organizations", organization.ID, "members")
	if err != nil {
		return fmt.Errorf("failed to join URL: %w", err)
	}

	resp, err := r.makeAuthenticatedRequest(ctx, "POST", addMemberURL, nil, userID)
	if err != nil {
		return ErrUnableToAddMember{userID, organizationID}
	}

	if !r.isSuccessStatus(resp.StatusCode(), http.StatusOK, http.StatusCreated) {
		return ErrUnableToAddMember{userID, organizationID}
	}
	return nil
}

func (r *restKC) RemoveMember(ctx context.Context, realm string, organizationID string, userID string) error {
	organization, err := r.searchOrganization(ctx, realm, organizationID)
	if err != nil {
		return fmt.Errorf("failed to get organization: %w", err)
	}

	delURL, err := r.buildRealmURL(realm, "organizations", organization.ID, "members", userID)
	if err != nil {
		return fmt.Errorf("failed to join URL: %w", err)
	}

	resp, err := r.makeAuthenticatedRequest(ctx, "DELETE", delURL, nil, nil)
	if err != nil {
		return err
	}
	// DELETE is idempotent: a 404 from Keycloak means the member is already
	// absent from the organization, which matches the desired end state.
	if resp.StatusCode() == http.StatusNotFound {
		return nil
	}
	if !r.isSuccessStatus(resp.StatusCode(), http.StatusOK, http.StatusNoContent) {
		return fmt.Errorf("failed to remove member: status code: %d", resp.StatusCode())
	}
	return nil
}

func (r *restKC) ListMembers(ctx context.Context, realm string, organizationID string) ([]OrganizationMember, error) {
	organization, err := r.searchOrganization(ctx, realm, organizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get organization: %w", err)
	}

	listURL, err := r.buildRealmURL(realm, "organizations", organization.ID, "members")
	if err != nil {
		return nil, fmt.Errorf("failed to join URL: %w", err)
	}

	queryParams := map[string]string{
		"max": fmt.Sprintf("%d", MaxOrganizationMembers),
	}

	resp, err := r.makeAuthenticatedRequest(ctx, "GET", listURL, queryParams, nil)
	if err != nil {
		return nil, err
	}
	if !r.isSuccessStatus(resp.StatusCode(), http.StatusOK) {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode())
	}

	var users []User
	if err := json.Unmarshal(resp.Body(), &users); err != nil {
		return nil, fmt.Errorf("failed to unmarshal members: %w", err)
	}

	res := make([]OrganizationMember, len(users))
	for i, u := range users {
		res[i] = OrganizationMember{
			Email: u.Email,
			Name:  fmt.Sprintf("%s %s", u.FirstName, u.LastName),
			ID:    u.ID,
		}
	}
	return res, nil
}

func (r *restKC) CreateInvitation(ctx context.Context, realm string, organizationID string, email string) error {
	organization, err := r.searchOrganization(ctx, realm, organizationID)
	if err != nil {
		return fmt.Errorf("failed to get organization: %w", err)
	}

	invURL, err := r.buildRealmURL(realm, "organizations", organization.ID, "members", "invite-user")
	if err != nil {
		return fmt.Errorf("failed to join URL: %w", err)
	}

	formData := url.Values{}
	formData.Set("email", email)

	resp, err := r.makeAuthenticatedRequest(ctx, "POST", invURL, nil, formData)
	if err != nil {
		return err
	}

	if resp.StatusCode() == http.StatusConflict {
		return ErrInvitationConflict{Email: email}
	}

	if !r.isSuccessStatus(resp.StatusCode(), http.StatusCreated, http.StatusNoContent, http.StatusOK) {
		return ErrInvitationFailed{Email: email}
	}

	return nil
}

func (r *restKC) ListInvitations(ctx context.Context, realm string, organizationID string, params ListInvitationsParams) ([]OrganizationInvitation, error) {
	organization, err := r.searchOrganization(ctx, realm, organizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get organization: %w", err)
	}

	listURL, err := r.buildRealmURL(realm, "organizations", organization.ID, "invitations")
	if err != nil {
		return nil, fmt.Errorf("failed to join URL: %w", err)
	}

	queryParams := make(map[string]string)
	if params.Status != nil {
		queryParams["status"] = *params.Status
	}
	if params.Email != nil {
		queryParams["email"] = *params.Email
	}
	if params.FirstName != nil {
		queryParams["firstName"] = *params.FirstName
	}
	if params.LastName != nil {
		queryParams["lastName"] = *params.LastName
	}
	if params.Search != nil {
		queryParams["search"] = *params.Search
	}
	if params.First != nil {
		queryParams["first"] = fmt.Sprintf("%d", *params.First)
	}
	if params.Max != nil {
		queryParams["max"] = fmt.Sprintf("%d", *params.Max)
	}

	resp, err := r.makeAuthenticatedRequest(ctx, "GET", listURL, queryParams, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list invitations: %w", err)
	}

	if !r.isSuccessStatus(resp.StatusCode(), http.StatusOK) {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode())
	}

	var invitations []OrganizationInvitation
	if err := json.Unmarshal(resp.Body(), &invitations); err != nil {
		return nil, fmt.Errorf("failed to unmarshal invitations: %w", err)
	}

	return invitations, nil
}

func (r *restKC) GetInvitation(ctx context.Context, realm string, organizationID string, invitationID string) (OrganizationInvitation, error) {
	organization, err := r.searchOrganization(ctx, realm, organizationID)
	if err != nil {
		return OrganizationInvitation{}, fmt.Errorf("failed to get organization: %w", err)
	}

	getURL, err := r.buildRealmURL(realm, "organizations", organization.ID, "invitations", invitationID)
	if err != nil {
		return OrganizationInvitation{}, fmt.Errorf("failed to join URL: %w", err)
	}

	resp, err := r.makeAuthenticatedRequest(ctx, "GET", getURL, nil, nil)
	if err != nil {
		return OrganizationInvitation{}, fmt.Errorf("failed to get invitation: %w", err)
	}

	if resp.StatusCode() == http.StatusNotFound {
		return OrganizationInvitation{}, ErrInvitationNotFound{ID: invitationID}
	}
	if !r.isSuccessStatus(resp.StatusCode(), http.StatusOK) {
		return OrganizationInvitation{}, fmt.Errorf("unexpected status code: %d", resp.StatusCode())
	}

	var invitation OrganizationInvitation
	if err := json.Unmarshal(resp.Body(), &invitation); err != nil {
		return OrganizationInvitation{}, fmt.Errorf("failed to unmarshal invitation: %w", err)
	}

	return invitation, nil
}

func (r *restKC) ResendInvitation(ctx context.Context, realm string, organizationID string, invitationID string) error {
	organization, err := r.searchOrganization(ctx, realm, organizationID)
	if err != nil {
		return fmt.Errorf("failed to get organization: %w", err)
	}

	resendURL, err := r.buildRealmURL(realm, "organizations", organization.ID, "invitations", invitationID, "resend")
	if err != nil {
		return fmt.Errorf("failed to join URL: %w", err)
	}

	resp, err := r.makeAuthenticatedRequest(ctx, "POST", resendURL, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to resend invitation: %w", err)
	}

	if resp.StatusCode() == http.StatusNotFound {
		return ErrInvitationNotFound{ID: invitationID}
	}
	if !r.isSuccessStatus(resp.StatusCode(), http.StatusOK, http.StatusNoContent) {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode())
	}

	return nil
}

func (r *restKC) DeleteInvitation(ctx context.Context, realm string, organizationID string, invitationID string) error {
	organization, err := r.searchOrganization(ctx, realm, organizationID)
	if err != nil {
		return fmt.Errorf("failed to get organization: %w", err)
	}

	deleteURL, err := r.buildRealmURL(realm, "organizations", organization.ID, "invitations", invitationID)
	if err != nil {
		return fmt.Errorf("failed to join URL: %w", err)
	}

	resp, err := r.makeAuthenticatedRequest(ctx, "DELETE", deleteURL, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to delete invitation: %w", err)
	}

	// Deleting is idempotent: a 404 means the invitation is already gone, which
	// is the end state the caller asked for.
	if resp.StatusCode() == http.StatusNotFound {
		return nil
	}
	if !r.isSuccessStatus(resp.StatusCode(), http.StatusOK, http.StatusNoContent) {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode())
	}

	return nil
}

func (r *restKC) UpdateOrganization(
	ctx context.Context,
	realm, organizationID string,
	update OrganizationUpdate,
) (Organization, error) {
	if update.BillingCollectionMethod != nil && !update.BillingCollectionMethod.Valid() {
		return Organization{}, fmt.Errorf("unsupported billing collection method %q", *update.BillingCollectionMethod)
	}

	organization, err := r.searchOrganization(ctx, realm, organizationID)
	if err != nil {
		return Organization{}, fmt.Errorf("failed to get organization: %w", err)
	}

	if update.BillingCollectionMethod != nil && *update.BillingCollectionMethod == OrganizationBillingCollectionMethodMarketplace {
		marketplace, _ := FirstAttr(organization.Attributes, OrganizationMarketplaceKey)
		if marketplace == "" {
			return Organization{}, fmt.Errorf("marketplace billing collection method requires a marketplace organization")
		}
	}

	// Collect all attribute updates first
	updates := make(map[string][]string)

	if update.Name != nil {
		updates[OrganizationDisplayNameKey] = []string{*update.Name}
	}
	if update.DisabledByAdmin != nil {
		updates[OrganizationDisabledByAdminKey] = []string{fmt.Sprintf("%t", *update.DisabledByAdmin)}
	}
	if update.BillingStatus != nil {
		updates[OrganizationBillingStatusKey] = []string{string(*update.BillingStatus)}
	}
	if update.AdminReason != nil {
		updates[OrganizationAdminReasonKey] = []string{*update.AdminReason}
	}
	if update.BillingReason != nil {
		updates[OrganizationBillingReasonKey] = []string{*update.BillingReason}
	}
	if update.UsageTier != nil {
		updates[OrganizationUsageTierKey] = []string{string(*update.UsageTier)}
	}
	if update.BillingCollectionMethod != nil {
		updates[OrganizationBillingCollectionMethodKey] = []string{string(*update.BillingCollectionMethod)}
	}
	var deleteResourcesCleanedAt bool
	if update.ResourcesCleanedAt != nil {
		if *update.ResourcesCleanedAt == "" {
			deleteResourcesCleanedAt = true
		} else {
			updates[OrganizationResourcesCleanedAtKey] = []string{*update.ResourcesCleanedAt}
		}
	}

	if len(updates) == 0 && !deleteResourcesCleanedAt {
		return r.GetOrganization(ctx, realm, organizationID, GetOrganizationOptions{IncludeDeleted: false})
	}

	// Apply updates
	if organization.Attributes == nil {
		organization.Attributes = make(map[string][]string)
	}

	maps.Copy(organization.Attributes, updates)

	if deleteResourcesCleanedAt {
		delete(organization.Attributes, OrganizationResourcesCleanedAtKey)
	}
	organization.Attributes[OrganizationLastUpdatedKey] = []string{
		time.Now().UTC().Format(time.RFC3339),
	}

	updateOrgURL, err := r.buildRealmURL(realm, "organizations", organization.ID)
	if err != nil {
		return Organization{}, fmt.Errorf("failed to join URL: %w", err)
	}

	resp, err := r.makeAuthenticatedRequest(ctx, http.MethodPut, updateOrgURL, nil, organization)
	if err != nil {
		return Organization{}, fmt.Errorf("failed to update organization: %w", err)
	}

	if !r.isSuccessStatus(resp.StatusCode(), http.StatusOK, http.StatusNoContent) {
		return Organization{}, fmt.Errorf(
			"failed to update organization: %s, status code: %d, error: %s",
			organizationID, resp.StatusCode(), resp.String(),
		)
	}

	return r.GetOrganization(ctx, realm, organizationID, GetOrganizationOptions{IncludeDeleted: false})
}

func (r *restKC) DeleteOrganization(ctx context.Context, realm, organizationID string) error {
	organization, err := r.searchOrganization(ctx, realm, organizationID)
	if err != nil {
		return fmt.Errorf("get organization: %w", err)
	}

	if organization.Attributes == nil {
		organization.Attributes = make(map[string][]string)
	}

	if _, ok := FirstAttr(organization.Attributes, OrganizationDeletedAtKey); ok {
		return nil
	}

	now := time.Now().UTC().Format(time.RFC3339)
	organization.Attributes[OrganizationDeletedAtKey] = []string{now}
	organization.Attributes[OrganizationLastUpdatedKey] = []string{now}

	orgURL, err := r.buildRealmURL(realm, "organizations", organization.ID)
	if err != nil {
		return fmt.Errorf("build organization URL: %w", err)
	}

	resp, err := r.makeAuthenticatedRequest(ctx, http.MethodPut, orgURL, nil, organization)
	if err != nil {
		return fmt.Errorf("set deletedAt: %w", err)
	}
	if !r.isSuccessStatus(resp.StatusCode(), http.StatusOK, http.StatusNoContent) {
		return fmt.Errorf("set deletedAt: unexpected status %d: %s", resp.StatusCode(), resp.String())
	}
	return nil
}

func (r *restKC) ListDisabledOrganizations(ctx context.Context, realm string, returnCleanedUpOrgs bool) ([]Organization, error) {
	orgsURL, err := r.buildRealmURL(realm, "organizations")
	if err != nil {
		return nil, err
	}

	queries := []string{
		"disabledByAdmin:true",
		"billingStatus:no_payment_method",
		"billingStatus:invoice_overdue",
		"billingStatus:unknown",
		"billingStatus:deletion_requested",
	}

	seen := make(map[string]struct{})
	result := make([]Organization, 0)

	const pageSize = 200

	for _, q := range queries {
		fetched := pageSize
		for first := 0; fetched >= pageSize; first += pageSize {
			queryParams := map[string]string{
				"q":                   q,
				"briefRepresentation": "false",
				"first":               fmt.Sprintf("%d", first),
				"max":                 fmt.Sprintf("%d", pageSize),
			}

			resp, err := r.makeAuthenticatedRequest(ctx, "GET", orgsURL, queryParams, nil)
			if err != nil {
				return nil, fmt.Errorf("keycloak request for %s: %w", q, err)
			}

			if !r.isSuccessStatus(resp.StatusCode(), http.StatusOK) {
				return nil, fmt.Errorf("unexpected status code for %s: %d", q, resp.StatusCode())
			}

			var keycloakOrganizations []KeycloakOrganization
			if err := json.Unmarshal(resp.Body(), &keycloakOrganizations); err != nil {
				return nil, fmt.Errorf("unmarshal organization list for %s: %w", q, err)
			}

			for _, org := range keycloakOrganizations {
				if _, exists := seen[org.Alias]; exists {
					continue
				}
				seen[org.Alias] = struct{}{}
				if _, ok := FirstAttr(org.Attributes, OrganizationDeletedAtKey); ok {
					continue
				}
				if !returnCleanedUpOrgs {
					if _, ok := FirstAttr(org.Attributes, OrganizationResourcesCleanedAtKey); ok {
						continue
					}
				}
				result = append(result, r.convertToOrganization(org))
			}

			fetched = len(keycloakOrganizations)
		}
	}

	return result, nil
}

func (r *restKC) GetUserRepresentation(ctx context.Context, realm string, userID string) (User, error) {
	userURL, err := r.buildRealmURL(realm, "users", userID)
	if err != nil {
		return User{}, fmt.Errorf("failed to join URL: %w", err)
	}

	resp, err := r.makeAuthenticatedRequest(ctx, "GET", userURL, nil, nil)
	if err != nil {
		return User{}, fmt.Errorf("failed to get user representation: %w", err)
	}

	if !r.isSuccessStatus(resp.StatusCode(), http.StatusOK) {
		return User{}, fmt.Errorf("unexpected status code: %d", resp.StatusCode())
	}

	var user User
	err = json.Unmarshal(resp.Body(), &user)
	if err != nil {
		return User{}, fmt.Errorf("failed to unmarshal user representation: %w", err)
	}

	parseUserAttributes(&user)
	return user, nil
}

func parseUserAttributes(user *User) {
	if v, ok := FirstAttr(user.Attributes, "marketplace"); ok {
		user.Marketplace = v
	}
	if v, ok := FirstAttr(user.Attributes, "awsCustomerId"); ok {
		user.AWSCustomerID = v
	}
	if v, ok := FirstAttr(user.Attributes, "awsProductId"); ok {
		user.AWSProductID = v
	}
	if v, ok := FirstAttr(user.Attributes, "awsAccountId"); ok {
		user.AWSAccountID = v
	}
}

// UpdateUserAttributes uses a GET-merge-PUT pattern because the Keycloak Admin REST API
// user endpoint is PUT (full replacement), not PATCH. We must GET the full user representation,
// merge in the new attributes, and PUT the entire object back to avoid clobbering existing fields.
// See: https://www.keycloak.org/docs-api/latest/rest-api/index.html#_users
func (r *restKC) UpdateUserAttributes(ctx context.Context, realm, userID string, update UserAttributesUpdate) error {
	updates := make(map[string][]string)
	if update.Marketplace != nil {
		updates["marketplace"] = []string{*update.Marketplace}
	}
	if update.MarketplaceRegisteredAt != nil {
		updates["marketplaceRegisteredAt"] = []string{*update.MarketplaceRegisteredAt}
	}
	if update.AWSAccountID != nil {
		updates["awsAccountId"] = []string{*update.AWSAccountID}
	}
	if update.AWSCustomerID != nil {
		updates["awsCustomerId"] = []string{*update.AWSCustomerID}
	}
	if update.AWSProductID != nil {
		updates["awsProductId"] = []string{*update.AWSProductID}
	}

	if len(updates) == 0 {
		return nil
	}

	userURL, err := r.buildRealmURL(realm, "users", userID)
	if err != nil {
		return fmt.Errorf("build user URL: %w", err)
	}

	resp, err := r.makeAuthenticatedRequest(ctx, "GET", userURL, nil, nil)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}
	if !r.isSuccessStatus(resp.StatusCode(), http.StatusOK) {
		return fmt.Errorf("get user: status %d", resp.StatusCode())
	}

	var raw map[string]any
	if err := json.Unmarshal(resp.Body(), &raw); err != nil {
		return fmt.Errorf("unmarshal user: %w", err)
	}

	existing, _ := raw["attributes"].(map[string]any)
	if existing == nil {
		existing = make(map[string]any)
	}
	for k, v := range updates {
		existing[k] = v
	}
	raw["attributes"] = existing

	resp, err = r.makeAuthenticatedRequest(ctx, "PUT", userURL, nil, raw)
	if err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	if !r.isSuccessStatus(resp.StatusCode(), http.StatusOK, http.StatusNoContent) {
		return fmt.Errorf("update user: status %d", resp.StatusCode())
	}

	return nil
}

// getToken returns an admin token for authenticating Keycloak Admin REST calls.
// Tokens are cached and reused until shortly before they expire, so we pay for a
// password login roughly once per token lifetime instead of on every REST call.
func (r *restKC) getToken(ctx context.Context) (*gocloak.JWT, error) {
	r.tokenMu.Lock()
	defer r.tokenMu.Unlock()

	if r.cachedToken != nil && time.Now().Before(r.tokenExpiry) {
		return r.cachedToken, nil
	}

	// Holding the lock across the login serializes concurrent refreshes so we
	// don't stampede Keycloak with parallel logins when the token expires.
	jwt, err := r.client.LoginAdmin(ctx, "temp-admin", r.authConfig.KeycloakAdminPassword, "master")
	if err != nil {
		return nil, fmt.Errorf("failed to login as admin: %w", err)
	}

	r.cachedToken = jwt
	r.tokenExpiry = time.Now().Add(time.Duration(jwt.ExpiresIn)*time.Second - tokenSafetyMargin)
	return jwt, nil
}

// invalidateToken drops the cached admin token, forcing the next getToken to log
// in again. Used when a request is rejected with 401 despite a non-expired token.
func (r *restKC) invalidateToken() {
	r.tokenMu.Lock()
	defer r.tokenMu.Unlock()
	r.cachedToken = nil
	r.tokenExpiry = time.Time{}
}

func (r *restKC) searchOrganization(ctx context.Context, realm, alias string) (KeycloakOrganization, error) {
	searchURL, err := r.buildRealmURL(realm, "organizations")
	if err != nil {
		return KeycloakOrganization{}, fmt.Errorf("failed to join URL: %w", err)
	}

	queryParams := map[string]string{
		"q":                   fmt.Sprintf("alias:%s", alias),
		"briefRepresentation": "false",
	}

	resp, err := r.makeAuthenticatedRequest(ctx, "GET", searchURL, queryParams, nil)
	if err != nil {
		return KeycloakOrganization{}, fmt.Errorf("failed to get organization: %w", err)
	}

	if !r.isSuccessStatus(resp.StatusCode(), http.StatusOK, http.StatusConflict) {
		return KeycloakOrganization{}, ErrOrganizationNotFound{ID: alias}
	}

	var orgs []KeycloakOrganization
	err = json.Unmarshal(resp.Body(), &orgs)
	if err != nil {
		return KeycloakOrganization{}, fmt.Errorf("failed to unmarshal organization: %w", err)
	}

	if len(orgs) == 0 {
		return KeycloakOrganization{}, ErrOrganizationNotFound{ID: alias}
	}

	return orgs[0], nil
}

func (r *restKC) buildRealmURL(realm string, pathSegments ...string) (string, error) {
	segments := append([]string{"admin/realms", realm}, pathSegments...)
	return url.JoinPath(r.authConfig.KeycloakURL, segments...)
}

// GetIdentityProviderToken retrieves the stored external identity provider token for
// the user identified by userToken via the Keycloak identity broker token endpoint.
// The request is authenticated with the user's own token, not the admin token.
func (r *restKC) GetIdentityProviderToken(ctx context.Context, realm, provider, userToken string) (string, error) {
	brokerURL, err := url.JoinPath(r.authConfig.KeycloakURL, "realms", realm, "broker", provider, "token")
	if err != nil {
		return "", fmt.Errorf("build broker token url: %w", err)
	}

	resp, err := r.client.GetRequestWithBearerAuth(ctx, userToken).Get(brokerURL)
	if err != nil {
		return "", fmt.Errorf("get identity provider token: %w", err)
	}

	switch resp.StatusCode() {
	case http.StatusOK:
		token, err := parseIdentityProviderToken(resp.Body())
		if err != nil {
			return "", fmt.Errorf("parse %s identity provider token: %w", provider, err)
		}
		return token, nil
	case http.StatusBadRequest, http.StatusNotFound:
		// Keycloak returns 400 when the identity is not linked (callers are expected
		// to have validated the user token beforehand) and 404 when the linked
		// identity has no stored token.
		return "", ErrIdentityProviderNotLinked{Provider: provider}
	case http.StatusForbidden:
		return "", ErrIdentityProviderTokenForbidden{Provider: provider}
	default:
		return "", fmt.Errorf("get identity provider token: status %d", resp.StatusCode())
	}
}

// parseIdentityProviderToken extracts the access token from the identity broker
// response. Keycloak stores the provider's token endpoint response verbatim, so the
// body can be JSON or form-encoded depending on the provider configuration.
func parseIdentityProviderToken(body []byte) (string, error) {
	var jsonBody struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &jsonBody); err == nil {
		if jsonBody.AccessToken == "" {
			return "", fmt.Errorf("no access_token in response")
		}
		return jsonBody.AccessToken, nil
	}

	values, err := url.ParseQuery(string(body))
	if err != nil {
		return "", fmt.Errorf("response is neither JSON nor form-encoded")
	}
	token := values.Get("access_token")
	if token == "" {
		return "", fmt.Errorf("no access_token in response")
	}
	return token, nil
}

func (r *restKC) makeAuthenticatedRequest(ctx context.Context, method, urlStr string, queryParams map[string]string, data any) (*resty.Response, error) {
	// doRequest builds and sends the request using the current admin token. It is
	// a closure so we can rebuild the request with a fresh token on retry.
	doRequest := func() (*resty.Response, error) {
		jwt, err := r.getToken(ctx)
		if err != nil {
			return nil, err
		}

		req := r.client.GetRequestWithBearerAuth(ctx, jwt.AccessToken)

		if data != nil {
			switch v := data.(type) {
			case url.Values:
				req = req.SetHeader("Content-Type", "application/x-www-form-urlencoded")
				req = req.SetBody(v.Encode())
				// Handle JSON body for any other type
			default:
				req = req.SetBody(data)
			}
		}

		for key, value := range queryParams {
			req = req.SetQueryParam(key, value)
		}

		switch method {
		case "GET":
			return req.Get(urlStr)
		case "POST":
			return req.Post(urlStr)
		case "PUT":
			return req.Put(urlStr)
		case "DELETE":
			return req.Delete(urlStr)
		default:
			return nil, fmt.Errorf("unsupported HTTP method: %s", method)
		}
	}

	resp, err := doRequest()
	if err != nil {
		return nil, err
	}

	// A cached token may be rejected before its expiry (e.g. Keycloak restart or
	// server-side revocation). Drop it and retry once with a fresh login.
	if resp.StatusCode() == http.StatusUnauthorized {
		r.invalidateToken()
		return doRequest()
	}

	return resp, nil
}

func (r *restKC) isSuccessStatus(actual int, expected ...int) bool {
	return slices.Contains(expected, actual)
}

func displayName(name string, attributes map[string][]string) string {
	if v, ok := attributes[OrganizationDisplayNameKey]; ok && len(v) > 0 {
		return v[0]
	}
	return name
}

func (r *restKC) convertToOrganization(org KeycloakOrganization) Organization {
	result := OrganizationFromAttributes(org.Alias, org.Attributes)
	result.Name = displayName(org.Name, org.Attributes)
	return result
}

// OrganizationFromAttributes derives an organization from its Keycloak
// attributes. Exported so callers that read the same attributes from an access
// token claim cannot drift from what the Admin REST API returns. Name is not
// derivable from attributes alone and is left to the caller.
func OrganizationFromAttributes(alias string, attributes map[string][]string) Organization {
	result := Organization{
		ID:                      alias,
		BillingCollectionMethod: extractBillingCollectionMethod(attributes),
		Status:                  extractStatus(attributes),
	}
	if v, ok := FirstAttr(attributes, OrganizationMarketplaceKey); ok && v != "" {
		marketplace := OrganizationMarketplaceProvider(v)
		result.Marketplace = &marketplace
		switch marketplace {
		case OrganizationMarketplaceProviderAWS:
			awsMarketplace := AWSMarketplaceFromKeycloakAttributes(attributes)
			result.AWSMarketplace = &awsMarketplace
		case OrganizationMarketplaceProviderVercel:
			vercelMarketplace := VercelMarketplaceFromKeycloakAttributes(attributes)
			result.VercelMarketplace = &vercelMarketplace
		}
	}
	return result
}

func extractBillingCollectionMethod(attributes map[string][]string) OrganizationBillingCollectionMethod {
	value, ok := FirstAttr(attributes, OrganizationBillingCollectionMethodKey)
	method := OrganizationBillingCollectionMethod(value)
	if !ok || !method.Valid() {
		return OrganizationBillingCollectionMethodUnknown
	}
	return method
}

func extractStatus(attributes map[string][]string) OrganizationStatus {
	// This default apply for organizations with no status in Keycloak (created before org status was introduced)
	status := OrganizationStatus{
		DisabledByAdmin: false,
		BillingStatus:   OrganizationBillingStatusOK,
	}

	if v, ok := FirstAttr(attributes, OrganizationDisabledByAdminKey); ok {
		switch strings.ToLower(v) {
		case "true":
			status.DisabledByAdmin = true
		case "false":
			status.DisabledByAdmin = false
		}
	}

	// Billing status default is `ok` for organizations created before billing integration
	status.BillingStatus = OrganizationBillingStatusOK
	if v, ok := FirstAttr(attributes, OrganizationBillingStatusKey); ok {
		billingStatus := OrganizationBillingStatus(v)
		switch billingStatus {
		case OrganizationBillingStatusOK,
			OrganizationBillingStatusInvoiceOverdue,
			OrganizationBillingStatusNoPaymentMethod,
			OrganizationBillingStatusDeletionRequested:
			status.BillingStatus = billingStatus
		default:
			// If the billing status is unrecognized, set it to unknown
			status.BillingStatus = OrganizationBillingStatusUnknown
		}
	}

	if v, ok := FirstAttr(attributes, OrganizationAdminReasonKey); ok {
		status.AdminReason = &v
	}
	if v, ok := FirstAttr(attributes, OrganizationBillingReasonKey); ok {
		status.BillingReason = &v
	}

	if v, ok := FirstAttr(attributes, OrganizationLastUpdatedKey); ok {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			status.LastUpdated = t.UTC()
		}
	}
	if status.LastUpdated.IsZero() {
		status.LastUpdated = time.Unix(0, 0).UTC()
	}

	if v, ok := FirstAttr(attributes, OrganizationCreatedAtKey); ok {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			createdAt := t.UTC()
			status.CreatedAt = &createdAt
		}
	}

	// GetOrganization validates this timestamp before returning included deleted organizations.
	if v, ok := FirstAttr(attributes, OrganizationDeletedAtKey); ok {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			deletedAt := t.UTC()
			status.DeletedAt = &deletedAt
		}
	}

	status.UsageTier = OrganizationUsageTierT1
	if v, ok := FirstAttr(attributes, OrganizationUsageTierKey); ok && OrganizationUsageTier(v) == OrganizationUsageTierT2 {
		status.UsageTier = OrganizationUsageTierT2
	}

	return status
}

// FirstAttr reads a multi-valued Keycloak attribute. An empty or blank
// value reports absent, so callers cannot mistake it for a real setting.
func FirstAttr(attrs map[string][]string, key string) (string, bool) {
	if attrs == nil {
		return "", false
	}
	v, ok := attrs[key]
	if !ok || len(v) == 0 {
		return "", false
	}
	s := strings.TrimSpace(v[0])
	return s, s != ""
}
