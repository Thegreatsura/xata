package keycloak

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOrganizationIDCache(t *testing.T) {
	tests := map[string]struct {
		cache        bool
		listFirst    bool
		wantSearches int64
		wantRequests int64
	}{
		"without the cache every call searches for the organization": {
			wantSearches: 5,
			wantRequests: 10,
		},
		"the cache searches once": {
			cache:        true,
			wantSearches: 1,
			wantRequests: 6,
		},
		"listing organizations fills the cache": {
			cache:        true,
			listFirst:    true,
			wantSearches: 0,
			wantRequests: 5,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var searches, requests atomic.Int64
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				path := req.URL.Path
				if strings.HasSuffix(path, tokenEndpointSuffix) {
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"access_token":"test-token","expires_in":300,"token_type":"Bearer"}`))
					return
				}
				requests.Add(1)
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasSuffix(path, "/organizations") && req.URL.Query().Get("q") != "":
					searches.Add(1)
					_, _ = w.Write([]byte(`[{"id":"internal-1","alias":"org-alias"}]`))
				case strings.HasSuffix(path, "/organizations"):
					_, _ = w.Write([]byte(`[{"id":"internal-1","alias":"org-alias"}]`))
				case strings.Contains(path, "/groups/") && strings.HasSuffix(path, "/members"):
					_, _ = w.Write([]byte(`[]`))
				case strings.HasSuffix(path, "/members"):
					_, _ = w.Write([]byte(`[{"id":"user-1"}]`))
				case strings.HasSuffix(path, "/groups"):
					_, _ = w.Write([]byte(`[{"id":"g-admin","name":"Admin"},{"id":"g-editor","name":"Editor"},{"id":"g-viewer","name":"Viewer"}]`))
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			t.Cleanup(srv.Close)

			kc := newTestRestKC(srv.URL)
			if tt.cache {
				WithOrganizationIDCache()(kc)
			}
			ctx := context.Background()
			if tt.listFirst {
				orgs, err := kc.ListAllOrganizations(ctx, "xata")
				require.NoError(t, err)
				require.Len(t, orgs, 1)
				requests.Store(0)
			}

			_, err := kc.ListMembers(ctx, "xata", "org-alias")
			require.NoError(t, err)
			groups, err := kc.ListGroups(ctx, "xata", "org-alias")
			require.NoError(t, err)
			for _, g := range groups {
				_, err := kc.ListGroupMembers(ctx, "xata", "org-alias", g.ID)
				require.NoError(t, err)
			}

			require.Equal(t, tt.wantSearches, searches.Load())
			require.Equal(t, tt.wantRequests, requests.Load())
		})
	}
}
