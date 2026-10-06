package keycloak

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewIdentityProvider(t *testing.T) {
	got := NewIdentityProvider(IdentityProviderSpec{
		OrganizationID: "acme",
		Domain:         "acme.com",
		ProviderID:     "oidc",
		Issuer:         "https://idp.acme.com",
	})

	tests := map[string]struct {
		got  any
		want any
	}{
		"trusts the email, which xata-require-domain-sso keeps safe by refusing an address outside the bound domain": {
			got: got.TrustEmail, want: true,
		},
		"runs the xata first broker login, where xata-require-domain-sso holds it to its domain": {
			got: got.FirstBrokerLoginFlowAlias, want: FirstBrokerLoginFlow,
		},
		"runs the xata post broker login, which holds a returning member the same way": {
			got: got.PostBrokerLoginFlowAlias, want: PostBrokerLoginFlow,
		},
		"is never a button on the login page; members arrive by the domain redirect, or by a kc_idp_hint link that ignores this": {
			got: got.HideOnLogin, want: true,
		},
		"leaves routing to the organization domain":    {got: got.Config["kc.org.domain"], want: ""},
		"names itself for the organization and domain": {got: got.Alias, want: "sso-acme-acme-com"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.got)
		})
	}
}

func TestUpsertIdentityProviderBindsTheBrokerFlows(t *testing.T) {
	var got IdentityProvider
	srv := orgAdminTestServer(t, func(w http.ResponseWriter, req *http.Request) {
		require.NoError(t, json.NewDecoder(req.Body).Decode(&got))
		w.WriteHeader(http.StatusCreated)
	})
	defer srv.Close()

	err := newTestRestKC(srv.URL).UpsertIdentityProvider(context.Background(), "xata", IdentityProvider{Alias: "sso-acme-acme-com"})

	require.NoError(t, err)
	require.Equal(t, FirstBrokerLoginFlow, got.FirstBrokerLoginFlowAlias)
	require.Equal(t, PostBrokerLoginFlow, got.PostBrokerLoginFlowAlias)
}

func TestBindBrokerFlows(t *testing.T) {
	tests := map[string]struct {
		idp       IdentityProvider
		wantFirst string
		wantPost  string
	}{
		"binds both when unset, so a provider read back from Keycloak cannot be written out blank": {
			idp:       IdentityProvider{Alias: "sso-acme-acme-com"},
			wantFirst: FirstBrokerLoginFlow,
			wantPost:  PostBrokerLoginFlow,
		},
		"binds the one that is missing without touching the other": {
			idp:       IdentityProvider{Alias: "sso-acme-acme-com", FirstBrokerLoginFlowAlias: "custom"},
			wantFirst: "custom",
			wantPost:  PostBrokerLoginFlow,
		},
		"leaves a deliberate choice alone": {
			idp: IdentityProvider{
				Alias:                     "sso-acme-acme-com",
				FirstBrokerLoginFlowAlias: "custom first",
				PostBrokerLoginFlowAlias:  "custom post",
			},
			wantFirst: "custom first",
			wantPost:  "custom post",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := tt.idp.bindBrokerFlows()

			require.Equal(t, tt.wantFirst, got.FirstBrokerLoginFlowAlias)
			require.Equal(t, tt.wantPost, got.PostBrokerLoginFlowAlias)
		})
	}
}

func TestLinkIdentityProviderToOrganization(t *testing.T) {
	const linkPath = "/admin/realms/xata/organizations/internal-1/identity-providers"

	tests := map[string]struct {
		linkStatus int
		wantErr    bool
	}{
		"links a new provider":                 {linkStatus: http.StatusNoContent},
		"treats an existing link as done":      {linkStatus: http.StatusConflict},
		"fails when Keycloak refuses the link": {linkStatus: http.StatusBadRequest, wantErr: true},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var gotBody string
			srv := orgAdminTestServer(t, func(w http.ResponseWriter, req *http.Request) {
				// Any other request would override the UNMANAGED membership the POST creates the link with.
				if req.Method != http.MethodPost || req.URL.Path != linkPath {
					t.Errorf("unexpected request %s %s", req.Method, req.URL.Path)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				body, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				gotBody = string(body)
				w.WriteHeader(tt.linkStatus)
			})
			defer srv.Close()

			err := newTestRestKC(srv.URL).LinkIdentityProviderToOrganization(context.Background(), "xata", "org-alias", "sso-acme-acme-com")

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.JSONEq(t, `"sso-acme-acme-com"`, gotBody)
		})
	}
}
