package keycloak

import (
	"context"
	"encoding/json"
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
		"does not enforce": {got: got.RedirectsOnEmailMatch(), want: false},
		"binds the domain": {got: got.OrganizationDomain(), want: "acme.com"},
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
