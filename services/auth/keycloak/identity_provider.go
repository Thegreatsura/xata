package keycloak

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"strings"
)

// Config keys Keycloak defines in org.keycloak.models.OrganizationModel.
const (
	IdentityProviderOrganizationDomainKey       = "kc.org.domain"
	IdentityProviderRedirectModeEmailMatchesKey = "kc.org.broker.redirect.mode.email-matches"

	configTrue = "true"
)

// IdentityProvider mirrors the fields of Keycloak's IdentityProviderRepresentation
// an organization's provider needs. Since Keycloak 26 hideOnLogin is top-level,
// not config["hideOnLoginPage"].
type IdentityProvider struct {
	Alias       string            `json:"alias"`
	DisplayName string            `json:"displayName,omitempty"`
	ProviderID  string            `json:"providerId"`
	Enabled     bool              `json:"enabled"`
	HideOnLogin bool              `json:"hideOnLogin"`
	TrustEmail  bool              `json:"trustEmail"`
	Config      map[string]string `json:"config,omitempty"`
}

func (idp IdentityProvider) Issuer() string { return idp.Config["issuer"] }

func (idp IdentityProvider) ClientID() string { return idp.Config["clientId"] }

// OIDCEndpoints must be set explicitly; Keycloak does not derive them from the
// issuer at runtime.
type OIDCEndpoints struct {
	AuthorizationURL string
	TokenURL         string
	JWKSURL          string
	UserInfoURL      string
	LogoutURL        string
}

// OrganizationIdentityProviderAlias names an organization's provider on one domain.
func OrganizationIdentityProviderAlias(organizationID, domain string) string {
	return "sso-" + organizationID + "-" + strings.ReplaceAll(domain, ".", "-")
}

type IdentityProviderSpec struct {
	OrganizationID string
	Domain         string
	// ProviderID is Keycloak's provider type, e.g. "oidc" or "google".
	ProviderID   string
	DisplayName  string
	Issuer       string
	ClientID     string
	ClientSecret string
	Endpoints    OIDCEndpoints
	// Extra carries provider-specific config, such as Google's hostedDomain.
	Extra map[string]string
}

// NewIdentityProvider builds the representation for one verified domain. See
// TestNewIdentityProvider for the invariants its defaults carry.
func NewIdentityProvider(spec IdentityProviderSpec) IdentityProvider {
	config := map[string]string{
		"clientId":          spec.ClientID,
		"clientSecret":      spec.ClientSecret,
		"clientAuthMethod":  "client_secret_post",
		"issuer":            spec.Issuer,
		"defaultScope":      "openid profile email",
		"validateSignature": configTrue,
		"useJwksUrl":        configTrue,
	}
	for key, value := range map[string]string{
		"authorizationUrl": spec.Endpoints.AuthorizationURL,
		"tokenUrl":         spec.Endpoints.TokenURL,
		"jwksUrl":          spec.Endpoints.JWKSURL,
		"userInfoUrl":      spec.Endpoints.UserInfoURL,
		"logoutUrl":        spec.Endpoints.LogoutURL,
	} {
		if value != "" {
			config[key] = value
		}
	}
	maps.Copy(config, spec.Extra)

	idp := IdentityProvider{
		Alias:       OrganizationIdentityProviderAlias(spec.OrganizationID, spec.Domain),
		DisplayName: spec.DisplayName,
		ProviderID:  spec.ProviderID,
		Enabled:     true,
		// Never a button on the login page. Members arrive by the email-domain
		// redirect, or by a kc_idp_hint link, which ignores this flag.
		HideOnLogin: true,
		TrustEmail:  true,
		Config:      config,
	}
	idp.SetOrganizationDomain(spec.Domain)
	return idp
}

func (idp IdentityProvider) OrganizationDomain() string {
	return idp.Config[IdentityProviderOrganizationDomainKey]
}

func (idp *IdentityProvider) SetOrganizationDomain(domain string) {
	if idp.Config == nil {
		idp.Config = make(map[string]string)
	}
	idp.Config[IdentityProviderOrganizationDomainKey] = domain
}

func (idp *IdentityProvider) SetRedirectOnEmailMatch(redirect bool) {
	if !redirect {
		delete(idp.Config, IdentityProviderRedirectModeEmailMatchesKey)
		return
	}
	if idp.Config == nil {
		idp.Config = make(map[string]string)
	}
	idp.Config[IdentityProviderRedirectModeEmailMatchesKey] = configTrue
}

func (idp IdentityProvider) RedirectsOnEmailMatch() bool {
	return idp.Config[IdentityProviderRedirectModeEmailMatchesKey] == configTrue
}

func (r *restKC) identityProviderURL(realm, alias string) (string, error) {
	url, err := r.buildRealmURL(realm, "identity-provider", "instances", alias)
	if err != nil {
		return "", fmt.Errorf("build identity provider URL: %w", err)
	}
	return url, nil
}

func (r *restKC) organizationIdentityProvidersURL(ctx context.Context, realm, organizationID string) (string, error) {
	organization, err := r.searchOrganization(ctx, realm, organizationID)
	if err != nil {
		return "", fmt.Errorf("get organization: %w", err)
	}
	url, err := r.buildRealmURL(realm, "organizations", organization.ID, "identity-providers")
	if err != nil {
		return "", fmt.Errorf("build organization identity providers URL: %w", err)
	}
	return url, nil
}

// UpsertIdentityProvider creates the provider, or replaces it on 409. Keycloak
// has no upsert.
func (r *restKC) UpsertIdentityProvider(ctx context.Context, realm string, idp IdentityProvider) error {
	instancesURL, err := r.buildRealmURL(realm, "identity-provider", "instances")
	if err != nil {
		return fmt.Errorf("build identity provider URL: %w", err)
	}

	resp, err := r.makeAuthenticatedRequest(ctx, http.MethodPost, instancesURL, nil, idp)
	if err != nil {
		return fmt.Errorf("create identity provider: %w", err)
	}
	if r.isSuccessStatus(resp.StatusCode(), http.StatusCreated, http.StatusOK, http.StatusNoContent) {
		return nil
	}
	if resp.StatusCode() != http.StatusConflict {
		return fmt.Errorf("create identity provider %s: unexpected status %d: %s", idp.Alias, resp.StatusCode(), resp.String())
	}

	idpURL, err := r.identityProviderURL(realm, idp.Alias)
	if err != nil {
		return err
	}

	resp, err = r.makeAuthenticatedRequest(ctx, http.MethodPut, idpURL, nil, idp)
	if err != nil {
		return fmt.Errorf("update identity provider: %w", err)
	}
	if !r.isSuccessStatus(resp.StatusCode(), http.StatusOK, http.StatusNoContent) {
		return fmt.Errorf("update identity provider %s: unexpected status %d: %s", idp.Alias, resp.StatusCode(), resp.String())
	}
	return nil
}

func (r *restKC) DeleteIdentityProvider(ctx context.Context, realm, alias string) error {
	idpURL, err := r.identityProviderURL(realm, alias)
	if err != nil {
		return err
	}

	resp, err := r.makeAuthenticatedRequest(ctx, http.MethodDelete, idpURL, nil, nil)
	if err != nil {
		return fmt.Errorf("delete identity provider: %w", err)
	}
	if resp.StatusCode() == http.StatusNotFound {
		return nil
	}
	if !r.isSuccessStatus(resp.StatusCode(), http.StatusOK, http.StatusNoContent) {
		return fmt.Errorf("delete identity provider %s: unexpected status %d: %s", alias, resp.StatusCode(), resp.String())
	}
	return nil
}

func (r *restKC) LinkIdentityProviderToOrganization(ctx context.Context, realm, organizationID, alias string) error {
	linkURL, err := r.organizationIdentityProvidersURL(ctx, realm, organizationID)
	if err != nil {
		return err
	}

	// Keycloak takes the bare alias as the whole body, not an object wrapping it.
	body, err := json.Marshal(alias)
	if err != nil {
		return fmt.Errorf("marshal identity provider alias: %w", err)
	}

	resp, err := r.makeAuthenticatedRequest(ctx, http.MethodPost, linkURL, nil, string(body))
	if err != nil {
		return fmt.Errorf("link identity provider: %w", err)
	}
	// Already linked is the state the caller asked for.
	if resp.StatusCode() == http.StatusConflict {
		return nil
	}
	if !r.isSuccessStatus(resp.StatusCode(), http.StatusCreated, http.StatusOK, http.StatusNoContent) {
		return fmt.Errorf("link identity provider %s to organization %s: unexpected status %d: %s", alias, organizationID, resp.StatusCode(), resp.String())
	}
	return nil
}

func (r *restKC) ListOrganizationIdentityProviders(ctx context.Context, realm, organizationID string) ([]IdentityProvider, error) {
	listURL, err := r.organizationIdentityProvidersURL(ctx, realm, organizationID)
	if err != nil {
		return nil, err
	}

	resp, err := r.makeAuthenticatedRequest(ctx, http.MethodGet, listURL, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("list organization identity providers: %w", err)
	}
	if !r.isSuccessStatus(resp.StatusCode(), http.StatusOK) {
		return nil, fmt.Errorf("list identity providers for %s: unexpected status %d: %s", organizationID, resp.StatusCode(), resp.String())
	}

	var providers []IdentityProvider
	if err := json.Unmarshal(resp.Body(), &providers); err != nil {
		return nil, fmt.Errorf("unmarshal organization identity providers: %w", err)
	}
	return providers, nil
}
