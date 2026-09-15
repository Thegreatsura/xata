package roles

import (
	"context"
	"errors"
	"fmt"
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
		"a member with no reserved group holds the least role": {
			orgMembers: []string{testUserID},
			holders:    map[string][]string{},
			want:       map[string]Role{testUserID: Unassigned},
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

func TestMembersReportsEveryListedMember(t *testing.T) {
	const count = 250
	ids := make([]string, count)
	for i := range ids {
		ids[i] = fmt.Sprintf("user-%d", i)
	}

	s := newService(t, ids, map[string][]string{adminID: ids[:1]}, nil)

	got, err := s.Members(context.Background(), testOrgID)

	require.NoError(t, err)
	require.Len(t, got, count)
	require.Equal(t, Admin, got[ids[0]])
	require.Equal(t, Unassigned, got[ids[count-1]])
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

func TestIsAdmin(t *testing.T) {
	tests := map[string]struct {
		groups []keycloak.Group
		admins []string
		userID string
		want   bool
	}{
		"an Admin is": {
			groups: reservedGroups(),
			admins: []string{otherID, testUserID},
			userID: testUserID,
			want:   true,
		},
		"a member outside the Admin group is not": {
			groups: reservedGroups(),
			admins: []string{otherID},
			userID: testUserID,
		},
		"an organization without an Admin group has no Admin": {
			groups: reservedGroups()[1:],
			userID: testUserID,
		},
		"an empty user id is never an Admin and reads nothing": {
			userID: "",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			kc := keycloakMocks.NewKeyCloak(t)
			if tt.userID != "" {
				kc.EXPECT().ListGroups(mock.Anything, apitest.TestRealm, testOrgID).Return(tt.groups, nil).Once()
			}
			if tt.admins != nil {
				kc.EXPECT().ListGroupMembers(mock.Anything, apitest.TestRealm, testOrgID, adminID).
					Return(members(tt.admins...), nil).Once()
			}

			got, err := NewRoles(apitest.TestRealm, kc).IsAdmin(context.Background(), testOrgID, tt.userID)

			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestAddAdmins(t *testing.T) {
	tests := map[string]struct {
		expect  func(*keycloakMocks.KeyCloak)
		userIDs []string
		wantErr bool
	}{
		"a new organization gets every reserved group and the Admin group it just created": {
			expect: func(kc *keycloakMocks.KeyCloak) {
				kc.EXPECT().ListGroups(mock.Anything, apitest.TestRealm, testOrgID).Return(nil, nil).Once()
				for _, g := range reservedGroups() {
					kc.EXPECT().CreateGroup(mock.Anything, apitest.TestRealm, testOrgID, g.Name).Return(g, nil).Once()
				}
				kc.EXPECT().AddGroupMember(mock.Anything, apitest.TestRealm, testOrgID, adminID, testUserID).Return(nil).Once()
			},
			userIDs: []string{testUserID},
		},
		"existing groups are reused": {
			expect: func(kc *keycloakMocks.KeyCloak) {
				kc.EXPECT().ListGroups(mock.Anything, apitest.TestRealm, testOrgID).Return(reservedGroups(), nil).Once()
				kc.EXPECT().AddGroupMember(mock.Anything, apitest.TestRealm, testOrgID, adminID, testUserID).Return(nil).Once()
				kc.EXPECT().AddGroupMember(mock.Anything, apitest.TestRealm, testOrgID, adminID, otherID).Return(nil).Once()
			},
			userIDs: []string{testUserID, "", otherID},
		},
		"a failing user does not stop the rest": {
			expect: func(kc *keycloakMocks.KeyCloak) {
				kc.EXPECT().ListGroups(mock.Anything, apitest.TestRealm, testOrgID).Return(reservedGroups(), nil).Once()
				kc.EXPECT().AddGroupMember(mock.Anything, apitest.TestRealm, testOrgID, adminID, testUserID).
					Return(errors.New("keycloak unavailable")).Once()
				kc.EXPECT().AddGroupMember(mock.Anything, apitest.TestRealm, testOrgID, adminID, otherID).Return(nil).Once()
			},
			userIDs: []string{testUserID, otherID},
			wantErr: true,
		},
		"a group that cannot be created grants nothing": {
			expect: func(kc *keycloakMocks.KeyCloak) {
				kc.EXPECT().ListGroups(mock.Anything, apitest.TestRealm, testOrgID).Return(nil, nil).Once()
				kc.EXPECT().CreateGroup(mock.Anything, apitest.TestRealm, testOrgID, "Admin").
					Return(keycloak.Group{}, errors.New("keycloak unavailable")).Once()
			},
			userIDs: []string{testUserID},
			wantErr: true,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			kc := keycloakMocks.NewKeyCloak(t)
			tt.expect(kc)

			got := NewRoles(apitest.TestRealm, kc).AddAdmins(context.Background(), testOrgID, tt.userIDs...)

			if tt.wantErr {
				require.Error(t, got)
				return
			}
			require.NoError(t, got)
		})
	}
}

func TestAddAdminsConcurrentCreation(t *testing.T) {
	tests := map[string]struct {
		relisted []keycloak.Group
		wantErr  bool
	}{
		"the group another request created is used": {
			relisted: reservedGroups(),
		},
		"a conflict with the group still missing fails": {
			relisted: reservedGroups()[:2],
			wantErr:  true,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			kc := keycloakMocks.NewKeyCloak(t)
			kc.EXPECT().ListGroups(mock.Anything, apitest.TestRealm, testOrgID).
				Return(reservedGroups()[:2], nil).Once()
			kc.EXPECT().CreateGroup(mock.Anything, apitest.TestRealm, testOrgID, "Viewer").
				Return(keycloak.Group{}, keycloak.ErrGroupAlreadyExists{Name: "Viewer"}).Once()
			kc.EXPECT().ListGroups(mock.Anything, apitest.TestRealm, testOrgID).
				Return(tt.relisted, nil).Once()

			got := NewRoles(apitest.TestRealm, kc).AddAdmins(context.Background(), testOrgID)

			if tt.wantErr {
				require.ErrorAs(t, got, &keycloak.ErrGroupAlreadyExists{})
				return
			}
			require.NoError(t, got)
		})
	}
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

func TestAudit(t *testing.T) {
	tests := map[string]struct {
		orgMembers []string
		groups     []keycloak.Group
		holders    map[string][]string
		want       Audit
	}{
		"every member holding a role is healthy": {
			orgMembers: []string{testUserID, otherID},
			groups:     reservedGroups(),
			holders:    map[string][]string{adminID: {testUserID}, editorID: {otherID}},
			want:       Audit{Members: 2, Admins: 1},
		},
		"missing reserved groups are reported": {
			orgMembers: []string{testUserID},
			groups:     []keycloak.Group{{ID: adminID, Name: "Admin"}},
			holders:    map[string][]string{adminID: {testUserID}},
			want:       Audit{Members: 1, Admins: 1, MissingRoles: []Role{Editor, Viewer}},
		},
		"a role backed by groups differing only in case is a duplicate": {
			orgMembers: []string{testUserID},
			groups:     append(reservedGroups(), keycloak.Group{ID: "group-admin-2", Name: "ADMIN"}),
			holders:    map[string][]string{adminID: {testUserID}, "group-admin-2": {testUserID}},
			want:       Audit{Members: 1, Admins: 1, DuplicateRoles: []Role{Admin}},
		},
		"members in no reserved group are listed in organization order": {
			orgMembers: []string{testUserID, otherID, "user-3"},
			groups:     reservedGroups(),
			holders:    map[string][]string{adminID: {testUserID}},
			want:       Audit{Members: 3, Admins: 1, Unassigned: []string{otherID, "user-3"}},
		},
		"a holder who has left the organization is not counted": {
			orgMembers: []string{testUserID},
			groups:     reservedGroups(),
			holders:    map[string][]string{adminID: {"who-left"}},
			want:       Audit{Members: 1, Unassigned: []string{testUserID}},
		},
		"an organization with no members": {
			groups: reservedGroups(),
			want:   Audit{},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			kc := keycloakMocks.NewKeyCloak(t)
			kc.EXPECT().ListMembers(mock.Anything, apitest.TestRealm, testOrgID).Return(members(tt.orgMembers...), nil).Once()
			kc.EXPECT().ListGroups(mock.Anything, apitest.TestRealm, testOrgID).Return(tt.groups, nil).Once()
			for _, g := range tt.groups {
				kc.EXPECT().ListGroupMembers(mock.Anything, apitest.TestRealm, testOrgID, g.ID).
					Return(members(tt.holders[g.ID]...), nil).Once()
			}

			got, err := NewRoles(apitest.TestRealm, kc).Audit(context.Background(), testOrgID)

			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}
