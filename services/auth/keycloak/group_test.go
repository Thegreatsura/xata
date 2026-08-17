package keycloak

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGroupOperations(t *testing.T) {
	tests := map[string]struct {
		admin http.HandlerFunc
		run   func(t *testing.T, kc KeyCloak)
	}{
		"create returns the id from the Location header": {
			admin: func(w http.ResponseWriter, req *http.Request) {
				require.Equal(t, http.MethodPost, req.Method)
				require.True(t, strings.HasSuffix(req.URL.Path, "/organizations/internal-1/groups"))
				w.Header().Set("Location", "https://kc.example/admin/realms/test-realm/organizations/internal-1/groups/new-group-id")
				w.WriteHeader(http.StatusCreated)
			},
			run: func(t *testing.T, kc KeyCloak) {
				group, err := kc.CreateGroup(context.Background(), "test-realm", "org-alias", "engineering")
				require.NoError(t, err)
				assert.Equal(t, "new-group-id", group.ID)
				assert.Equal(t, "engineering", group.Name)
			},
		},
		"create maps 409 to ErrGroupAlreadyExists": {
			admin: func(w http.ResponseWriter, req *http.Request) {
				w.WriteHeader(http.StatusConflict)
			},
			run: func(t *testing.T, kc KeyCloak) {
				_, err := kc.CreateGroup(context.Background(), "test-realm", "org-alias", "engineering")
				require.ErrorAs(t, err, &ErrGroupAlreadyExists{})
			},
		},
		"list returns the organization groups": {
			admin: func(w http.ResponseWriter, req *http.Request) {
				require.Equal(t, http.MethodGet, req.Method)
				require.True(t, strings.HasSuffix(req.URL.Path, "/organizations/internal-1/groups"))
				_, _ = w.Write([]byte(`[{"id":"owner-id","name":"Owner","path":"/Owner"},{"id":"g1","name":"eng"}]`))
			},
			run: func(t *testing.T, kc KeyCloak) {
				groups, err := kc.ListGroups(context.Background(), "test-realm", "org-alias")
				require.NoError(t, err)
				require.Len(t, groups, 2)
				assert.Equal(t, "Owner", groups[0].Name)
				assert.Equal(t, "g1", groups[1].ID)
			},
		},
		"get maps 404 to ErrGroupNotFound": {
			admin: func(w http.ResponseWriter, req *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			},
			run: func(t *testing.T, kc KeyCloak) {
				_, err := kc.GetGroup(context.Background(), "test-realm", "org-alias", "missing")
				require.ErrorAs(t, err, &ErrGroupNotFound{})
			},
		},
		"add member uses PUT on the member path": {
			admin: func(w http.ResponseWriter, req *http.Request) {
				assert.Equal(t, http.MethodPut, req.Method)
				assert.Equal(t, "/admin/realms/test-realm/organizations/internal-1/groups/g1/members/u1", req.URL.Path)
				w.WriteHeader(http.StatusNoContent)
			},
			run: func(t *testing.T, kc KeyCloak) {
				require.NoError(t, kc.AddGroupMember(context.Background(), "test-realm", "org-alias", "g1", "u1"))
			},
		},
		"remove member is idempotent on 404": {
			admin: func(w http.ResponseWriter, req *http.Request) {
				require.Equal(t, http.MethodDelete, req.Method)
				w.WriteHeader(http.StatusNotFound)
			},
			run: func(t *testing.T, kc KeyCloak) {
				require.NoError(t, kc.RemoveGroupMember(context.Background(), "test-realm", "org-alias", "g1", "u1"))
			},
		},
		"list members maps user representations": {
			admin: func(w http.ResponseWriter, req *http.Request) {
				require.True(t, strings.HasSuffix(req.URL.Path, "/organizations/internal-1/groups/g1/members"))
				_, _ = w.Write([]byte(`[{"id":"u1","email":"a@b.com","firstName":"Ada","lastName":"Byron"}]`))
			},
			run: func(t *testing.T, kc KeyCloak) {
				members, err := kc.ListGroupMembers(context.Background(), "test-realm", "org-alias", "g1")
				require.NoError(t, err)
				require.Len(t, members, 1)
				assert.Equal(t, "u1", members[0].ID)
				assert.Equal(t, "Ada Byron", members[0].Name)
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			srv := orgAdminTestServer(t, tt.admin)
			defer srv.Close()
			tt.run(t, newTestRestKC(srv.URL))
		})
	}
}
