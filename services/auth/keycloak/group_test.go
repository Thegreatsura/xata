package keycloak

import (
	"context"
	"io"
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
		"add member is idempotent on 409": {
			admin: func(w http.ResponseWriter, req *http.Request) {
				require.Equal(t, http.MethodPut, req.Method)
				w.WriteHeader(http.StatusConflict)
			},
			run: func(t *testing.T, kc KeyCloak) {
				require.NoError(t, kc.AddGroupMember(context.Background(), "test-realm", "org-alias", "g1", "u1"))
			},
		},
		"add member fails on a server error": {
			admin: func(w http.ResponseWriter, req *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			run: func(t *testing.T, kc KeyCloak) {
				require.Error(t, kc.AddGroupMember(context.Background(), "test-realm", "org-alias", "g1", "u1"))
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
		"remove member is idempotent when the user is not in the group": {
			admin: func(w http.ResponseWriter, req *http.Request) {
				require.Equal(t, http.MethodDelete, req.Method)
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"errorMessage":"User not a member"}`))
			},
			run: func(t *testing.T, kc KeyCloak) {
				require.NoError(t, kc.RemoveGroupMember(context.Background(), "test-realm", "org-alias", "g1", "u1"))
			},
		},
		"remove member surfaces any other bad request": {
			admin: func(w http.ResponseWriter, req *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"errorMessage":"Invalid request"}`))
			},
			run: func(t *testing.T, kc KeyCloak) {
				require.Error(t, kc.RemoveGroupMember(context.Background(), "test-realm", "org-alias", "g1", "u1"))
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
		"list asks for attributes": {
			admin: func(w http.ResponseWriter, req *http.Request) {
				require.Equal(t, "false", req.URL.Query().Get("briefRepresentation"))
				_, _ = w.Write([]byte(`[{"id":"g1","name":"Viewer","attributes":{"invitedRoles":["a@b.com=admin"]}}]`))
			},
			run: func(t *testing.T, kc KeyCloak) {
				groups, err := kc.ListGroups(context.Background(), "test-realm", "org-alias")
				require.NoError(t, err)
				require.Len(t, groups, 1)
				assert.Equal(t, []string{"a@b.com=admin"}, groups[0].Attributes["invitedRoles"])
			},
		},
		"update attribute keeps the other attributes and the description": {
			admin: groupAttributeServer(t, `{"id":"g1","name":"Viewer","description":"Read-only","attributes":{"keep":["x"],"invitedRoles":["a@b.com=admin"]}}`,
				`{"name":"Viewer","description":"Read-only","attributes":{"invitedRoles":["a@b.com=admin","c@d.com=editor"],"keep":["x"]}}`),
			run: func(t *testing.T, kc KeyCloak) {
				err := kc.UpdateGroupAttribute(context.Background(), "test-realm", "org-alias", "g1", "invitedRoles", func(values []string) []string {
					assert.Equal(t, []string{"a@b.com=admin"}, values)
					return append(values, "c@d.com=editor")
				})
				require.NoError(t, err)
			},
		},
		"update attribute to no values removes it": {
			admin: groupAttributeServer(t, `{"id":"g1","name":"Viewer","description":"Read-only","attributes":{"keep":["x"],"invitedRoles":["a@b.com=admin"]}}`,
				`{"name":"Viewer","description":"Read-only","attributes":{"keep":["x"]}}`),
			run: func(t *testing.T, kc KeyCloak) {
				err := kc.UpdateGroupAttribute(context.Background(), "test-realm", "org-alias", "g1", "invitedRoles", func([]string) []string { return nil })
				require.NoError(t, err)
			},
		},
		"update attribute that changes nothing skips the write": {
			admin: groupAttributeServer(t, `{"id":"g1","name":"Viewer","attributes":{"invitedRoles":["a@b.com=admin"]}}`, ""),
			run: func(t *testing.T, kc KeyCloak) {
				err := kc.UpdateGroupAttribute(context.Background(), "test-realm", "org-alias", "g1", "invitedRoles", func(values []string) []string {
					return values
				})
				require.NoError(t, err)
			},
		},
		"update attribute with nothing to remove skips the write": {
			admin: groupAttributeServer(t, `{"id":"g1","name":"Viewer"}`, ""),
			run: func(t *testing.T, kc KeyCloak) {
				err := kc.UpdateGroupAttribute(context.Background(), "test-realm", "org-alias", "g1", "invitedRoles", func([]string) []string { return nil })
				require.NoError(t, err)
			},
		},
		"update attribute maps 404 to ErrGroupNotFound": {
			admin: func(w http.ResponseWriter, req *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			},
			run: func(t *testing.T, kc KeyCloak) {
				err := kc.UpdateGroupAttribute(context.Background(), "test-realm", "org-alias", "missing", "invitedRoles", func(values []string) []string {
					t.Error("update ran without a group")
					return values
				})
				require.ErrorAs(t, err, &ErrGroupNotFound{})
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

func groupAttributeServer(t *testing.T, current, want string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		assert.Equal(t, "/admin/realms/test-realm/organizations/internal-1/groups/g1", req.URL.Path)
		switch req.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(current))
		case http.MethodPut:
			if want == "" {
				t.Error("unexpected write")
			}
			got, err := io.ReadAll(req.Body)
			assert.NoError(t, err)
			assert.JSONEq(t, want, string(got))
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected method %s", req.Method)
		}
	}
}
