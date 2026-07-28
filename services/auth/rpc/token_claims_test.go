package rpc

import (
	"encoding/json"
	"testing"
	"time"

	"xata/internal/token"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

// mapClaims parses the JSON shape Keycloak emits, so the tests exercise the
// same types the JWT decoder produces rather than hand-built Go maps.
func mapClaims(t *testing.T, raw string) jwt.MapClaims {
	t.Helper()
	var claims jwt.MapClaims
	require.NoError(t, json.Unmarshal([]byte(raw), &claims))
	return claims
}

func TestClaimsFromAccessToken(t *testing.T) {
	for name, test := range map[string]struct {
		raw      string
		wantOK   bool
		wantOrgs map[string]token.Organization
		wantMail string
	}{
		"enabled organization with attributes": {
			raw: `{
				"email": "user@xata.io",
				"organization": {
					"okta": {
						"billingStatus": ["ok"],
						"disabledByAdmin": ["false"],
						"usageTier": ["t1"],
						"createdAt": ["2026-04-30T13:23:06Z"]
					}
				}
			}`,
			wantOK:   true,
			wantMail: "user@xata.io",
			wantOrgs: map[string]token.Organization{
				"okta": {
					ID:        "okta",
					Status:    "enabled",
					UsageTier: "t1",
					CreatedAt: time.Date(2026, 4, 30, 13, 23, 6, 0, time.UTC),
				},
			},
		},
		"multiple organizations": {
			raw: `{
				"email": "user@xata.io",
				"organization": {
					"okta": {"billingStatus": ["ok"], "usageTier": ["t2"]},
					"acme": {"billingStatus": ["invoice_overdue"], "usageTier": ["t1"]}
				}
			}`,
			wantOK: true,
			wantOrgs: map[string]token.Organization{
				"okta": {ID: "okta", Status: "enabled", UsageTier: "t2"},
				"acme": {ID: "acme", Status: "disabled", UsageTier: "t1"},
			},
		},
		"disabled by admin is disabled": {
			raw:    `{"email": "user@xata.io", "organization": {"okta": {"billingStatus": ["ok"], "disabledByAdmin": ["true"], "usageTier": ["t1"]}}}`,
			wantOK: true,
			wantOrgs: map[string]token.Organization{
				"okta": {ID: "okta", Status: "disabled", UsageTier: "t1"},
			},
		},
		"marketplace is carried over": {
			raw:    `{"email": "user@xata.io", "organization": {"okta": {"billingStatus": ["ok"], "usageTier": ["t1"], "marketplace": ["aws"]}}}`,
			wantOK: true,
			wantOrgs: map[string]token.Organization{
				"okta": {ID: "okta", Status: "enabled", UsageTier: "t1", Marketplace: "aws"},
			},
		},
		"marketplace and usage tier are carried over": {
			raw:    `{"email": "user@xata.io", "organization": {"okta": {"billingStatus": ["ok"], "usageTier": ["t2"], "marketplace": ["aws"]}}}`,
			wantOK: true,
			wantOrgs: map[string]token.Organization{
				"okta": {ID: "okta", Status: "enabled", UsageTier: "t2", Marketplace: "aws"},
			},
		},
		"missing claim falls back": {
			raw:    `{"email": "user@xata.io"}`,
			wantOK: false,
		},
		"empty claim falls back": {
			raw:    `{"email": "user@xata.io", "organization": {}}`,
			wantOK: false,
		},
		// The String-typed mapper emits aliases without attributes, which would
		// make a suspended organization read as enabled.
		"alias list without attributes falls back": {
			raw:    `{"email": "user@xata.io", "organization": ["okta"]}`,
			wantOK: false,
		},
		// Organizations created before status attributes existed carry only a
		// display name, and Keycloak defaults them to enabled/ok/t1.
		"legacy organization with only a display name": {
			raw:    `{"email": "user@xata.io", "organization": {"okta": {"displayName": ["SferaDev"], "lastUpdated": ["2026-04-30T13:23:06Z"]}}}`,
			wantOK: true,
			wantOrgs: map[string]token.Organization{
				"okta": {ID: "okta", Status: "enabled", UsageTier: "t1"},
			},
		},
		"legacy and modern organizations together": {
			raw: `{"email": "user@xata.io", "organization": {
				"okta": {"displayName": ["SferaDev"]},
				"acme": {"billingStatus": ["invoice_overdue"], "usageTier": ["t2"]}
			}}`,
			wantOK: true,
			wantOrgs: map[string]token.Organization{
				"okta": {ID: "okta", Status: "enabled", UsageTier: "t1"},
				"acme": {ID: "acme", Status: "disabled", UsageTier: "t2"},
			},
		},
		// Soft-deleted organizations are hidden by ListOrganizations, so the
		// token path must hide them too.
		"soft-deleted organization is dropped": {
			raw: `{"email": "user@xata.io", "organization": {
				"okta": {"billingStatus": ["ok"], "usageTier": ["t1"]},
				"gone": {"billingStatus": ["ok"], "deletedAt": ["2026-07-01T00:00:00Z"]}
			}}`,
			wantOK: true,
			wantOrgs: map[string]token.Organization{
				"okta": {ID: "okta", Status: "enabled", UsageTier: "t1"},
			},
		},
		// No attributes at all means the mapper is not mapping them, so a
		// suspended organization would read as enabled.
		"organization without attributes falls back": {
			raw:    `{"email": "user@xata.io", "organization": {"okta": {}}}`,
			wantOK: false,
		},
		"scalar claim falls back": {
			raw:    `{"email": "user@xata.io", "organization": "okta"}`,
			wantOK: false,
		},
		"scalar attribute values fall back": {
			raw:    `{"email": "user@xata.io", "organization": {"okta": {"billingStatus": "ok"}}}`,
			wantOK: false,
		},
		// Only keys Xata never writes: the mapper is emitting organizations
		// without their attributes, so status cannot be trusted.
		"organization with only foreign attributes falls back": {
			raw:    `{"email": "user@xata.io", "organization": {"okta": {"id": ["42c3e46f"], "domain": ["xata.io"]}}}`,
			wantOK: false,
		},
		"every organization soft-deleted yields no memberships": {
			raw:      `{"email": "user@xata.io", "organization": {"gone": {"displayName": ["Gone"], "deletedAt": ["2026-07-01T00:00:00Z"]}}}`,
			wantOK:   true,
			wantOrgs: map[string]token.Organization{},
		},
		"empty deletedAt is not deleted": {
			raw:    `{"email": "user@xata.io", "organization": {"okta": {"displayName": ["SferaDev"], "deletedAt": [""]}}}`,
			wantOK: true,
			wantOrgs: map[string]token.Organization{
				"okta": {ID: "okta", Status: "enabled", UsageTier: "t1"},
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			got, ok := claimsFromAccessToken("user-1", mapClaims(t, test.raw))
			require.Equal(t, test.wantOK, ok)
			if !test.wantOK {
				require.Nil(t, got)
				return
			}

			require.Equal(t, "user-1", got.ID)
			require.Equal(t, "user@xata.io", got.Email)
			require.Equal(t, test.wantOrgs, got.Organizations)
			require.Equal(t, []string{"*"}, got.Scopes)
			require.Equal(t, []string{"*"}, got.Projects)
			require.Equal(t, []string{"*"}, got.Branches)
		})
	}
}

func TestAddDefaultOrganization(t *testing.T) {
	for name, test := range map[string]struct {
		defaultOrgID string
		claims       *token.Claims
		wantOrgIDs   []string
		wantOrg      *token.Organization
	}{
		"adds the default organization": {
			defaultOrgID: "default-org",
			claims:       &token.Claims{Organizations: map[string]token.Organization{}},
			wantOrgIDs:   []string{"default-org"},
		},
		"a real membership is never downgraded": {
			defaultOrgID: "okta",
			claims: &token.Claims{Organizations: map[string]token.Organization{
				"okta": {ID: "okta", Status: "disabled", UsageTier: "t2"},
			}},
			wantOrgIDs: []string{"okta"},
			wantOrg:    &token.Organization{ID: "okta", Status: "disabled", UsageTier: "t2"},
		},
		"no default configured": {
			defaultOrgID: "",
			claims:       &token.Claims{Organizations: map[string]token.Organization{"okta": {ID: "okta"}}},
			wantOrgIDs:   []string{"okta"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			addDefaultOrganization(test.claims.Organizations, test.defaultOrgID)
			got := test.claims
			for _, id := range test.wantOrgIDs {
				require.Contains(t, got.Organizations, id)
			}
			require.Len(t, got.Organizations, len(test.wantOrgIDs))
			if test.wantOrg != nil {
				require.Equal(t, *test.wantOrg, got.Organizations[test.wantOrg.ID])
			}
		})
	}
}

// TestClaimsFromTokenGate covers the rollout switch and the revalidation that
// keeps a membership granted after issuance from being denied.
// TestTrustedClaimsDefaultOrganizationOrdering pins that the default
// organization is added before the membership check, so an OSS deployment can
// still reach its own default organization.
func TestTrustedClaimsDefaultOrganizationOrdering(t *testing.T) {
	svc := &AuthService{trustTokenClaims: true, defaultOrgID: "default-org"}

	got, fromToken := svc.trustedClaims("user-1", "default-org",
		mapClaims(t, `{"email": "user@xata.io", "organization": {"okta": {"displayName": ["SferaDev"]}}}`))
	require.True(t, fromToken)
	require.Contains(t, got.Organizations, "default-org")
}

func TestTrustedClaimsGate(t *testing.T) {
	claim := mapClaims(t, `{"email": "user@xata.io", "organization": {"okta": {"displayName": ["SferaDev"]}}}`)

	for name, test := range map[string]struct {
		trustTokenClaims bool
		organizationID   string
		wantFromToken    bool
	}{
		"disabled always falls back":   {trustTokenClaims: false, wantFromToken: false},
		"disabled ignores a known org": {trustTokenClaims: false, organizationID: "okta", wantFromToken: false},
		"enabled with no target org":   {trustTokenClaims: true, wantFromToken: true},
		"enabled with a known org":     {trustTokenClaims: true, organizationID: "okta", wantFromToken: true},
		"enabled with an unknown org":  {trustTokenClaims: true, organizationID: "brand-new", wantFromToken: false},
	} {
		t.Run(name, func(t *testing.T) {
			svc := &AuthService{trustTokenClaims: test.trustTokenClaims}

			got, fromToken := svc.trustedClaims("user-1", test.organizationID, claim)
			require.Equal(t, test.wantFromToken, fromToken)
			if !test.wantFromToken {
				require.Nil(t, got)
				return
			}
			require.Equal(t, "user-1", got.ID)
			require.Contains(t, got.Organizations, "okta")
		})
	}
}
