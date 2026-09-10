package keycloak

import (
	"context"
	"net/http"
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
