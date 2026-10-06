package keycloak

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

func (r *restKC) GetOrganizationDomains(ctx context.Context, realm, organizationID string) ([]Domain, error) {
	organization, err := r.searchOrganization(ctx, realm, organizationID)
	if err != nil {
		return nil, fmt.Errorf("get organization: %w", err)
	}
	return organization.Domains, nil
}

// OrganizationsForDomain counts an unverified entry too, since Keycloak refuses a
// second holder either way. It returns every holder rather than the first:
// Keycloak checks uniqueness in application code only, so two concurrent writes
// can both land. Search also matches names, so a hit only counts when the
// organization really lists the domain.
func (r *restKC) OrganizationsForDomain(ctx context.Context, realm, domain string) ([]string, error) {
	orgsURL, err := r.buildRealmURL(realm, "organizations")
	if err != nil {
		return nil, fmt.Errorf("build organizations URL: %w", err)
	}

	queryParams := map[string]string{
		"search":              domain,
		"exact":               "true",
		"briefRepresentation": "true",
	}
	resp, err := r.makeAuthenticatedRequest(ctx, http.MethodGet, orgsURL, queryParams, nil)
	if err != nil {
		return nil, fmt.Errorf("search organizations by domain: %w", err)
	}
	if !r.isSuccessStatus(resp.StatusCode(), http.StatusOK) {
		return nil, fmt.Errorf("search organizations by domain %s: unexpected status %d: %s", domain, resp.StatusCode(), resp.String())
	}

	var organizations []KeycloakOrganization
	if err := json.Unmarshal(resp.Body(), &organizations); err != nil {
		return nil, fmt.Errorf("unmarshal organizations: %w", err)
	}
	var holders []string
	for _, org := range organizations {
		for _, d := range org.Domains {
			if strings.EqualFold(d.Name, domain) {
				holders = append(holders, org.Alias)
				break
			}
		}
	}
	return holders, nil
}

// SetOrganizationDomains replaces the whole set: Keycloak has no per-domain endpoint.
func (r *restKC) SetOrganizationDomains(ctx context.Context, realm, organizationID string, domains []Domain) error {
	organization, err := r.searchOrganization(ctx, realm, organizationID)
	if err != nil {
		return fmt.Errorf("get organization: %w", err)
	}

	organization.Domains = domains

	orgURL, err := r.buildRealmURL(realm, "organizations", organization.ID)
	if err != nil {
		return fmt.Errorf("build organization URL: %w", err)
	}

	resp, err := r.makeAuthenticatedRequest(ctx, http.MethodPut, orgURL, nil, organization)
	if err != nil {
		return fmt.Errorf("set organization domains: %w", err)
	}
	// A 409 is a claim that lost the race for the same domain.
	badRequest := resp.StatusCode() == http.StatusBadRequest
	switch {
	case resp.StatusCode() == http.StatusConflict, badRequest && strings.Contains(resp.String(), "is already linked to organization"):
		return ErrDomainAlreadyClaimed{}
	case badRequest && strings.Contains(resp.String(), "Invalid domain format"):
		return ErrInvalidDomain{}
	}
	if !r.isSuccessStatus(resp.StatusCode(), http.StatusOK, http.StatusNoContent) {
		return fmt.Errorf("set organization domains for %s: unexpected status %d: %s", organizationID, resp.StatusCode(), resp.String())
	}
	return nil
}

// GetSSOPendingDomains returns claims not yet verified. They live in an
// attribute rather than Keycloak's domain list; the sso package doc says why.
func (r *restKC) GetSSOPendingDomains(ctx context.Context, realm, organizationID string) ([]string, error) {
	organization, err := r.searchOrganization(ctx, realm, organizationID)
	if err != nil {
		return nil, fmt.Errorf("get organization: %w", err)
	}
	return ssoPendingDomains(organization), nil
}

func ssoPendingDomains(organization KeycloakOrganization) []string {
	raw, ok := FirstAttr(organization.Attributes, OrganizationSSOPendingDomainsKey)
	if !ok {
		return nil
	}

	var domains []string
	if err := json.Unmarshal([]byte(raw), &domains); err != nil {
		// An unreadable marker costs a re-claim; erroring would wedge every read.
		return nil
	}
	return domains
}

// SetSSOPendingDomains replaces the set. Empty clears the attribute rather
// than storing "[]".
func (r *restKC) SetSSOPendingDomains(ctx context.Context, realm, organizationID string, domains []string) error {
	encoded := ""
	if len(domains) > 0 {
		marshalled, err := json.Marshal(domains)
		if err != nil {
			return fmt.Errorf("marshal pending domains: %w", err)
		}
		encoded = string(marshalled)
	}

	if _, err := r.UpdateOrganization(ctx, realm, organizationID, OrganizationUpdate{SSOPendingDomains: &encoded}); err != nil {
		return fmt.Errorf("set pending domains: %w", err)
	}
	return nil
}

func (r *restKC) GetSSOOrganization(ctx context.Context, realm, organizationID string) (SSOOrganization, error) {
	organization, err := r.searchOrganization(ctx, realm, organizationID)
	if err != nil {
		return SSOOrganization{}, fmt.Errorf("get organization: %w", err)
	}
	return toSSOOrganization(organization), nil
}

// ListSSOOrganizations returns every organization holding a domain, verified or
// revoked. There is no server-side filter for that, so it walks the realm.
// briefRepresentation=false is required, not merely nicer: the brief form
// carries domains but drops attributes, and the recheck needs both.
func (r *restKC) ListSSOOrganizations(ctx context.Context, realm string) ([]SSOOrganization, error) {
	orgsURL, err := r.buildRealmURL(realm, "organizations")
	if err != nil {
		return nil, err
	}

	const pageSize = 200

	result := make([]SSOOrganization, 0)
	fetched := pageSize
	for first := 0; fetched >= pageSize; first += pageSize {
		queryParams := map[string]string{
			"briefRepresentation": "false",
			"first":               fmt.Sprintf("%d", first),
			"max":                 fmt.Sprintf("%d", pageSize),
		}

		resp, err := r.makeAuthenticatedRequest(ctx, http.MethodGet, orgsURL, queryParams, nil)
		if err != nil {
			return nil, fmt.Errorf("list organizations: %w", err)
		}
		if !r.isSuccessStatus(resp.StatusCode(), http.StatusOK) {
			return nil, fmt.Errorf("list organizations: unexpected status %d: %s", resp.StatusCode(), resp.String())
		}

		var organizations []KeycloakOrganization
		if err := json.Unmarshal(resp.Body(), &organizations); err != nil {
			return nil, fmt.Errorf("unmarshal organization list: %w", err)
		}

		for _, org := range organizations {
			if _, deleted := FirstAttr(org.Attributes, OrganizationDeletedAtKey); deleted {
				continue
			}
			if len(org.Domains) == 0 {
				continue
			}
			result = append(result, toSSOOrganization(org))
		}

		fetched = len(organizations)
	}

	return result, nil
}

func toSSOOrganization(organization KeycloakOrganization) SSOOrganization {
	missingSince, _ := FirstAttr(organization.Attributes, OrganizationSSODomainsMissingSinceKey)
	return SSOOrganization{
		Alias:               organization.Alias,
		Domains:             organization.Domains,
		PendingDomains:      ssoPendingDomains(organization),
		DomainsMissingSince: missingSince,
	}
}
