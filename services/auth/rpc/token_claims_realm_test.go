package rpc

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"xata/services/auth/keycloak"

	"github.com/stretchr/testify/require"
)

const (
	realmFile          = "../../../charts/keycloak/files/realm.json"
	localRealmFile     = "../../../kustomize/overlays/local/auth/keycloak/realm.json"
	trimmingMapper     = "xata-organization-membership-mapper"
	stockMapper        = "oidc-organization-membership-mapper"
	includedAttributes = "includedAttributes"
	addRole            = "addRole"
)

type realmMapper struct {
	ProtocolMapper string            `json:"protocolMapper"`
	Config         map[string]string `json:"config"`
}

func realmScopeMappers(t *testing.T, file, scope string) []realmMapper {
	t.Helper()
	raw, err := os.ReadFile(file)
	require.NoError(t, err)

	var realm struct {
		ClientScopes []struct {
			Name            string        `json:"name"`
			ProtocolMappers []realmMapper `json:"protocolMappers"`
		} `json:"clientScopes"`
	}
	require.NoError(t, json.Unmarshal(raw, &realm))

	for _, s := range realm.ClientScopes {
		if s.Name == scope {
			return s.ProtocolMappers
		}
	}
	require.FailNow(t, file+" has no client scope "+scope)
	return nil
}

// realmTrimmingMapper returns the mapper that writes the organization claim
// into access tokens for organization:*.
func realmTrimmingMapper(t *testing.T, file string) realmMapper {
	t.Helper()
	for _, mapper := range realmScopeMappers(t, file, "organization:*") {
		if mapper.ProtocolMapper == trimmingMapper && mapper.Config["access.token.claim"] == "true" {
			return mapper
		}
	}
	require.FailNow(t, "organization:* has no access token mapper of type "+trimmingMapper)
	return realmMapper{}
}

// realmIncludedAttributes returns the attributes the realm's trimming mapper
// keeps in the access token.
func realmIncludedAttributes(t *testing.T) []string {
	t.Helper()
	return strings.Split(realmTrimmingMapper(t, realmFile).Config[includedAttributes], "##")
}

// trimOrganizations mirrors OrganizationMembershipMapper.trim and addRoles in
// the Keycloak extensions.
func trimOrganizations(organizations map[string]map[string][]string, included []string) map[string]map[string][]string {
	trimmed := map[string]map[string][]string{}
	for alias, attributes := range organizations {
		if _, deleted := keycloak.FirstAttr(attributes, keycloak.OrganizationDeletedAtKey); deleted {
			continue
		}
		kept := map[string][]string{}
		for key, values := range attributes {
			if slices.Contains(included, key) {
				kept[key] = values
			}
		}
		kept["role"] = []string{"admin"}
		trimmed[alias] = kept
	}
	return trimmed
}

func tokenWith(t *testing.T, organizations map[string]map[string][]string) string {
	t.Helper()
	claims := map[string]any{"email": "user@xata.io"}
	if len(organizations) > 0 {
		claims["organization"] = organizations
	}
	raw, err := json.Marshal(claims)
	require.NoError(t, err)
	return string(raw)
}

// The realm trims the organization claim, so every attribute the token path
// reads must survive the trim.
func TestRealmOrganizationClaimTrim(t *testing.T) {
	included := realmIncludedAttributes(t)

	for name, test := range map[string]struct {
		organizations map[string]map[string][]string
		// wantFallback marks inputs where only the trimmed claim falls back to
		// Keycloak, which returns the same organizations at a higher cost.
		wantFallback bool
	}{
		"every attribute Xata writes": {
			organizations: map[string]map[string][]string{
				"acme": {
					keycloak.OrganizationDisplayNameKey:             {"Acme Inc"},
					keycloak.OrganizationDisabledByAdminKey:         {"false"},
					keycloak.OrganizationBillingStatusKey:           {"ok"},
					keycloak.OrganizationBillingCollectionMethodKey: {"charge_automatically"},
					keycloak.OrganizationBillingReasonKey:           {""},
					keycloak.OrganizationUsageTierKey:               {"t2"},
					keycloak.OrganizationCreatedAtKey:               {"2026-04-30T13:23:06Z"},
					keycloak.OrganizationLastUpdatedKey:             {"2026-09-01T10:00:00Z"},
					keycloak.OrganizationSSOPendingDomainsKey:       {`["acme.com"]`},
					keycloak.OrganizationSSODomainsMissingSinceKey:  {"2026-09-02T10:00:00Z"},
				},
			},
		},
		"disabled by admin": {
			organizations: map[string]map[string][]string{
				"acme": {
					keycloak.OrganizationDisabledByAdminKey: {"true"},
					keycloak.OrganizationAdminReasonKey:     {"abuse"},
					keycloak.OrganizationBillingStatusKey:   {"ok"},
					keycloak.OrganizationUsageTierKey:       {"t1"},
				},
			},
		},
		"billing suspended": {
			organizations: map[string]map[string][]string{
				"overdue": {
					keycloak.OrganizationBillingStatusKey: {"invoice_overdue"},
					keycloak.OrganizationBillingReasonKey: {"invoice unpaid"},
				},
				"leaving": {keycloak.OrganizationBillingStatusKey: {"deletion_requested"}},
				"no_card": {keycloak.OrganizationBillingStatusKey: {"no_payment_method"}},
			},
		},
		"aws marketplace": {
			organizations: map[string]map[string][]string{
				"acme": {
					keycloak.OrganizationBillingStatusKey: {"ok"},
					keycloak.OrganizationMarketplaceKey:   {"aws"},
					keycloak.OrganizationAWSCustomerIDKey: {"cust-1"},
					keycloak.OrganizationAWSProductIDKey:  {"prod-1"},
					keycloak.OrganizationAWSAccountIDKey:  {"123456789012"},
				},
			},
		},
		"vercel marketplace": {
			organizations: map[string]map[string][]string{
				"acme": {
					keycloak.OrganizationBillingStatusKey:        {"ok"},
					keycloak.OrganizationMarketplaceKey:          {"vercel"},
					keycloak.OrganizationVercelInstallationIDKey: {"icfg_1"},
					keycloak.OrganizationVercelAccountIDKey:      {"team_1"},
					keycloak.OrganizationVercelAccountEmailKey:   {"owner@acme.com"},
				},
			},
		},
		"soft-deleted beside a live organization": {
			organizations: map[string]map[string][]string{
				"acme": {keycloak.OrganizationBillingStatusKey: {"ok"}},
				"gone": {
					keycloak.OrganizationBillingStatusKey: {"ok"},
					keycloak.OrganizationDeletedAtKey:     {"2026-07-01T00:00:00Z"},
				},
			},
		},
		"legacy organization with a last update": {
			organizations: map[string]map[string][]string{
				"acme": {
					keycloak.OrganizationDisplayNameKey: {"Acme Inc"},
					keycloak.OrganizationLastUpdatedKey: {"2026-04-30T13:23:06Z"},
				},
			},
		},
		"dev user organization": {
			organizations: map[string]map[string][]string{
				"acme": {keycloak.OrganizationUsageTierKey: {"t2"}},
			},
		},
		"legacy organization with only a display name": {
			organizations: map[string]map[string][]string{
				"acme": {keycloak.OrganizationDisplayNameKey: {"Acme Inc"}},
			},
			wantFallback: true,
		},
		"only soft-deleted organizations": {
			organizations: map[string]map[string][]string{
				"gone": {
					keycloak.OrganizationBillingStatusKey: {"ok"},
					keycloak.OrganizationDeletedAtKey:     {"2026-07-01T00:00:00Z"},
				},
			},
			wantFallback: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			want, wantOK := claimsFromAccessToken("user-1", mapClaims(t, tokenWith(t, test.organizations)))
			require.True(t, wantOK, "the full claim must be trusted")

			got, ok := claimsFromAccessToken("user-1", mapClaims(t, tokenWith(t, trimOrganizations(test.organizations, included))))
			if test.wantFallback {
				require.False(t, ok)
				return
			}
			require.True(t, ok)
			require.Equal(t, want, got)
		})
	}
}

// Every attribute that decides whether an organization is enabled must be
// kept, or a suspended organization reads as enabled.
func TestRealmOrganizationClaimKeepsStatus(t *testing.T) {
	included := realmIncludedAttributes(t)
	for _, key := range []string{
		keycloak.OrganizationDisabledByAdminKey,
		keycloak.OrganizationBillingStatusKey,
	} {
		require.Contains(t, included, key)
	}
	require.True(t, slices.ContainsFunc(included, func(key string) bool {
		return slices.Contains(xataOrganizationAttributes, key)
	}), "no attribute that marks the claim as trusted survives the trim")
}

func TestRealmOrganizationMappers(t *testing.T) {
	for _, file := range []string{realmFile, localRealmFile} {
		t.Run(file, func(t *testing.T) {
			require.Equal(t, "true", realmTrimmingMapper(t, file).Config[addRole])

			// Keycloak creates the organization scope with a stock mapper that
			// replaces the claim with an alias list when a refresh requests it.
			for _, scope := range []string{"organization", "organization:*"} {
				stock := 0
				for _, mapper := range realmScopeMappers(t, file, scope) {
					if mapper.ProtocolMapper != stockMapper {
						continue
					}
					stock++
					for _, flag := range []string{"access.token.claim", "id.token.claim", "introspection.token.claim", "userinfo.token.claim"} {
						require.Equal(t, "false", mapper.Config[flag], "%s %s", scope, flag)
					}
				}
				require.Equal(t, 1, stock, "%s must keep the stock mapper (keycloak#49021)", scope)
			}
		})
	}
}
