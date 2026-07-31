package keycloak

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"xata/services/auth/config"

	"github.com/Nerzal/gocloak/v13"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestRestKC builds a restKC pointed at a test server for both the token
// endpoint (via the gocloak client) and the Admin REST API (via KeycloakURL).
func newTestRestKC(baseURL string) *restKC {
	return &restKC{
		client: gocloak.NewClient(baseURL),
		authConfig: config.AuthConfig{
			KeycloakURL:           baseURL,
			KeycloakAdminPassword: "test-password",
		},
	}
}

const tokenEndpointSuffix = "/protocol/openid-connect/token"

func TestGetTokenCachesAdminLogin(t *testing.T) {
	var loginCount, userCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch {
		case strings.HasSuffix(req.URL.Path, tokenEndpointSuffix):
			loginCount.Add(1)
			// gocloak auto-unmarshals the token via resty, which requires a JSON content type.
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"test-token","expires_in":300,"token_type":"Bearer"}`))
		case strings.Contains(req.URL.Path, "/admin/realms/"):
			userCount.Add(1)
			_, _ = w.Write([]byte(`{"id":"user-1","username":"alice"}`))
		default:
			t.Errorf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
	}))
	defer srv.Close()

	r := newTestRestKC(srv.URL)

	for range 3 {
		_, err := r.GetUserRepresentation(context.Background(), "test-realm", "user-1")
		require.NoError(t, err)
	}

	assert.Equal(t, int32(1), loginCount.Load(), "admin login should be cached across calls")
	assert.Equal(t, int32(3), userCount.Load())
}

func TestGetTokenDedupesConcurrentLogins(t *testing.T) {
	var loginCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch {
		case strings.HasSuffix(req.URL.Path, tokenEndpointSuffix):
			loginCount.Add(1)
			// gocloak auto-unmarshals the token via resty, which requires a JSON content type.
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"test-token","expires_in":300,"token_type":"Bearer"}`))
		case strings.Contains(req.URL.Path, "/admin/realms/"):
			_, _ = w.Write([]byte(`{"id":"user-1","username":"alice"}`))
		default:
			t.Errorf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
	}))
	defer srv.Close()

	r := newTestRestKC(srv.URL)

	const goroutines = 20
	errCh := make(chan error, goroutines)
	var wg sync.WaitGroup
	for range goroutines {
		wg.Go(func() {
			_, err := r.GetUserRepresentation(context.Background(), "test-realm", "user-1")
			errCh <- err
		})
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		require.NoError(t, err)
	}
	assert.Equal(t, int32(1), loginCount.Load(), "concurrent callers should share a single admin login")
}

func TestMakeAuthenticatedRequestRetriesOn401(t *testing.T) {
	var loginCount, userCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch {
		case strings.HasSuffix(req.URL.Path, tokenEndpointSuffix):
			loginCount.Add(1)
			// gocloak auto-unmarshals the token via resty, which requires a JSON content type.
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"test-token","expires_in":300,"token_type":"Bearer"}`))
		case strings.Contains(req.URL.Path, "/admin/realms/"):
			// Reject the first request as if the cached token was revoked.
			if userCount.Add(1) == 1 {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"id":"user-1","username":"alice"}`))
		default:
			t.Errorf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
	}))
	defer srv.Close()

	r := newTestRestKC(srv.URL)

	user, err := r.GetUserRepresentation(context.Background(), "test-realm", "user-1")
	require.NoError(t, err)
	assert.Equal(t, "user-1", user.ID)
	assert.Equal(t, int32(2), loginCount.Load(), "a 401 should invalidate the cache and force a fresh login")
	assert.Equal(t, int32(2), userCount.Load(), "the request should be retried once after re-login")
}

func TestFirstAttr(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		attrs   map[string][]string
		key     string
		wantVal string
		wantOK  bool
	}{
		{
			name:    "nil map",
			attrs:   nil,
			key:     "anything",
			wantVal: "",
			wantOK:  false,
		},
		{
			name:    "key missing",
			attrs:   map[string][]string{"other": {"val"}},
			key:     "missing",
			wantVal: "",
			wantOK:  false,
		},
		{
			name:    "key present but empty slice",
			attrs:   map[string][]string{"k": {}},
			key:     "k",
			wantVal: "",
			wantOK:  false,
		},
		{
			name:    "key present with whitespace-only value",
			attrs:   map[string][]string{"k": {"   "}},
			key:     "k",
			wantVal: "",
			wantOK:  false,
		},
		{
			name:    "key present with valid value",
			attrs:   map[string][]string{"k": {"hello"}},
			key:     "k",
			wantVal: "hello",
			wantOK:  true,
		},
		{
			name:    "trims leading and trailing whitespace",
			attrs:   map[string][]string{"k": {"  trimmed  "}},
			key:     "k",
			wantVal: "trimmed",
			wantOK:  true,
		},
		{
			name:    "returns first element only",
			attrs:   map[string][]string{"k": {"first", "second"}},
			key:     "k",
			wantVal: "first",
			wantOK:  true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := FirstAttr(tc.attrs, tc.key)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.wantVal, got)
		})
	}
}

func TestUserAttributesDeserialization(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		jsonBody   string
		wantAttrs  map[string][]string
		wantFields User
	}{
		{
			name:     "no attributes field",
			jsonBody: `{"id":"u1","username":"alice","email":"alice@example.com","emailVerified":true}`,
			wantFields: User{
				ID:            "u1",
				Username:      "alice",
				Email:         "alice@example.com",
				EmailVerified: true,
			},
		},
		{
			name:     "empty attributes",
			jsonBody: `{"id":"u2","username":"bob","attributes":{}}`,
			wantFields: User{
				ID:       "u2",
				Username: "bob",
			},
			wantAttrs: map[string][]string{},
		},
		{
			name: "marketplace attributes present",
			jsonBody: `{
				"id":"u3",
				"username":"carol",
				"email":"carol@example.com",
				"emailVerified":true,
				"attributes":{
					"marketplace":["aws"],
					"marketplaceRegisteredAt":["2026-03-23T00:00:00Z"],
					"awsAccountId":["123456789"],
					"awsCustomerId":["cust-abc"],
					"awsProductId":["prod-xyz"]
				}
			}`,
			wantFields: User{
				ID:            "u3",
				Username:      "carol",
				Email:         "carol@example.com",
				EmailVerified: true,
			},
			wantAttrs: map[string][]string{
				"marketplace":             {"aws"},
				"marketplaceRegisteredAt": {"2026-03-23T00:00:00Z"},
				"awsAccountId":            {"123456789"},
				"awsCustomerId":           {"cust-abc"},
				"awsProductId":            {"prod-xyz"},
			},
		},
		{
			name: "partial attributes",
			jsonBody: `{
				"id":"u4",
				"username":"dave",
				"attributes":{
					"marketplace":["aws"],
					"awsCustomerId":["cust-only"]
				}
			}`,
			wantFields: User{
				ID:       "u4",
				Username: "dave",
			},
			wantAttrs: map[string][]string{
				"marketplace":   {"aws"},
				"awsCustomerId": {"cust-only"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got User
			err := json.Unmarshal([]byte(tc.jsonBody), &got)
			require.NoError(t, err)

			assert.Equal(t, tc.wantFields.ID, got.ID)
			assert.Equal(t, tc.wantFields.Username, got.Username)
			assert.Equal(t, tc.wantFields.Email, got.Email)
			assert.Equal(t, tc.wantFields.EmailVerified, got.EmailVerified)

			if tc.wantAttrs != nil {
				assert.Equal(t, tc.wantAttrs, got.Attributes)
			} else {
				assert.Nil(t, got.Attributes)
			}
		})
	}
}

func TestExtractStatus(t *testing.T) {
	ts := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC) // RFC3339-friendly

	cases := []struct {
		name   string
		attrs  map[string][]string
		expect OrganizationStatus
	}{
		{
			name:  "when nil attributes the organization is enabled and lastUpdated is epoch",
			attrs: nil,
			expect: OrganizationStatus{
				DisabledByAdmin: false,
				BillingStatus:   OrganizationBillingStatusOK,
				AdminReason:     nil,
				BillingReason:   nil,
				LastUpdated:     time.Unix(0, 0).UTC(),
				UsageTier:       OrganizationUsageTierT1,
			},
		},
		{
			name: "admin disabled true (case-insensitive)",
			attrs: map[string][]string{
				OrganizationDisabledByAdminKey: {"TrUe"},
			},
			expect: OrganizationStatus{
				DisabledByAdmin: true,
				BillingStatus:   OrganizationBillingStatusOK,
				LastUpdated:     time.Unix(0, 0).UTC(),
				UsageTier:       OrganizationUsageTierT1,
			},
		},
		{
			name: "admin disabled false, billing ok → enabled",
			attrs: map[string][]string{
				OrganizationDisabledByAdminKey: {"false"},
				OrganizationBillingStatusKey:   {string(OrganizationBillingStatusOK)},
			},
			expect: OrganizationStatus{
				DisabledByAdmin: false,
				BillingStatus:   OrganizationBillingStatusOK,
				LastUpdated:     time.Unix(0, 0).UTC(),
				UsageTier:       OrganizationUsageTierT1,
			},
		},
		{
			name: "billing overdue disables org",
			attrs: map[string][]string{
				OrganizationDisabledByAdminKey: {"false"},
				OrganizationBillingStatusKey:   {string(OrganizationBillingStatusInvoiceOverdue)},
			},
			expect: OrganizationStatus{
				DisabledByAdmin: false,
				BillingStatus:   OrganizationBillingStatusInvoiceOverdue,
				LastUpdated:     time.Unix(0, 0).UTC(),
				UsageTier:       OrganizationUsageTierT1,
			},
		},
		{
			name: "billing no payment method disables org",
			attrs: map[string][]string{
				OrganizationDisabledByAdminKey: {"false"},
				OrganizationBillingStatusKey:   {string(OrganizationBillingStatusNoPaymentMethod)},
			},
			expect: OrganizationStatus{
				DisabledByAdmin: false,
				BillingStatus:   OrganizationBillingStatusNoPaymentMethod,
				LastUpdated:     time.Unix(0, 0).UTC(),
				UsageTier:       OrganizationUsageTierT1,
			},
		},
		{
			name: "billing unrecognized disables org",
			attrs: map[string][]string{
				OrganizationDisabledByAdminKey: {"false"},
				OrganizationBillingStatusKey:   {"some-new-status"},
			},
			expect: OrganizationStatus{
				DisabledByAdmin: false,
				BillingStatus:   OrganizationBillingStatusUnknown,
				LastUpdated:     time.Unix(0, 0).UTC(),
				UsageTier:       OrganizationUsageTierT1,
			},
		},
		{
			name: "reasons and valid lastUpdated",
			attrs: map[string][]string{
				OrganizationDisabledByAdminKey: {"true"},
				OrganizationBillingStatusKey:   {string(OrganizationBillingStatusInvoiceOverdue)},
				OrganizationAdminReasonKey:     {"policy violation"},
				OrganizationBillingReasonKey:   {"card declined"},
				OrganizationLastUpdatedKey:     {ts.Format(time.RFC3339)},
			},
			expect: OrganizationStatus{
				DisabledByAdmin: true,
				BillingStatus:   OrganizationBillingStatusInvoiceOverdue,
				AdminReason:     new("policy violation"),
				BillingReason:   new("card declined"),
				LastUpdated:     ts,
				UsageTier:       OrganizationUsageTierT1,
			},
		},
		{
			name: "invalid lastUpdated falls back to epoch",
			attrs: map[string][]string{
				OrganizationDisabledByAdminKey: {"false"},
				OrganizationLastUpdatedKey:     {"not-a-timestamp"},
			},
			expect: OrganizationStatus{
				DisabledByAdmin: false,
				BillingStatus:   OrganizationBillingStatusOK,
				LastUpdated:     time.Unix(0, 0).UTC(),
				UsageTier:       OrganizationUsageTierT1,
			},
		},
		{
			name: "trims values via firstAttr",
			attrs: map[string][]string{
				OrganizationDisabledByAdminKey: {" false  "},
				OrganizationBillingStatusKey:   {"   ok "},
				OrganizationAdminReasonKey:     {"  "}, // empty after trim → nil
			},
			expect: OrganizationStatus{
				DisabledByAdmin: false,
				BillingStatus:   OrganizationBillingStatusOK,
				AdminReason:     nil,
				LastUpdated:     time.Unix(0, 0).UTC(),
				UsageTier:       OrganizationUsageTierT1,
			},
		},
		{
			name: "usage tier t2",
			attrs: map[string][]string{
				OrganizationDisabledByAdminKey: {"false"},
				OrganizationBillingStatusKey:   {string(OrganizationBillingStatusOK)},
				OrganizationUsageTierKey:       {string(OrganizationUsageTierT2)},
			},
			expect: OrganizationStatus{
				DisabledByAdmin: false,
				BillingStatus:   OrganizationBillingStatusOK,
				LastUpdated:     time.Unix(0, 0).UTC(),
				UsageTier:       OrganizationUsageTierT2,
			},
		},
		{
			name: "deletion_requested disables org",
			attrs: map[string][]string{
				OrganizationDisabledByAdminKey: {"false"},
				OrganizationBillingStatusKey:   {string(OrganizationBillingStatusDeletionRequested)},
			},
			expect: OrganizationStatus{
				DisabledByAdmin: false,
				BillingStatus:   OrganizationBillingStatusDeletionRequested,
				LastUpdated:     time.Unix(0, 0).UTC(),
				UsageTier:       OrganizationUsageTierT1,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractStatus(tc.attrs)
			assert.Equal(t, tc.expect, got)
		})
	}
}

func TestCreateOrganizationRequiresUsageTier(t *testing.T) {
	t.Parallel()

	r := &restKC{}

	_, err := r.CreateOrganization(context.Background(), "xata", OrganizationCreate{Name: "Acme"})

	require.EqualError(t, err, "usage tier is required")
}

func TestBuildCreateOrganizationPayload_UsageTier(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		params OrganizationCreate
		want   OrganizationUsageTier
	}{
		"normal organization uses t1": {
			params: OrganizationCreate{Name: "Acme", UsageTier: OrganizationUsageTierT1},
			want:   OrganizationUsageTierT1,
		},
		"aws marketplace organization uses t2": {
			params: OrganizationCreate{
				Name:      "Acme",
				UsageTier: OrganizationUsageTierT2,
				Marketplace: AWSMarketplace{
					CustomerID: "cust-1",
					ProductID:  "prod-1",
					AccountID:  "acct-1",
				},
			},
			want: OrganizationUsageTierT2,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			r := &restKC{}

			org := r.buildCreateOrganizationPayload("org_123", tc.params)

			require.NotNil(t, org.Attributes)
			require.Equal(t, string(tc.want), org.Attributes[OrganizationUsageTierKey][0])
		})
	}
}

func TestConvertToOrganization_AWSMarketplace(t *testing.T) {
	t.Parallel()

	r := &restKC{}
	org := r.buildCreateOrganizationPayload("org_123", OrganizationCreate{
		Name:          "Acme",
		UsageTier:     OrganizationUsageTierT2,
		BillingStatus: OrganizationBillingStatusOK,
		BillingReason: "Organization enabled with marketplace billing",
		Marketplace: AWSMarketplace{
			CustomerID: "cust-1",
			ProductID:  "prod-1",
			AccountID:  "acct-1",
		},
	})

	converted := r.convertToOrganization(org)

	require.NotNil(t, converted.Marketplace)
	require.NotNil(t, converted.AWSMarketplace)
	assert.Equal(t, OrganizationMarketplaceProviderAWS, *converted.Marketplace)
	assert.Equal(t, "cust-1", converted.AWSMarketplace.CustomerID)
	assert.Equal(t, "prod-1", converted.AWSMarketplace.ProductID)
	assert.Equal(t, "acct-1", converted.AWSMarketplace.AccountID)
	assert.Equal(t, OrganizationBillingStatusOK, converted.Status.BillingStatus)
	assert.Equal(t, OrganizationStateEnabled, converted.Status.EffectiveState())
}

func TestConvertToOrganization_VercelMarketplace(t *testing.T) {
	t.Parallel()

	r := &restKC{}
	org := r.buildCreateOrganizationPayload("org_456", OrganizationCreate{
		Name:          "Acme",
		UsageTier:     OrganizationUsageTierT2,
		BillingStatus: OrganizationBillingStatusOK,
		BillingReason: "Organization enabled with marketplace billing",
		Marketplace: VercelMarketplace{
			InstallationID: "icfg_1",
			AccountID:      "acc_1",
			Email:          "ops@acme.example",
		},
	})

	// The attribute keys are the on-the-wire contract with Keycloak; assert
	// them explicitly so a rename can't silently break round-tripping.
	assert.Equal(t, VercelMarketplaceProviderName, org.Attributes[OrganizationMarketplaceKey][0])
	assert.Equal(t, "icfg_1", org.Attributes[OrganizationVercelInstallationIDKey][0])
	assert.Equal(t, "acc_1", org.Attributes[OrganizationVercelAccountIDKey][0])
	assert.Equal(t, "ops@acme.example", org.Attributes[OrganizationVercelAccountEmailKey][0])

	converted := r.convertToOrganization(org)

	require.NotNil(t, converted.Marketplace)
	require.NotNil(t, converted.VercelMarketplace)
	assert.Equal(t, OrganizationMarketplaceProviderVercel, *converted.Marketplace)
	assert.Equal(t, "icfg_1", converted.VercelMarketplace.InstallationID)
	assert.Equal(t, "acc_1", converted.VercelMarketplace.AccountID)
	assert.Equal(t, "ops@acme.example", converted.VercelMarketplace.Email)
	assert.Equal(t, OrganizationBillingStatusOK, converted.Status.BillingStatus)
	assert.Equal(t, OrganizationStateEnabled, converted.Status.EffectiveState())
}

func TestBuildCreateOrganizationPayload_BillingStatus(t *testing.T) {
	t.Parallel()

	fixedID := "org_123"

	t.Run("no_payment_method", func(t *testing.T) {
		r := &restKC{}

		org := r.buildCreateOrganizationPayload(fixedID, OrganizationCreate{
			Name:          "Acme",
			UsageTier:     OrganizationUsageTierT1,
			BillingStatus: OrganizationBillingStatusNoPaymentMethod,
			BillingReason: "Organization created, no payment method set",
		})

		require.NotNil(t, org.Attributes)
		assert.Equal(t, string(OrganizationBillingStatusNoPaymentMethod), org.Attributes[OrganizationBillingStatusKey][0])
		assert.Equal(t, "Organization created, no payment method set", org.Attributes[OrganizationBillingReasonKey][0])

		// sanity checks
		assert.Equal(t, "Acme", org.Attributes["displayName"][0])
		assert.Equal(t, "false", org.Attributes[OrganizationDisabledByAdminKey][0])
		assert.Equal(t, fixedID, org.Name)
		assert.Equal(t, fixedID, org.Alias)

		lu := org.Attributes[OrganizationLastUpdatedKey][0]
		_, err := time.Parse(time.RFC3339, lu)
		assert.NoError(t, err)

		converted := r.convertToOrganization(org)
		assert.Equal(t, fixedID, converted.ID)
		assert.Equal(t, "Acme", converted.Name)
		assert.Equal(t, OrganizationBillingStatusNoPaymentMethod, converted.Status.BillingStatus)
		assert.Equal(t, OrganizationStateDisabled, converted.Status.EffectiveState())
		assert.Equal(t, OrganizationUsageTierT1, converted.Status.UsageTier)
		assert.False(t, converted.Status.LastUpdated.IsZero())
		require.NotNil(t, converted.Status.CreatedAt)
		assert.False(t, converted.Status.CreatedAt.IsZero())
	})

	t.Run("ok", func(t *testing.T) {
		r := &restKC{}

		org := r.buildCreateOrganizationPayload(fixedID, OrganizationCreate{
			Name:          "Acme",
			UsageTier:     OrganizationUsageTierT1,
			BillingStatus: OrganizationBillingStatusOK,
			BillingReason: "Organization enabled by default since billing is not required",
		})

		require.NotNil(t, org.Attributes)
		assert.Equal(t, string(OrganizationBillingStatusOK), org.Attributes[OrganizationBillingStatusKey][0])
		assert.Equal(t, "Organization enabled by default since billing is not required", org.Attributes[OrganizationBillingReasonKey][0])

		lu := org.Attributes[OrganizationLastUpdatedKey][0]
		_, err := time.Parse(time.RFC3339, lu)
		assert.NoError(t, err)

		converted := r.convertToOrganization(org)
		assert.Equal(t, fixedID, converted.ID)
		assert.Equal(t, "Acme", converted.Name)
		assert.Equal(t, OrganizationBillingStatusOK, converted.Status.BillingStatus)
		assert.Equal(t, OrganizationStateEnabled, converted.Status.EffectiveState())
		assert.Equal(t, OrganizationUsageTierT1, converted.Status.UsageTier)
		assert.False(t, converted.Status.LastUpdated.IsZero())
		require.NotNil(t, converted.Status.CreatedAt)
		assert.False(t, converted.Status.CreatedAt.IsZero())
	})
}

func TestGetIdentityProviderToken(t *testing.T) {
	tests := map[string]struct {
		status      int
		body        string
		wantToken   string
		wantErr     error
		wantErrText string
	}{
		"json response": {
			status:    http.StatusOK,
			body:      `{"access_token":"gh-user-token","token_type":"bearer","scope":""}`,
			wantToken: "gh-user-token",
		},
		"form encoded response": {
			status:    http.StatusOK,
			body:      `access_token=gh-user-token&scope=&token_type=bearer`,
			wantToken: "gh-user-token",
		},
		"missing access token": {
			status:      http.StatusOK,
			body:        `{"token_type":"bearer"}`,
			wantErrText: "no access_token in response",
		},
		"identity not linked": {
			status:  http.StatusBadRequest,
			body:    `{"errorMessage":"Identity Provider [github] does not exist or is not linked."}`,
			wantErr: ErrIdentityProviderNotLinked{Provider: "github"},
		},
		"no stored token": {
			status:  http.StatusNotFound,
			wantErr: ErrIdentityProviderNotLinked{Provider: "github"},
		},
		"missing broker read-token role": {
			status:  http.StatusForbidden,
			wantErr: ErrIdentityProviderTokenForbidden{Provider: "github"},
		},
		"server error": {
			status:      http.StatusInternalServerError,
			wantErrText: "status 500",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var gotPath, gotAuth string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				gotPath = req.URL.Path
				gotAuth = req.Header.Get("Authorization")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			r := newTestRestKC(srv.URL)

			got, err := r.GetIdentityProviderToken(context.Background(), "test-realm", "github", "user-token")

			assert.Equal(t, "/realms/test-realm/broker/github/token", gotPath)
			assert.Equal(t, "Bearer user-token", gotAuth, "request must use the user's token, not the admin token")

			switch {
			case tc.wantErr != nil:
				require.ErrorIs(t, err, tc.wantErr)
			case tc.wantErrText != "":
				require.ErrorContains(t, err, tc.wantErrText)
			default:
				require.NoError(t, err)
				assert.Equal(t, tc.wantToken, got)
			}
		})
	}
}
