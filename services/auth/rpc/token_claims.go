package rpc

import (
	"encoding/json"

	"xata/internal/token"
	"xata/services/auth/keycloak"

	"github.com/golang-jwt/jwt/v5"
	"k8s.io/utils/ptr"
)

const (
	// organizationClaim is written by Keycloak's Organization Membership mapper,
	// keyed by organization alias, which is the Xata organization ID.
	organizationClaim = "organization"
	emailClaim        = "email"
)

// xataOrganizationAttributes are the keys Xata writes on every organization it
// creates. At least one must be present for the claim to be trusted, because the
// mapper can be configured to emit organizations carrying only unrelated keys,
// which would resolve to the enabled defaults and hide a suspended organization.
var xataOrganizationAttributes = []string{
	keycloak.OrganizationDisplayNameKey,
	keycloak.OrganizationBillingStatusKey,
	keycloak.OrganizationUsageTierKey,
	keycloak.OrganizationCreatedAtKey,
	keycloak.OrganizationLastUpdatedKey,
}

// claimsFromAccessToken rebuilds what buildUserClaims fetches over the Keycloak
// Admin REST API out of the signed access token itself. It reports false when
// the token cannot be trusted to carry the full picture, so the caller falls
// back to Keycloak: a user with no organizations is indistinguishable from a
// missing claim, so a miss is not an error.
func claimsFromAccessToken(userID string, mc jwt.MapClaims) (*token.Claims, bool) {
	// The email reaches billing customer creation and analytics targeting, so an
	// absent claim falls back rather than recording a blank address.
	email, _ := mc[emailClaim].(string)
	if email == "" {
		return nil, false
	}

	encoded, err := json.Marshal(mc[organizationClaim])
	if err != nil {
		return nil, false
	}

	// Anything that is not a map of attribute lists (a bare alias list, scalar
	// attributes) fails to decode, which is the safe direction.
	var claimed map[string]map[string][]string
	if err := json.Unmarshal(encoded, &claimed); err != nil || len(claimed) == 0 {
		return nil, false
	}

	organizations := make(map[string]token.Organization, len(claimed))
	for alias, attributes := range claimed {
		if !hasXataAttributes(attributes) {
			return nil, false
		}
		// ListOrganizations hides soft-deleted organizations; the mapper does not.
		if _, ok := keycloak.FirstAttr(attributes, keycloak.OrganizationDeletedAtKey); ok {
			continue
		}
		organizations[alias] = tokenOrganization(keycloak.OrganizationFromAttributes(alias, attributes))
	}

	return &token.Claims{
		ID:            userID,
		Email:         email,
		Organizations: organizations,
		Scopes:        []string{"*"},
		Projects:      []string{"*"},
		Branches:      []string{"*"},
	}, true
}

func hasXataAttributes(attributes map[string][]string) bool {
	for _, key := range xataOrganizationAttributes {
		if _, ok := keycloak.FirstAttr(attributes, key); ok {
			return true
		}
	}
	return false
}

// tokenOrganization projects a Keycloak organization onto the platform claim.
func tokenOrganization(organization keycloak.Organization) token.Organization {
	result := token.Organization{
		ID:          organization.ID,
		Status:      string(organization.Status.EffectiveState()),
		UsageTier:   string(organization.Status.UsageTier),
		Marketplace: string(ptr.Deref(organization.Marketplace, "")),
	}
	if organization.Status.CreatedAt != nil {
		result.CreatedAt = *organization.Status.CreatedAt
	}
	return result
}

// addDefaultOrganization grants access to the OSS default organization, which
// exists outside Keycloak's membership records.
func addDefaultOrganization(organizations map[string]token.Organization, defaultOrgID string) {
	if defaultOrgID == "" {
		return
	}
	if _, exists := organizations[defaultOrgID]; exists {
		return
	}
	organizations[defaultOrgID] = token.Organization{ID: defaultOrgID, Status: token.OrgEnabledStatus}
}
