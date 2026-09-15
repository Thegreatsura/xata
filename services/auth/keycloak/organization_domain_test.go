package keycloak

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Bodies captured from Keycloak 26.7.3. A mismatch here surfaces only as a 500
// in production.
func TestSetOrganizationDomainsClassifiesRejections(t *testing.T) {
	tests := map[string]struct {
		body string
		want error
	}{
		"claimed by another organization, 26.7.3 wording": {
			body: `{"errorMessage":"Domain acme.test is already linked to organization 123xyz in realm xata"}`,
			want: ErrDomainAlreadyClaimed{},
		},
		"claimed by another organization, alternative wording": {
			body: `{"errorMessage":"Domain acme.test is already linked to another organization in realm xata"}`,
			want: ErrDomainAlreadyClaimed{},
		},
		"malformed domain, 26.7.3 wording": {
			body: `{"errorMessage":"Invalid domain format: not a domain"}`,
			want: ErrInvalidDomain{},
		},
		"malformed domain, alternative wording": {
			body: `{"errorMessage":"The specified domain is invalid: not a domain"}`,
			want: ErrInvalidDomain{},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			srv := orgAdminTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(tt.body))
			})
			defer srv.Close()

			got := newTestRestKC(srv.URL).SetOrganizationDomains(context.Background(), "xata", "org-alias", []Domain{{Name: "acme.test"}})

			require.ErrorAs(t, got, &tt.want)
		})
	}
}

func TestSetOrganizationDomainsRoundTripsRouting(t *testing.T) {
	tests := map[string]struct {
		domain Domain
		want   string
	}{
		"leaves unset routing out": {
			domain: Domain{Name: "acme.test", Verified: true},
			want:   `{"name":"acme.test","verified":true}`,
		},
		"sends routing that is set": {
			domain: Domain{Name: "acme.test", Verified: true, IdentityProviderAlias: "sso-acme", AutoRedirect: true},
			want:   `{"name":"acme.test","verified":true,"identityProviderAlias":"sso-acme","autoRedirect":true}`,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var got json.RawMessage
			srv := orgAdminTestServer(t, func(w http.ResponseWriter, req *http.Request) {
				var body struct {
					Domains []json.RawMessage `json:"domains"`
				}
				if err := json.NewDecoder(req.Body).Decode(&body); err == nil && len(body.Domains) == 1 {
					got = body.Domains[0]
				}
				w.WriteHeader(http.StatusNoContent)
			})
			defer srv.Close()

			err := newTestRestKC(srv.URL).SetOrganizationDomains(context.Background(), "xata", "org-alias", []Domain{tt.domain})

			require.NoError(t, err)
			require.JSONEq(t, tt.want, string(got))
		})
	}
}

func TestOrganizationsForDomain(t *testing.T) {
	tests := map[string]struct {
		status  int
		body    string
		want    []string
		wantErr bool
	}{
		"returns the organization listing the domain": {
			status: http.StatusOK,
			body:   `[{"id":"internal-1","alias":"org-a","domains":[{"name":"acme.test","verified":true}]}]`,
			want:   []string{"org-a"},
		},
		"returns every holder left by concurrent writes": {
			status: http.StatusOK,
			body: `[{"id":"internal-1","alias":"org-a","domains":[{"name":"acme.test","verified":true}]},` +
				`{"id":"internal-2","alias":"org-b","domains":[{"name":"acme.test","verified":true}]}]`,
			want: []string{"org-a", "org-b"},
		},
		"ignores an organization matched only by its name": {
			status: http.StatusOK,
			body:   `[{"id":"internal-2","alias":"acme.test","name":"acme.test","domains":[]}]`,
		},
		"matches the domain regardless of case": {
			status: http.StatusOK,
			body:   `[{"id":"internal-1","alias":"org-a","domains":[{"name":"ACME.test","verified":false}]}]`,
			want:   []string{"org-a"},
		},
		"reports nobody holding it": {
			status: http.StatusOK,
			body:   `[]`,
		},
		"surfaces a failure rather than reading it as free": {
			status:  http.StatusInternalServerError,
			body:    `{}`,
			wantErr: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			srv := orgAdminTestServer(t, func(w http.ResponseWriter, req *http.Request) {
				if req.URL.Query().Get("search") != "acme.test" || req.URL.Query().Get("exact") != "true" {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			})
			defer srv.Close()

			got, err := newTestRestKC(srv.URL).OrganizationsForDomain(context.Background(), "xata", "acme.test")

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestSetOrganizationDomainsKeepsOtherFields(t *testing.T) {
	const stored = `{"id":"internal-1","name":"abc123","alias":"abc123","enabled":false,"description":"kept",` +
		`"redirectUrl":"https://app.xata.io/organizations/abc123",` +
		`"domains":[{"name":"abc123","verified":false},{"name":"acme.com","verified":true}],` +
		`"attributes":{"displayName":["Acme"],"billingStatus":["ok"]}}`

	tests := map[string]struct {
		domains []Domain
		want    string
	}{
		"replaces only the domains": {
			domains: []Domain{{Name: "acme.com", Verified: true}},
			want: `{"id":"internal-1","name":"abc123","alias":"abc123","enabled":false,"description":"kept",` +
				`"redirectUrl":"https://app.xata.io/organizations/abc123",` +
				`"domains":[{"name":"acme.com","verified":true}],` +
				`"attributes":{"displayName":["Acme"],"billingStatus":["ok"]}}`,
		},
		"sends an empty list when every domain goes": {
			domains: []Domain{},
			want: `{"id":"internal-1","name":"abc123","alias":"abc123","enabled":false,"description":"kept",` +
				`"redirectUrl":"https://app.xata.io/organizations/abc123","domains":[],` +
				`"attributes":{"displayName":["Acme"],"billingStatus":["ok"]}}`,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			bodies := make(chan string, 1)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				switch {
				case strings.HasSuffix(req.URL.Path, tokenEndpointSuffix):
					_, _ = w.Write([]byte(`{"access_token":"test-token","expires_in":300,"token_type":"Bearer"}`))
				case req.Method == http.MethodPut:
					body, _ := io.ReadAll(req.Body)
					bodies <- string(body)
					w.WriteHeader(http.StatusNoContent)
				default:
					_, _ = w.Write([]byte("[" + stored + "]"))
				}
			}))
			defer srv.Close()

			err := newTestRestKC(srv.URL).SetOrganizationDomains(context.Background(), "xata", "abc123", tt.domains)

			require.NoError(t, err)
			require.JSONEq(t, tt.want, <-bodies)
		})
	}
}
