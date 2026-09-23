package roles

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"xata/internal/apitest"
	"xata/services/auth/keycloak"
	keycloakMocks "xata/services/auth/keycloak/mocks"

	"github.com/rs/zerolog"
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
func newService(t *testing.T, orgMembers []string, holders map[string][]string, extra func(*keycloakMocks.KeyCloak), opts ...Option) Roles {
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
	return NewRoles(apitest.TestRealm, kc, opts...)
}

func viewerGrantable(enabled bool) Option {
	return WithViewer(func(context.Context) bool { return enabled })
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
		"a member in several reserved groups holds the most privileged": {
			orgMembers: []string{testUserID, otherID},
			holders:    map[string][]string{adminID: {testUserID}, editorID: {testUserID, otherID}, viewerID: {otherID}},
			want:       map[string]Role{testUserID: Admin, otherID: Editor},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := newService(t, tt.orgMembers, tt.holders, nil).Members(context.Background(), testOrgID, Unassigned)
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

	got, err := s.Members(context.Background(), testOrgID, Unassigned)

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
		caller     Caller
		viewer     bool
		expect     func(*keycloakMocks.KeyCloak)
		wantErr    error
	}{
		"an Admin may change a member's role": {
			orgMembers: []string{testUserID, otherID},
			holders:    map[string][]string{adminID: {testUserID}},
			target:     otherID, role: Editor, caller: Caller{UserID: testUserID},
			expect: func(kc *keycloakMocks.KeyCloak) {
				kc.EXPECT().AddGroupMember(mock.Anything, apitest.TestRealm, testOrgID, editorID, otherID).Return(nil).Once()
				kc.EXPECT().RemoveGroupMember(mock.Anything, apitest.TestRealm, testOrgID, adminID, otherID).Return(nil).Once()
				kc.EXPECT().RemoveGroupMember(mock.Anything, apitest.TestRealm, testOrgID, viewerID, otherID).Return(nil).Once()
			},
		},
		"Viewer is refused without the viewer role": {
			orgMembers: []string{testUserID, otherID},
			holders:    map[string][]string{adminID: {testUserID}},
			target:     otherID, role: Viewer, caller: Caller{UserID: testUserID},
			wantErr: ErrRoleNotGrantable{Role: "viewer"},
		},
		"Viewer is granted with the viewer role": {
			orgMembers: []string{testUserID, otherID},
			holders:    map[string][]string{adminID: {testUserID}},
			target:     otherID, role: Viewer, caller: Caller{UserID: testUserID}, viewer: true,
			expect: func(kc *keycloakMocks.KeyCloak) {
				kc.EXPECT().AddGroupMember(mock.Anything, apitest.TestRealm, testOrgID, viewerID, otherID).Return(nil).Once()
				kc.EXPECT().RemoveGroupMember(mock.Anything, apitest.TestRealm, testOrgID, adminID, otherID).Return(nil).Once()
				kc.EXPECT().RemoveGroupMember(mock.Anything, apitest.TestRealm, testOrgID, editorID, otherID).Return(nil).Once()
			},
		},
		"a non-Admin may not": {
			orgMembers: []string{testUserID, otherID},
			holders:    map[string][]string{adminID: {otherID}},
			target:     otherID, role: Viewer, caller: Caller{UserID: testUserID},
			wantErr: ErrNotAdmin{},
		},
		"a caller with no identity is not an Admin": {
			orgMembers: []string{testUserID, otherID},
			holders:    map[string][]string{adminID: {testUserID}},
			target:     otherID, role: Viewer, caller: Caller{},
			wantErr: ErrNotAdmin{},
		},
		"an organization key acts with the access an Admin has": {
			orgMembers: []string{testUserID, otherID},
			holders:    map[string][]string{adminID: {testUserID, otherID}},
			target:     otherID, role: Editor, caller: Caller{OrganizationKey: true},
			expect: func(kc *keycloakMocks.KeyCloak) {
				kc.EXPECT().AddGroupMember(mock.Anything, apitest.TestRealm, testOrgID, editorID, otherID).Return(nil).Once()
				kc.EXPECT().RemoveGroupMember(mock.Anything, apitest.TestRealm, testOrgID, adminID, otherID).Return(nil).Once()
				kc.EXPECT().RemoveGroupMember(mock.Anything, apitest.TestRealm, testOrgID, viewerID, otherID).Return(nil).Once()
			},
		},
		"an organization key cannot demote the last Admin": {
			orgMembers: []string{testUserID, otherID},
			holders:    map[string][]string{adminID: {testUserID}},
			target:     testUserID, role: Editor, caller: Caller{OrganizationKey: true},
			wantErr: ErrLastAdmin{},
		},
		"the last Admin cannot be demoted": {
			orgMembers: []string{testUserID, otherID},
			holders:    map[string][]string{adminID: {testUserID}},
			target:     testUserID, role: Editor, caller: Caller{UserID: testUserID},
			wantErr: ErrLastAdmin{},
		},
		"an Admin may be demoted while another remains": {
			orgMembers: []string{testUserID, otherID},
			holders:    map[string][]string{adminID: {testUserID, otherID}},
			target:     otherID, role: Editor, caller: Caller{UserID: testUserID},
			expect: func(kc *keycloakMocks.KeyCloak) {
				kc.EXPECT().AddGroupMember(mock.Anything, apitest.TestRealm, testOrgID, editorID, otherID).Return(nil).Once()
				kc.EXPECT().RemoveGroupMember(mock.Anything, apitest.TestRealm, testOrgID, adminID, otherID).Return(nil).Once()
				kc.EXPECT().RemoveGroupMember(mock.Anything, apitest.TestRealm, testOrgID, viewerID, otherID).Return(nil).Once()
			},
		},
		"a user outside the organization cannot be given a role": {
			orgMembers: []string{testUserID},
			holders:    map[string][]string{adminID: {testUserID}},
			target:     "stranger", role: Viewer, caller: Caller{UserID: testUserID},
			wantErr: ErrUserNotOrganizationMember{UserID: "stranger"},
		},
		"an unknown role is refused": {
			orgMembers: []string{testUserID, otherID},
			holders:    map[string][]string{adminID: {testUserID}},
			target:     otherID, role: "superuser", caller: Caller{UserID: testUserID},
			wantErr: ErrUnknownRole{Role: "superuser"},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			s := newService(t, tt.orgMembers, tt.holders, tt.expect, viewerGrantable(tt.viewer))

			got := s.SetMember(context.Background(), testOrgID, tt.target, tt.role, tt.caller)

			if tt.wantErr != nil {
				require.ErrorIs(t, got, tt.wantErr)
				return
			}
			require.NoError(t, got)
		})
	}
}

// TestSetMemberKeepsAnAdmin demotes the only two Admins by each other at once. Neither request writes
// until both have read, so both pass the last-Admin check before either changes anything.
func TestSetMemberKeepsAnAdmin(t *testing.T) {
	errKeycloak := errors.New("keycloak unavailable")
	tests := map[string]struct {
		restoreErr  error
		wantAdmin   bool
		wantErr     error
		wantAlerted bool
	}{
		"one demotion is refused and an Admin remains": {
			wantAdmin: true,
			wantErr:   ErrLastAdmin{},
		},
		"a failed restore is logged for alerting": {
			restoreErr:  errKeycloak,
			wantErr:     errKeycloak,
			wantAlerted: true,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var (
				mu       sync.Mutex
				holders  = map[string][]string{adminID: {testUserID, otherID}}
				arrived  = 0
				bothRead = make(chan struct{})
				logs     bytes.Buffer
			)
			kc := keycloakMocks.NewKeyCloak(t)
			kc.EXPECT().ListMembers(mock.Anything, apitest.TestRealm, testOrgID).Return(members(testUserID, otherID), nil)
			kc.EXPECT().ListGroups(mock.Anything, apitest.TestRealm, testOrgID).Return(reservedGroups(), nil)
			kc.EXPECT().ListGroupMembers(mock.Anything, apitest.TestRealm, testOrgID, mock.Anything).
				RunAndReturn(func(_ context.Context, _, _, group string) ([]keycloak.OrganizationMember, error) {
					mu.Lock()
					defer mu.Unlock()
					return members(holders[group]...), nil
				})
			kc.EXPECT().AddGroupMember(mock.Anything, apitest.TestRealm, testOrgID, mock.Anything, mock.Anything).
				RunAndReturn(func(_ context.Context, _, _, group, user string) error {
					mu.Lock()
					if arrived++; arrived == 2 {
						close(bothRead)
					}
					mu.Unlock()
					<-bothRead

					mu.Lock()
					defer mu.Unlock()
					if group == adminID && tt.restoreErr != nil {
						return tt.restoreErr
					}
					if !slices.Contains(holders[group], user) {
						holders[group] = append(holders[group], user)
					}
					return nil
				})
			kc.EXPECT().RemoveGroupMember(mock.Anything, apitest.TestRealm, testOrgID, mock.Anything, mock.Anything).
				RunAndReturn(func(_ context.Context, _, _, group, user string) error {
					mu.Lock()
					defer mu.Unlock()
					holders[group] = slices.DeleteFunc(holders[group], func(held string) bool { return held == user })
					return nil
				}).Maybe()
			s := NewRoles(apitest.TestRealm, kc)
			ctx := zerolog.New(zerolog.SyncWriter(&logs)).WithContext(context.Background())

			errs := make([]error, 2)
			var wg sync.WaitGroup
			wg.Go(func() { errs[0] = s.SetMember(ctx, testOrgID, otherID, Editor, Caller{UserID: testUserID}) })
			wg.Go(func() { errs[1] = s.SetMember(ctx, testOrgID, testUserID, Editor, Caller{UserID: otherID}) })
			wg.Wait()

			require.Equal(t, tt.wantAdmin, len(holders[adminID]) > 0)
			got := errors.Join(errs...)
			require.ErrorIs(t, got, tt.wantErr)
			require.Equal(t, tt.wantAlerted, strings.Contains(logs.String(), `"roles_last_admin_restore_failed":true`))
		})
	}
}

func TestIsAdmin(t *testing.T) {
	errKeycloak := errors.New("keycloak unavailable")
	tests := map[string]struct {
		groups     []keycloak.Group
		holders    map[string][]string
		userID     string
		unassigned Role
		wantReads  []string
		want       bool
		wantErr    error
	}{
		"an Admin is": {
			groups:     reservedGroups(),
			holders:    map[string][]string{adminID: {otherID, testUserID}},
			userID:     testUserID,
			unassigned: Unassigned,
			wantReads:  []string{adminID},
			want:       true,
		},
		"a member outside the Admin group is not": {
			groups:     reservedGroups(),
			holders:    map[string][]string{adminID: {otherID}},
			userID:     testUserID,
			unassigned: Unassigned,
			wantReads:  []string{adminID},
		},
		"an organization without an Admin group has no Admin": {
			groups:     reservedGroups()[1:],
			userID:     testUserID,
			unassigned: Unassigned,
		},
		"an empty user id is never an Admin and reads nothing": {
			userID:     "",
			unassigned: Unassigned,
		},
		"an empty user id is never an Admin even when unassigned counts as Admin": {
			userID:     "",
			unassigned: Admin,
		},
		"counted as Admin, an Admin is found reading only the Admin group": {
			groups:     reservedGroups(),
			holders:    map[string][]string{adminID: {testUserID}, viewerID: {testUserID}},
			userID:     testUserID,
			unassigned: Admin,
			wantReads:  []string{adminID},
			want:       true,
		},
		"counted as Admin, an explicit Editor is refused after the Admin group": {
			groups:     reservedGroups(),
			holders:    map[string][]string{adminID: {otherID}, editorID: {testUserID}},
			userID:     testUserID,
			unassigned: Admin,
			wantReads:  []string{adminID, editorID},
		},
		"counted as Admin, an explicit Viewer is refused after every other reserved group": {
			groups:     reservedGroups(),
			holders:    map[string][]string{viewerID: {testUserID}},
			userID:     testUserID,
			unassigned: Admin,
			wantReads:  []string{adminID, editorID, viewerID},
		},
		"counted as Admin, a member only in a group outside the reserved ones passes": {
			groups:     append(reservedGroups(), keycloak.Group{ID: "group-eng", Name: "eng"}),
			holders:    map[string][]string{"group-eng": {testUserID}},
			userID:     testUserID,
			unassigned: Admin,
			wantReads:  []string{adminID, editorID, viewerID},
			want:       true,
		},
		"counted as Admin, a failed read is returned": {
			groups:     reservedGroups(),
			userID:     testUserID,
			unassigned: Admin,
			wantReads:  []string{adminID},
			wantErr:    errKeycloak,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			kc := keycloakMocks.NewKeyCloak(t)
			if tt.userID != "" {
				kc.EXPECT().ListGroups(mock.Anything, apitest.TestRealm, testOrgID).Return(tt.groups, nil).Once()
			}
			for _, id := range tt.wantReads {
				kc.EXPECT().ListGroupMembers(mock.Anything, apitest.TestRealm, testOrgID, id).
					Return(members(tt.holders[id]...), tt.wantErr).Once()
			}

			got, err := NewRoles(apitest.TestRealm, kc).IsAdmin(context.Background(), testOrgID, tt.userID, tt.unassigned)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
			require.Len(t, kc.Calls, len(tt.wantReads)+boolToInt(tt.userID != ""))
		})
	}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
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
		holders    map[string][]string
		userID     string
		unassigned Role
		wantErr    bool
	}{
		"the last Admin cannot leave": {
			holders: map[string][]string{adminID: {testUserID}}, userID: testUserID, unassigned: Unassigned, wantErr: true,
		},
		"an Admin may leave while another stays": {
			holders: map[string][]string{adminID: {testUserID, otherID}}, userID: otherID, unassigned: Unassigned,
		},
		"a non-Admin may always leave": {
			holders: map[string][]string{adminID: {testUserID}}, userID: otherID, unassigned: Unassigned,
		},
		"a member in no reserved group counted as Admin is the last Admin": {
			holders: map[string][]string{viewerID: {otherID}}, userID: testUserID, unassigned: Admin, wantErr: true,
		},
		"an Admin may leave while a member counted as Admin stays": {
			holders: map[string][]string{adminID: {otherID}}, userID: otherID, unassigned: Admin,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			s := newService(t, []string{testUserID, otherID}, tt.holders, nil)

			got := s.CheckOrganizationMemberRemovable(context.Background(), testOrgID, tt.userID, tt.unassigned)

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

func expectInvitationsUpdate(t *testing.T, kc *keycloakMocks.KeyCloak, current, want []string) {
	kc.EXPECT().UpdateGroupAttribute(mock.Anything, apitest.TestRealm, testOrgID, viewerID, invitedRolesAttribute, mock.Anything).
		RunAndReturn(func(_ context.Context, _, _, _, _ string, update func([]string) []string) error {
			require.Equal(t, want, update(current))
			return nil
		}).Once()
}

func TestOffered(t *testing.T) {
	tests := map[string]struct {
		viewer bool
		want   []Role
	}{
		"without the viewer role": {want: []Role{Admin, Editor}},
		"with the viewer role":    {viewer: true, want: []Role{Admin, Editor, Viewer}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var got []Role
			for _, d := range Offered(tt.viewer) {
				got = append(got, d.Role)
			}
			require.Equal(t, tt.want, got)
		})
	}
}

func TestReported(t *testing.T) {
	tests := map[string]struct {
		role   Role
		viewer bool
		want   Role
	}{
		"a Viewer is reported as Editor without the viewer role": {role: Viewer, want: Editor},
		"a Viewer is reported as Viewer with the viewer role":    {role: Viewer, viewer: true, want: Viewer},
		"other roles are reported as held":                       {role: Admin, want: Admin},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.role.Reported(tt.viewer))
		})
	}
}

func TestSetInvitation(t *testing.T) {
	errKeycloak := errors.New("keycloak unavailable")
	tests := map[string]struct {
		groups  []keycloak.Group
		email   string
		role    Role
		viewer  bool
		current []string
		want    []string
		expect  func(*keycloakMocks.KeyCloak)
		wantErr error
	}{
		"records one entry on the Viewer group": {
			groups: reservedGroups(),
			email:  " Invitee@Example.com ",
			role:   Editor,
			want:   []string{"invitee@example.com=editor"},
		},
		"replaces the entry already recorded for the address": {
			groups:  reservedGroups(),
			email:   "invitee@example.com",
			role:    Editor,
			current: []string{"other@example.com=admin", "INVITEE@example.com=admin"},
			want:    []string{"other@example.com=admin", "invitee@example.com=editor"},
		},
		"Viewer is refused without the viewer role": {
			email:   "invitee@example.com",
			role:    Viewer,
			wantErr: ErrRoleNotGrantable{Role: "viewer"},
		},
		"Viewer is recorded with the viewer role": {
			groups: reservedGroups(),
			email:  "invitee@example.com",
			role:   Viewer,
			viewer: true,
			want:   []string{"invitee@example.com=viewer"},
		},
		"creates the missing reserved groups first": {
			email: "invitee@example.com",
			role:  Admin,
			want:  []string{"invitee@example.com=admin"},
			expect: func(kc *keycloakMocks.KeyCloak) {
				for _, g := range reservedGroups() {
					kc.EXPECT().CreateGroup(mock.Anything, apitest.TestRealm, testOrgID, g.Name).Return(g, nil).Once()
				}
			},
		},
		"a failed write is returned": {
			groups: reservedGroups(),
			email:  "invitee@example.com",
			role:   Editor,
			expect: func(kc *keycloakMocks.KeyCloak) {
				kc.EXPECT().UpdateGroupAttribute(mock.Anything, apitest.TestRealm, testOrgID, viewerID, invitedRolesAttribute, mock.Anything).
					Return(errKeycloak).Once()
			},
			wantErr: errKeycloak,
		},
		"an unknown role is refused": {
			email:   "invitee@example.com",
			role:    "owner",
			wantErr: ErrUnknownRole{Role: "owner"},
		},
		"an address too long to record is refused": {
			email:   strings.Repeat("a", 243) + "@example.com",
			role:    Editor,
			wantErr: ErrEmailTooLong{},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			kc := keycloakMocks.NewKeyCloak(t)
			kc.EXPECT().ListGroups(mock.Anything, apitest.TestRealm, testOrgID).Return(tt.groups, nil).Maybe()
			if tt.want != nil {
				expectInvitationsUpdate(t, kc, tt.current, tt.want)
			}
			if tt.expect != nil {
				tt.expect(kc)
			}

			got := NewRoles(apitest.TestRealm, kc, viewerGrantable(tt.viewer)).SetInvitation(context.Background(), testOrgID, tt.email, tt.role)

			if tt.wantErr != nil {
				require.ErrorIs(t, got, tt.wantErr)
				return
			}
			require.NoError(t, got)
		})
	}
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

func TestClearInvitation(t *testing.T) {
	tests := map[string]struct {
		current []string
		want    []string
	}{
		"removes only the address's entry": {
			current: []string{"invitee@example.com=editor", "other@example.com=admin"},
			want:    []string{"other@example.com=admin"},
		},
		"the last entry leaves none": {
			current: []string{"invitee@example.com=editor"},
			want:    []string{},
		},
		"an address with no entry changes nothing": {
			current: []string{"other@example.com=admin"},
			want:    []string{"other@example.com=admin"},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			kc := keycloakMocks.NewKeyCloak(t)
			kc.EXPECT().ListGroups(mock.Anything, apitest.TestRealm, testOrgID).Return(reservedGroups(), nil).Once()
			expectInvitationsUpdate(t, kc, tt.current, tt.want)

			got := NewRoles(apitest.TestRealm, kc).ClearInvitation(context.Background(), testOrgID, "INVITEE@example.com")

			require.NoError(t, got)
		})
	}
}

func TestInvitations(t *testing.T) {
	withEntries := func(name string, entries ...string) keycloak.Group {
		return keycloak.Group{ID: "group-" + name, Name: name, Attributes: map[string][]string{invitedRolesAttribute: entries}}
	}
	tests := map[string]struct {
		groups []keycloak.Group
		want   map[string]Role
	}{
		"parses the entries of the Viewer group": {
			groups: []keycloak.Group{
				withEntries("Admin", "elsewhere@example.com=admin"),
				withEntries("VIEWER", "admin@example.com=admin", "Editor@Example.com=editor", "a=b@example.com=viewer",
					"no-role@example.com", "owner@example.com=owner", "=admin"),
				withEntries("Viewer", "second@example.com=admin"),
				withEntries("eng", "eng@example.com=editor"),
			},
			want: map[string]Role{"admin@example.com": Admin, "editor@example.com": Editor, "a=b@example.com": Viewer},
		},
		"no entries report no roles": {
			groups: reservedGroups(),
			want:   map[string]Role{},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			kc := keycloakMocks.NewKeyCloak(t)
			kc.EXPECT().ListGroups(mock.Anything, apitest.TestRealm, testOrgID).Return(tt.groups, nil).Once()

			got, err := NewRoles(apitest.TestRealm, kc).Invitations(context.Background(), testOrgID)

			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}
