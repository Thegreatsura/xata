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

// SetOrganizationDomains replaces the whole set: Keycloak has no per-domain
// endpoint. It rejects a domain another organization holds, and clears
// kc.org.domain on any provider bound to one being removed.
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
	// Keycloak returns 400 for both, so only the message separates them, and its
	// wording has changed between releases. Match the part the phrasings share.
	if resp.StatusCode() == http.StatusBadRequest {
		body := strings.ToLower(resp.String())
		switch {
		case strings.Contains(body, "already linked to"):
			return ErrDomainAlreadyClaimed{}
		case strings.Contains(body, "invalid domain format"), strings.Contains(body, "domain is invalid"):
			return ErrInvalidDomain{}
		}
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

	raw, ok := FirstAttr(organization.Attributes, OrganizationSSOPendingDomainsKey)
	if !ok {
		return nil, nil
	}

	var domains []string
	if err := json.Unmarshal([]byte(raw), &domains); err != nil {
		// An unreadable marker costs a re-claim; erroring would wedge every read.
		return nil, nil
	}
	return domains, nil
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

// ListSSOOrganizations returns every organization holding a verified domain.
// There is no server-side filter for that, so it walks the realm.
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
			if !hasVerifiedDomain(org.Domains) {
				continue
			}
			missingSince, _ := FirstAttr(org.Attributes, OrganizationSSODomainsMissingSinceKey)
			result = append(result, SSOOrganization{
				Alias:               org.Alias,
				Domains:             org.Domains,
				DomainsMissingSince: missingSince,
			})
		}

		fetched = len(organizations)
	}

	return result, nil
}

func hasVerifiedDomain(domains []Domain) bool {
	for _, d := range domains {
		if d.Verified {
			return true
		}
	}
	return false
}
