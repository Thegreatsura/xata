package roles

import (
	"context"
	"testing"

	"xata/internal/apitest"
	"xata/services/auth/keycloak"
	keycloakMocks "xata/services/auth/keycloak/mocks"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const (
	testOrgID  = "org-123"
	adminID    = "group-admin"
	editorID   = "group-editor"
	viewerID   = "group-viewer"
	testUserID = "user-1"
	otherID    = "user-2"
)

func reservedGroups() []keycloak.Group {
	return []keycloak.Group{
		{ID: adminID, Name: "Admin"},
		{ID: editorID, Name: "Editor"},
		{ID: viewerID, Name: "Viewer"},
	}
}

func members(ids ...string) []keycloak.OrganizationMember {
	result := make([]keycloak.OrganizationMember, len(ids))
	for i, id := range ids {
		result[i] = keycloak.OrganizationMember{ID: id}
	}
	return result
}

// holders maps a reserved group id to the members Keycloak reports for it.
func newService(t *testing.T, orgMembers []string, holders map[string][]string, extra func(*keycloakMocks.KeyCloak)) Roles {
	t.Helper()
	kc := keycloakMocks.NewKeyCloak(t)
	kc.EXPECT().ListMembers(mock.Anything, apitest.TestRealm, testOrgID).
		Return(members(orgMembers...), nil).Maybe()
	kc.EXPECT().ListGroups(mock.Anything, apitest.TestRealm, testOrgID).
		Return(reservedGroups(), nil).Maybe()
	for _, g := range reservedGroups() {
		kc.EXPECT().ListGroupMembers(mock.Anything, apitest.TestRealm, testOrgID, g.ID).
			Return(members(holders[g.ID]...), nil).Maybe()
	}
	if extra != nil {
		extra(kc)
	}
	return NewRoles(apitest.TestRealm, kc)
}

func TestMembers(t *testing.T) {
	tests := map[string]struct {
		orgMembers []string
		holders    map[string][]string
		want       map[string]Role
	}{
		"a member with no reserved group holds the default role": {
			orgMembers: []string{testUserID},
			holders:    map[string][]string{},
			want:       map[string]Role{testUserID: Default},
		},
		"each reserved group reports its role": {
			orgMembers: []string{testUserID, otherID},
			holders:    map[string][]string{adminID: {testUserID}, viewerID: {otherID}},
			want:       map[string]Role{testUserID: Admin, otherID: Viewer},
		},
		"a holder who has left the organization is not reported": {
			orgMembers: []string{testUserID},
			holders:    map[string][]string{adminID: {testUserID, "who-left"}},
			want:       map[string]Role{testUserID: Admin},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := newService(t, tt.orgMembers, tt.holders, nil).Members(context.Background(), testOrgID)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestSetMember(t *testing.T) {
	tests := map[string]struct {
		orgMembers []string
		holders    map[string][]string
		target     string
		role       Role
		caller     string
		expect     func(*keycloakMocks.KeyCloak)
		wantErr    error
	}{
		"an Admin may change a member's role": {
			orgMembers: []string{testUserID, otherID},
			holders:    map[string][]string{adminID: {testUserID}},
			target:     otherID, role: Viewer, caller: testUserID,
			expect: func(kc *keycloakMocks.KeyCloak) {
				kc.EXPECT().AddGroupMember(mock.Anything, apitest.TestRealm, testOrgID, viewerID, otherID).Return(nil).Once()
				kc.EXPECT().RemoveGroupMember(mock.Anything, apitest.TestRealm, testOrgID, adminID, otherID).Return(nil).Once()
				kc.EXPECT().RemoveGroupMember(mock.Anything, apitest.TestRealm, testOrgID, editorID, otherID).Return(nil).Once()
			},
		},
		"a non-Admin may not": {
			orgMembers: []string{testUserID, otherID},
			holders:    map[string][]string{adminID: {otherID}},
			target:     otherID, role: Viewer, caller: testUserID,
			wantErr: ErrNotAdmin{},
		},
		"an organization API key carries no identity and is never an Admin": {
			orgMembers: []string{testUserID, otherID},
			holders:    map[string][]string{adminID: {testUserID}},
			target:     otherID, role: Viewer, caller: "",
			wantErr: ErrNotAdmin{},
		},
		"the last Admin cannot be demoted": {
			orgMembers: []string{testUserID, otherID},
			holders:    map[string][]string{adminID: {testUserID}},
			target:     testUserID, role: Editor, caller: testUserID,
			wantErr: ErrLastAdmin{},
		},
		"an Admin may be demoted while another remains": {
			orgMembers: []string{testUserID, otherID},
			holders:    map[string][]string{adminID: {testUserID, otherID}},
			target:     otherID, role: Editor, caller: testUserID,
			expect: func(kc *keycloakMocks.KeyCloak) {
				kc.EXPECT().AddGroupMember(mock.Anything, apitest.TestRealm, testOrgID, editorID, otherID).Return(nil).Once()
				kc.EXPECT().RemoveGroupMember(mock.Anything, apitest.TestRealm, testOrgID, adminID, otherID).Return(nil).Once()
				kc.EXPECT().RemoveGroupMember(mock.Anything, apitest.TestRealm, testOrgID, viewerID, otherID).Return(nil).Once()
			},
		},
		"a user outside the organization cannot be given a role": {
			orgMembers: []string{testUserID},
			holders:    map[string][]string{adminID: {testUserID}},
			target:     "stranger", role: Viewer, caller: testUserID,
			wantErr: ErrUserNotOrganizationMember{UserID: "stranger"},
		},
		"an unknown role is refused": {
			orgMembers: []string{testUserID, otherID},
			holders:    map[string][]string{adminID: {testUserID}},
			target:     otherID, role: "superuser", caller: testUserID,
			wantErr: ErrUnknownRole{Role: "superuser"},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			s := newService(t, tt.orgMembers, tt.holders, tt.expect)

			got := s.SetMember(context.Background(), testOrgID, tt.target, tt.role, tt.caller)

			if tt.wantErr != nil {
				require.ErrorIs(t, got, tt.wantErr)
				return
			}
			require.NoError(t, got)
		})
	}
}

func TestEnsureRoles(t *testing.T) {
	t.Run("creates the reserved groups an organization is missing", func(t *testing.T) {
		kc := keycloakMocks.NewKeyCloak(t)
		kc.EXPECT().ListGroups(mock.Anything, apitest.TestRealm, testOrgID).
			Return([]keycloak.Group{{ID: adminID, Name: "Admin"}}, nil).Maybe()
		kc.EXPECT().CreateGroup(mock.Anything, apitest.TestRealm, testOrgID, "Editor").
			Return(keycloak.Group{ID: editorID, Name: "Editor"}, nil).Once()
		kc.EXPECT().CreateGroup(mock.Anything, apitest.TestRealm, testOrgID, "Viewer").
			Return(keycloak.Group{ID: viewerID, Name: "Viewer"}, nil).Once()
		kc.EXPECT().ListMembers(mock.Anything, apitest.TestRealm, testOrgID).
			Return(members(testUserID), nil).Maybe()
		kc.EXPECT().ListGroupMembers(mock.Anything, apitest.TestRealm, testOrgID, adminID).
			Return(members(testUserID), nil).Maybe()

		require.NoError(t, NewRoles(apitest.TestRealm, kc).EnsureRoles(context.Background(), testOrgID, nil))
	})

	t.Run("seeds every member as Admin when an organization has none", func(t *testing.T) {
		kc := keycloakMocks.NewKeyCloak(t)
		kc.EXPECT().ListGroups(mock.Anything, apitest.TestRealm, testOrgID).Return(reservedGroups(), nil).Maybe()
		kc.EXPECT().ListMembers(mock.Anything, apitest.TestRealm, testOrgID).
			Return(members(testUserID, otherID), nil).Maybe()
		for _, g := range reservedGroups() {
			kc.EXPECT().ListGroupMembers(mock.Anything, apitest.TestRealm, testOrgID, g.ID).
				Return(nil, nil).Maybe()
		}
		kc.EXPECT().AddGroupMember(mock.Anything, apitest.TestRealm, testOrgID, adminID, testUserID).Return(nil).Once()
		kc.EXPECT().AddGroupMember(mock.Anything, apitest.TestRealm, testOrgID, adminID, otherID).Return(nil).Once()

		seed := func(context.Context) ([]string, error) { return []string{testUserID, otherID}, nil }
		require.NoError(t, NewRoles(apitest.TestRealm, kc).EnsureRoles(context.Background(), testOrgID, seed))
	})
}

func TestCheckOrganizationMemberRemovable(t *testing.T) {
	tests := map[string]struct {
		holders map[string][]string
		userID  string
		wantErr bool
	}{
		"the last Admin cannot leave":            {map[string][]string{adminID: {testUserID}}, testUserID, true},
		"an Admin may leave while another stays": {map[string][]string{adminID: {testUserID, otherID}}, otherID, false},
		"a non-Admin may always leave":           {map[string][]string{adminID: {testUserID}}, otherID, false},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			s := newService(t, []string{testUserID, otherID}, tt.holders, nil)

			got := s.CheckOrganizationMemberRemovable(context.Background(), testOrgID, tt.userID)

			if tt.wantErr {
				require.ErrorIs(t, got, ErrLastAdmin{})
				return
			}
			require.NoError(t, got)
		})
	}
}

func TestRemoveMemberFromAllRoles(t *testing.T) {
	kc := keycloakMocks.NewKeyCloak(t)
	kc.EXPECT().ListGroups(mock.Anything, apitest.TestRealm, testOrgID).Return(reservedGroups(), nil).Once()
	for _, g := range reservedGroups() {
		kc.EXPECT().RemoveGroupMember(mock.Anything, apitest.TestRealm, testOrgID, g.ID, testUserID).Return(nil).Once()
	}

	require.NoError(t, NewRoles(apitest.TestRealm, kc).RemoveMemberFromAllRoles(context.Background(), testOrgID, testUserID))
}
