package rpc

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"xata/internal/token"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
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
		raw        string
		wantReason fallbackReason
		wantOrgs   map[string]token.Organization
		wantMail   string
	}{
		"enabled organization with attributes": {
			raw: `{
				"email": "user@xata.io",
				"organization": {
					"acme": {
						"billingStatus": ["ok"],
						"disabledByAdmin": ["false"],
						"usageTier": ["t1"],
						"createdAt": ["2026-04-30T13:23:06Z"]
					}
				}
			}`,
			wantMail: "user@xata.io",
			wantOrgs: map[string]token.Organization{
				"acme": {
					ID:        "acme",
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
					"acme": {"billingStatus": ["ok"], "usageTier": ["t2"]},
					"globex": {"billingStatus": ["invoice_overdue"], "usageTier": ["t1"]}
				}
			}`,
			wantOrgs: map[string]token.Organization{
				"acme":   {ID: "acme", Status: "enabled", UsageTier: "t2"},
				"globex": {ID: "globex", Status: "disabled", UsageTier: "t1"},
			},
		},
		"disabled by admin is disabled": {
			raw: `{"email": "user@xata.io", "organization": {"acme": {"billingStatus": ["ok"], "disabledByAdmin": ["true"], "usageTier": ["t1"]}}}`,
			wantOrgs: map[string]token.Organization{
				"acme": {ID: "acme", Status: "disabled", UsageTier: "t1"},
			},
		},
		"marketplace is carried over": {
			raw: `{"email": "user@xata.io", "organization": {"acme": {"billingStatus": ["ok"], "usageTier": ["t1"], "marketplace": ["aws"]}}}`,
			wantOrgs: map[string]token.Organization{
				"acme": {ID: "acme", Status: "enabled", UsageTier: "t1", Marketplace: "aws"},
			},
		},
		"marketplace and usage tier are carried over": {
			raw: `{"email": "user@xata.io", "organization": {"acme": {"billingStatus": ["ok"], "usageTier": ["t2"], "marketplace": ["aws"]}}}`,
			wantOrgs: map[string]token.Organization{
				"acme": {ID: "acme", Status: "enabled", UsageTier: "t2", Marketplace: "aws"},
			},
		},
		"missing claim falls back": {
			raw:        `{"email": "user@xata.io"}`,
			wantReason: reasonNoOrgClaim,
		},
		"missing email falls back": {
			raw:        `{"organization": {"acme": {"billingStatus": ["ok"]}}}`,
			wantReason: reasonNoEmail,
		},
		"empty claim falls back": {
			raw:        `{"email": "user@xata.io", "organization": {}}`,
			wantReason: reasonEmptyOrgClaim,
		},
		// The String-typed mapper emits aliases without attributes, which would
		// make a suspended organization read as enabled.
		"alias list without attributes falls back": {
			raw:        `{"email": "user@xata.io", "organization": ["acme"]}`,
			wantReason: reasonOrgClaimMalformed,
		},
		// Organizations created before status attributes existed carry only a
		// display name, and Keycloak defaults them to enabled/ok/t1.
		"legacy organization with only a display name": {
			raw: `{"email": "user@xata.io", "organization": {"acme": {"displayName": ["Acme Inc"], "lastUpdated": ["2026-04-30T13:23:06Z"]}}}`,
			wantOrgs: map[string]token.Organization{
				"acme": {ID: "acme", Status: "enabled", UsageTier: "t1"},
			},
		},
		"legacy and modern organizations together": {
			raw: `{"email": "user@xata.io", "organization": {
				"acme": {"displayName": ["Acme Inc"]},
				"globex": {"billingStatus": ["invoice_overdue"], "usageTier": ["t2"]}
			}}`,
			wantOrgs: map[string]token.Organization{
				"acme":   {ID: "acme", Status: "enabled", UsageTier: "t1"},
				"globex": {ID: "globex", Status: "disabled", UsageTier: "t2"},
			},
		},
		// Soft-deleted organizations are hidden by ListOrganizations, so the
		// token path must hide them too.
		"soft-deleted organization is dropped": {
			raw: `{"email": "user@xata.io", "organization": {
				"acme": {"billingStatus": ["ok"], "usageTier": ["t1"]},
				"gone": {"billingStatus": ["ok"], "deletedAt": ["2026-07-01T00:00:00Z"]}
			}}`,
			wantOrgs: map[string]token.Organization{
				"acme": {ID: "acme", Status: "enabled", UsageTier: "t1"},
			},
		},
		// No attributes at all means the mapper is not mapping them, so a
		// suspended organization would read as enabled.
		"organization without attributes falls back": {
			raw:        `{"email": "user@xata.io", "organization": {"acme": {}}}`,
			wantReason: reasonOrgNotXata,
		},
		"scalar claim falls back": {
			raw:        `{"email": "user@xata.io", "organization": "acme"}`,
			wantReason: reasonOrgClaimMalformed,
		},
		"scalar attribute values fall back": {
			raw:        `{"email": "user@xata.io", "organization": {"acme": {"billingStatus": "ok"}}}`,
			wantReason: reasonOrgClaimMalformed,
		},
		// Only keys Xata never writes: the mapper is emitting organizations
		// without their attributes, so status cannot be trusted.
		"organization with only foreign attributes falls back": {
			raw:        `{"email": "user@xata.io", "organization": {"acme": {"id": ["42c3e46f"], "domain": ["xata.io"]}}}`,
			wantReason: reasonOrgNotXata,
		},
		"every organization soft-deleted yields no memberships": {
			raw:      `{"email": "user@xata.io", "organization": {"gone": {"displayName": ["Gone"], "deletedAt": ["2026-07-01T00:00:00Z"]}}}`,
			wantOrgs: map[string]token.Organization{},
		},
		// The Xata mapper's addRole setting adds the caller's role to each organization.
		"role is ignored": {
			raw: `{"email": "user@xata.io", "organization": {
				"acme": {"billingStatus": ["ok"], "usageTier": ["t2"], "role": ["admin"]},
				"globex": {"billingStatus": ["ok"], "usageTier": ["t1"], "role": []}
			}}`,
			wantOrgs: map[string]token.Organization{
				"acme":   {ID: "acme", Status: "enabled", UsageTier: "t2"},
				"globex": {ID: "globex", Status: "enabled", UsageTier: "t1"},
			},
		},
		"role alone falls back": {
			raw:        `{"email": "user@xata.io", "organization": {"acme": {"role": ["admin"]}}}`,
			wantReason: reasonOrgNotXata,
		},
		"empty deletedAt is not deleted": {
			raw: `{"email": "user@xata.io", "organization": {"acme": {"displayName": ["Acme Inc"], "deletedAt": [""]}}}`,
			wantOrgs: map[string]token.Organization{
				"acme": {ID: "acme", Status: "enabled", UsageTier: "t1"},
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			got, reason := claimsFromAccessToken("user-1", mapClaims(t, test.raw))
			require.Equal(t, test.wantReason, reason)
			if test.wantReason != reasonTrusted {
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
			wantOrg:      &token.Organization{ID: "default-org", Status: token.OrgEnabledStatus, UsageTier: "t2"},
		},
		"a real membership is never downgraded": {
			defaultOrgID: "acme",
			claims: &token.Claims{Organizations: map[string]token.Organization{
				"acme": {ID: "acme", Status: "disabled", UsageTier: "t2"},
			}},
			wantOrgIDs: []string{"acme"},
			wantOrg:    &token.Organization{ID: "acme", Status: "disabled", UsageTier: "t2"},
		},
		"no default configured": {
			defaultOrgID: "",
			claims:       &token.Claims{Organizations: map[string]token.Organization{"acme": {ID: "acme"}}},
			wantOrgIDs:   []string{"acme"},
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

// TestTrustedClaimsDefaultOrganizationOrdering pins that the default
// organization is added before the membership check, so an OSS deployment can
// still reach its own default organization.
func TestTrustedClaimsDefaultOrganizationOrdering(t *testing.T) {
	svc := &AuthService{defaultOrgID: "default-org"}

	got, reason := svc.trustedClaims("user-1", "default-org",
		mapClaims(t, `{"email": "user@xata.io", "organization": {"acme": {"displayName": ["Acme Inc"]}}}`))
	require.Empty(t, reason)
	require.Contains(t, got.Organizations, "default-org")
}

// TestTrustedClaimsRevalidation covers the revalidation that keeps a denial the
// token alone would produce from outliving the membership change behind it.
func TestTrustedClaimsRevalidation(t *testing.T) {
	for name, test := range map[string]struct {
		organizationID string
		billingStatus  string
		wantReason     fallbackReason
	}{
		"no target org":         {},
		"a known org":           {organizationID: "acme"},
		"an unknown org":        {organizationID: "brand-new", wantReason: reasonOrgNotInToken},
		"a disabled org":        {organizationID: "acme", billingStatus: "no_payment_method", wantReason: reasonOrgNotEnabled},
		"ignores another's org": {billingStatus: "no_payment_method"},
	} {
		t.Run(name, func(t *testing.T) {
			svc := &AuthService{}

			attributes := `"displayName": ["Acme Inc"]`
			if test.billingStatus != "" {
				attributes += fmt.Sprintf(`, "billingStatus": [%q]`, test.billingStatus)
			}
			claim := mapClaims(t, fmt.Sprintf(`{"email": "user@xata.io", "organization": {"acme": {%s}}}`, attributes))

			got, reason := svc.trustedClaims("user-1", test.organizationID, claim)
			require.Equal(t, test.wantReason, reason)
			if test.wantReason != reasonTrusted {
				require.Nil(t, got)
				return
			}
			require.Equal(t, "user-1", got.ID)
			require.Contains(t, got.Organizations, "acme")
		})
	}
}

func TestRecordClaimsSource(t *testing.T) {
	for name, test := range map[string]struct {
		raw  string
		want []attribute.KeyValue
	}{
		"trusted token": {
			raw:  `{"email": "user@xata.io", "organization": {"acme": {"displayName": ["Acme Inc"]}}}`,
			want: []attribute.KeyValue{attribute.Bool("xata.auth.claims_from_token", true)},
		},
		"no organization claim": {
			raw: `{"email": "user@xata.io"}`,
			want: []attribute.KeyValue{
				attribute.Bool("xata.auth.claims_from_token", false),
				attribute.String("xata.auth.claims_fallback", "no_org_claim"),
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			recorder := tracetest.NewSpanRecorder()
			_, span := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder)).Tracer("test").Start(t.Context(), "validate")

			svc := &AuthService{}
			_, reason := svc.trustedClaims("user-1", "", mapClaims(t, test.raw))
			recordClaimsSource(span, reason)
			span.End()

			ended := recorder.Ended()
			require.Len(t, ended, 1)
			require.ElementsMatch(t, test.want, ended[0].Attributes())
		})
	}
}
