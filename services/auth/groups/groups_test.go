package groups

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
	testOrgID   = "org-123"
	testGroupID = "group-abc"
	testUserID  = "user-1"
	ownerID     = "owner-id"
)

func ownerGroup() keycloak.Group {
	return keycloak.Group{ID: ownerID, Name: OwnerGroupName}
}

func regularGroup() keycloak.Group {
	return keycloak.Group{ID: testGroupID, Name: "engineering"}
}

func newService(t *testing.T, setup func(*keycloakMocks.KeyCloak)) Groups {
	t.Helper()
	mockKC := keycloakMocks.NewKeyCloak(t)
	if setup != nil {
		setup(mockKC)
	}
	return NewGroups(apitest.TestRealm, mockKC)
}

func TestListWithMemberCounts(t *testing.T) {
	s := newService(t, func(kc *keycloakMocks.KeyCloak) {
		kc.EXPECT().ListGroups(mock.Anything, apitest.TestRealm, testOrgID).
			Return([]keycloak.Group{ownerGroup(), regularGroup()}, nil).Once()
		kc.EXPECT().ListMembers(mock.Anything, apitest.TestRealm, testOrgID).
			Return([]keycloak.OrganizationMember{{ID: testUserID}}, nil).Once()
		kc.EXPECT().ListGroupMembers(mock.Anything, apitest.TestRealm, testOrgID, ownerID).
			Return([]keycloak.OrganizationMember{{ID: testUserID}}, nil).Once()
		// Keycloak still lists someone who has left the organization.
		kc.EXPECT().ListGroupMembers(mock.Anything, apitest.TestRealm, testOrgID, testGroupID).
			Return([]keycloak.OrganizationMember{{ID: testUserID}, {ID: "who-left"}}, nil).Once()
	})

	got, err := s.ListWithMemberCounts(context.Background(), testOrgID)

	require.NoError(t, err)
	require.Equal(t, []GroupWithMembers{
		{Group: ownerGroup(), MemberCount: 1},
		{Group: regularGroup(), MemberCount: 1},
	}, got)
}

func TestCreate(t *testing.T) {
	tests := map[string]struct {
		groupName string
		setup     func(*keycloakMocks.KeyCloak)
		check     func(t *testing.T, group keycloak.Group, err error)
	}{
		"rejects the reserved Owner name": {
			groupName: OwnerGroupName,
			check: func(t *testing.T, _ keycloak.Group, err error) {
				require.ErrorAs(t, err, &ErrGroupNameReserved{})
			},
		},
		"creates a regular group": {
			groupName: "engineering",
			setup: func(kc *keycloakMocks.KeyCloak) {
				kc.EXPECT().CreateGroup(mock.Anything, apitest.TestRealm, testOrgID, "engineering").
					Return(regularGroup(), nil).Once()
			},
			check: func(t *testing.T, group keycloak.Group, err error) {
				require.NoError(t, err)
				require.Equal(t, "engineering", group.Name)
			},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			group, err := newService(t, tt.setup).Create(context.Background(), testOrgID, tt.groupName)
			tt.check(t, group, err)
		})
	}
}

func TestUpdate(t *testing.T) {
	tests := map[string]struct {
		groupID string
		newName string
		setup   func(*keycloakMocks.KeyCloak)
		check   func(t *testing.T, group keycloak.Group, err error)
	}{
		"rejects updating the Owner group": {
			groupID: ownerID,
			newName: "renamed",
			setup: func(kc *keycloakMocks.KeyCloak) {
				kc.EXPECT().GetGroup(mock.Anything, apitest.TestRealm, testOrgID, ownerID).Return(ownerGroup(), nil).Once()
			},
			check: func(t *testing.T, _ keycloak.Group, err error) {
				require.ErrorAs(t, err, &ErrOwnerGroupImmutable{})
			},
		},
		"rejects renaming to the reserved Owner name": {
			groupID: testGroupID,
			newName: OwnerGroupName,
			setup: func(kc *keycloakMocks.KeyCloak) {
				kc.EXPECT().GetGroup(mock.Anything, apitest.TestRealm, testOrgID, testGroupID).Return(regularGroup(), nil).Once()
			},
			check: func(t *testing.T, _ keycloak.Group, err error) {
				require.ErrorAs(t, err, &ErrGroupNameReserved{})
			},
		},
		"renames a regular group": {
			groupID: testGroupID,
			newName: "platform",
			setup: func(kc *keycloakMocks.KeyCloak) {
				kc.EXPECT().GetGroup(mock.Anything, apitest.TestRealm, testOrgID, testGroupID).Return(regularGroup(), nil).Once()
				kc.EXPECT().UpdateGroup(mock.Anything, apitest.TestRealm, testOrgID, testGroupID, "platform").
					Return(keycloak.Group{ID: testGroupID, Name: "platform"}, nil).Once()
			},
			check: func(t *testing.T, group keycloak.Group, err error) {
				require.NoError(t, err)
				require.Equal(t, "platform", group.Name)
			},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			group, err := newService(t, tt.setup).Update(context.Background(), testOrgID, tt.groupID, tt.newName)
			tt.check(t, group, err)
		})
	}
}

func TestDelete(t *testing.T) {
	tests := map[string]struct {
		groupID string
		setup   func(*keycloakMocks.KeyCloak)
		check   func(t *testing.T, err error)
	}{
		"rejects deleting the Owner group": {
			groupID: ownerID,
			setup: func(kc *keycloakMocks.KeyCloak) {
				kc.EXPECT().GetGroup(mock.Anything, apitest.TestRealm, testOrgID, ownerID).Return(ownerGroup(), nil).Once()
			},
			check: func(t *testing.T, err error) {
				require.ErrorAs(t, err, &ErrOwnerGroupImmutable{})
			},
		},
		"deletes a regular group": {
			groupID: testGroupID,
			setup: func(kc *keycloakMocks.KeyCloak) {
				kc.EXPECT().GetGroup(mock.Anything, apitest.TestRealm, testOrgID, testGroupID).Return(regularGroup(), nil).Once()
				kc.EXPECT().DeleteGroup(mock.Anything, apitest.TestRealm, testOrgID, testGroupID).Return(nil).Once()
			},
			check: func(t *testing.T, err error) {
				require.NoError(t, err)
			},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			tt.check(t, newService(t, tt.setup).Delete(context.Background(), testOrgID, tt.groupID))
		})
	}
}

func TestAddMember(t *testing.T) {
	tests := map[string]struct {
		setup func(*keycloakMocks.KeyCloak)
		check func(t *testing.T, err error)
	}{
		"rejects users who are not organization members": {
			setup: func(kc *keycloakMocks.KeyCloak) {
				kc.EXPECT().GetGroup(mock.Anything, apitest.TestRealm, testOrgID, testGroupID).
					Return(regularGroup(), nil).Maybe()
				kc.EXPECT().ListMembers(mock.Anything, apitest.TestRealm, testOrgID).
					Return([]keycloak.OrganizationMember{{ID: "someone-else"}}, nil).Once()
			},
			check: func(t *testing.T, err error) {
				require.ErrorAs(t, err, &ErrUserNotOrganizationMember{})
			},
		},
		"adds an organization member to the group": {
			setup: func(kc *keycloakMocks.KeyCloak) {
				kc.EXPECT().GetGroup(mock.Anything, apitest.TestRealm, testOrgID, testGroupID).
					Return(regularGroup(), nil).Maybe()
				kc.EXPECT().ListMembers(mock.Anything, apitest.TestRealm, testOrgID).
					Return([]keycloak.OrganizationMember{{ID: testUserID}}, nil).Once()
				kc.EXPECT().AddGroupMember(mock.Anything, apitest.TestRealm, testOrgID, testGroupID, testUserID).Return(nil).Once()
			},
			check: func(t *testing.T, err error) {
				require.NoError(t, err)
			},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			tt.check(t, newService(t, tt.setup).AddMember(context.Background(), testOrgID, testGroupID, testUserID, "anyone"))
		})
	}
}

func TestRemoveMember(t *testing.T) {
	tests := map[string]struct {
		groupID  string
		callerID string
		setup    func(*keycloakMocks.KeyCloak)
		check    func(t *testing.T, err error)
	}{
		"blocks removing the last Owner member": {
			groupID:  ownerID,
			callerID: testUserID,
			setup: func(kc *keycloakMocks.KeyCloak) {
				kc.EXPECT().GetGroup(mock.Anything, apitest.TestRealm, testOrgID, ownerID).Return(ownerGroup(), nil).Once()
				kc.EXPECT().ListGroupMembers(mock.Anything, apitest.TestRealm, testOrgID, ownerID).
					Return([]keycloak.OrganizationMember{{ID: testUserID}}, nil).Once()
				kc.EXPECT().ListMembers(mock.Anything, apitest.TestRealm, testOrgID).
					Return([]keycloak.OrganizationMember{{ID: testUserID}}, nil).Maybe()
			},
			check: func(t *testing.T, err error) {
				require.ErrorAs(t, err, &ErrOwnerGroupLastMember{})
			},
		},
		"allows removing an Owner member when others remain": {
			groupID:  ownerID,
			callerID: "user-2",
			setup: func(kc *keycloakMocks.KeyCloak) {
				kc.EXPECT().GetGroup(mock.Anything, apitest.TestRealm, testOrgID, ownerID).Return(ownerGroup(), nil).Once()
				kc.EXPECT().ListGroupMembers(mock.Anything, apitest.TestRealm, testOrgID, ownerID).
					Return([]keycloak.OrganizationMember{{ID: testUserID}, {ID: "user-2"}}, nil).Once()
				kc.EXPECT().ListMembers(mock.Anything, apitest.TestRealm, testOrgID).
					Return([]keycloak.OrganizationMember{{ID: testUserID}, {ID: "user-2"}}, nil).Maybe()
				kc.EXPECT().RemoveGroupMember(mock.Anything, apitest.TestRealm, testOrgID, ownerID, testUserID).Return(nil).Once()
			},
			check: func(t *testing.T, err error) {
				require.NoError(t, err)
			},
		},
		"does not block a no-op removal of a non-member from the Owner group": {
			groupID:  ownerID,
			callerID: "the-only-owner",
			setup: func(kc *keycloakMocks.KeyCloak) {
				kc.EXPECT().GetGroup(mock.Anything, apitest.TestRealm, testOrgID, ownerID).Return(ownerGroup(), nil).Once()
				kc.EXPECT().ListGroupMembers(mock.Anything, apitest.TestRealm, testOrgID, ownerID).
					Return([]keycloak.OrganizationMember{{ID: "the-only-owner"}}, nil).Once()
				kc.EXPECT().ListMembers(mock.Anything, apitest.TestRealm, testOrgID).
					Return([]keycloak.OrganizationMember{{ID: "the-only-owner"}}, nil).Maybe()
				kc.EXPECT().RemoveGroupMember(mock.Anything, apitest.TestRealm, testOrgID, ownerID, testUserID).Return(nil).Once()
			},
			check: func(t *testing.T, err error) {
				require.NoError(t, err)
			},
		},
		"removes a member from a regular group without checks": {
			groupID:  testGroupID,
			callerID: "anyone",
			setup: func(kc *keycloakMocks.KeyCloak) {
				kc.EXPECT().GetGroup(mock.Anything, apitest.TestRealm, testOrgID, testGroupID).Return(regularGroup(), nil).Once()
				kc.EXPECT().RemoveGroupMember(mock.Anything, apitest.TestRealm, testOrgID, testGroupID, testUserID).Return(nil).Once()
			},
			check: func(t *testing.T, err error) {
				require.NoError(t, err)
			},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			tt.check(t, newService(t, tt.setup).RemoveMember(context.Background(), testOrgID, tt.groupID, testUserID, tt.callerID))
		})
	}
}

func TestEnsureOwnerGroup(t *testing.T) {
	tests := map[string]struct {
		seedMemberIDs []string
		setup         func(*keycloakMocks.KeyCloak)
		check         func(t *testing.T, group keycloak.Group, err error)
	}{
		"creates and seeds the Owner group when missing": {
			seedMemberIDs: []string{testUserID},
			setup: func(kc *keycloakMocks.KeyCloak) {
				kc.EXPECT().ListGroups(mock.Anything, apitest.TestRealm, testOrgID).Return([]keycloak.Group{regularGroup()}, nil).Once()
				kc.EXPECT().CreateGroup(mock.Anything, apitest.TestRealm, testOrgID, OwnerGroupName).Return(ownerGroup(), nil).Once()
				// A freshly created group is seeded directly, without an extra members lookup.
				kc.EXPECT().AddGroupMember(mock.Anything, apitest.TestRealm, testOrgID, ownerID, testUserID).Return(nil).Once()
			},
			check: func(t *testing.T, group keycloak.Group, err error) {
				require.NoError(t, err)
				require.Equal(t, OwnerGroupName, group.Name)
			},
		},
		"seeds all provided members when creating for a pre-existing org": {
			seedMemberIDs: []string{"u1", "u2"},
			setup: func(kc *keycloakMocks.KeyCloak) {
				kc.EXPECT().ListGroups(mock.Anything, apitest.TestRealm, testOrgID).Return([]keycloak.Group{}, nil).Once()
				kc.EXPECT().CreateGroup(mock.Anything, apitest.TestRealm, testOrgID, OwnerGroupName).Return(ownerGroup(), nil).Once()
				kc.EXPECT().AddGroupMember(mock.Anything, apitest.TestRealm, testOrgID, ownerID, "u1").Return(nil).Once()
				kc.EXPECT().AddGroupMember(mock.Anything, apitest.TestRealm, testOrgID, ownerID, "u2").Return(nil).Once()
			},
			check: func(t *testing.T, _ keycloak.Group, err error) {
				require.NoError(t, err)
			},
		},
		"is a no-op when the Owner group already has members": {
			seedMemberIDs: []string{testUserID},
			setup: func(kc *keycloakMocks.KeyCloak) {
				kc.EXPECT().ListGroups(mock.Anything, apitest.TestRealm, testOrgID).Return([]keycloak.Group{ownerGroup()}, nil).Once()
				kc.EXPECT().ListGroupMembers(mock.Anything, apitest.TestRealm, testOrgID, ownerID).
					Return([]keycloak.OrganizationMember{{ID: "existing-owner"}}, nil).Once()
				kc.EXPECT().ListMembers(mock.Anything, apitest.TestRealm, testOrgID).
					Return([]keycloak.OrganizationMember{{ID: "existing-owner"}}, nil).Maybe()
			},
			check: func(t *testing.T, group keycloak.Group, err error) {
				require.NoError(t, err)
				require.Equal(t, ownerID, group.ID)
			},
		},
		"seeds an existing but empty Owner group": {
			seedMemberIDs: []string{testUserID},
			setup: func(kc *keycloakMocks.KeyCloak) {
				kc.EXPECT().ListGroups(mock.Anything, apitest.TestRealm, testOrgID).Return([]keycloak.Group{ownerGroup()}, nil).Once()
				kc.EXPECT().ListGroupMembers(mock.Anything, apitest.TestRealm, testOrgID, ownerID).
					Return([]keycloak.OrganizationMember{}, nil).Once()
				kc.EXPECT().ListMembers(mock.Anything, apitest.TestRealm, testOrgID).
					Return([]keycloak.OrganizationMember{}, nil).Maybe()
				kc.EXPECT().AddGroupMember(mock.Anything, apitest.TestRealm, testOrgID, ownerID, testUserID).Return(nil).Once()
			},
			check: func(t *testing.T, _ keycloak.Group, err error) {
				require.NoError(t, err)
			},
		},
		"does not seed an existing group when no members are provided": {
			seedMemberIDs: nil,
			setup: func(kc *keycloakMocks.KeyCloak) {
				kc.EXPECT().ListGroups(mock.Anything, apitest.TestRealm, testOrgID).Return([]keycloak.Group{ownerGroup()}, nil).Once()
			},
			check: func(t *testing.T, _ keycloak.Group, err error) {
				require.NoError(t, err)
			},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var seed SeedMembers
			if tt.seedMemberIDs != nil {
				seed = func(context.Context) ([]string, error) { return tt.seedMemberIDs, nil }
			}
			group, err := newService(t, tt.setup).EnsureOwnerGroup(context.Background(), testOrgID, seed)
			tt.check(t, group, err)
		})
	}
}

func TestOwnerMembershipRequiresAnOwnerCaller(t *testing.T) {
	const owner, other = "an-owner", "not-an-owner"

	tests := map[string]struct {
		groupID    string
		callerID   string
		ownerGroup []keycloak.OrganizationMember
		orgMembers []keycloak.OrganizationMember
		wantErr    bool
	}{
		"any caller may change a regular group": {
			groupID:  testGroupID,
			callerID: other,
		},
		"a non-owner may not change the Owner group": {
			groupID:    ownerID,
			callerID:   other,
			ownerGroup: []keycloak.OrganizationMember{{ID: owner}, {ID: testUserID}},
			orgMembers: []keycloak.OrganizationMember{{ID: owner}, {ID: testUserID}},
			wantErr:    true,
		},
		"an owner who has left the organization may not": {
			groupID:    ownerID,
			callerID:   owner,
			ownerGroup: []keycloak.OrganizationMember{{ID: owner}, {ID: testUserID}},
			orgMembers: []keycloak.OrganizationMember{{ID: testUserID}},
			wantErr:    true,
		},
		"an organization API key carries no identity and is never an owner": {
			groupID:    ownerID,
			callerID:   "",
			ownerGroup: []keycloak.OrganizationMember{{ID: owner}, {ID: testUserID}},
			orgMembers: []keycloak.OrganizationMember{{ID: owner}, {ID: testUserID}},
			wantErr:    true,
		},
		"an active owner may": {
			groupID:    ownerID,
			callerID:   owner,
			ownerGroup: []keycloak.OrganizationMember{{ID: owner}, {ID: testUserID}},
			orgMembers: []keycloak.OrganizationMember{{ID: owner}, {ID: testUserID}},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			s := newService(t, func(kc *keycloakMocks.KeyCloak) {
				group := regularGroup()
				if tt.groupID == ownerID {
					group = ownerGroup()
				}
				kc.EXPECT().GetGroup(mock.Anything, apitest.TestRealm, testOrgID, tt.groupID).Return(group, nil).Maybe()
				kc.EXPECT().ListGroupMembers(mock.Anything, apitest.TestRealm, testOrgID, tt.groupID).
					Return(tt.ownerGroup, nil).Maybe()
				kc.EXPECT().ListMembers(mock.Anything, apitest.TestRealm, testOrgID).Return(tt.orgMembers, nil).Maybe()
				kc.EXPECT().RemoveGroupMember(mock.Anything, apitest.TestRealm, testOrgID, tt.groupID, testUserID).
					Return(nil).Maybe()
			})

			got := s.RemoveMember(context.Background(), testOrgID, tt.groupID, testUserID, tt.callerID)

			if tt.wantErr {
				var want ErrNotOwner
				require.ErrorAs(t, got, &want)
				return
			}
			require.NoError(t, got)
		})
	}
}

func TestCheckOrganizationMemberRemovable(t *testing.T) {
	tests := map[string]struct {
		setup func(*keycloakMocks.KeyCloak)
		check func(t *testing.T, err error)
	}{
		"blocks removing the sole owner from the organization": {
			setup: func(kc *keycloakMocks.KeyCloak) {
				kc.EXPECT().ListGroups(mock.Anything, apitest.TestRealm, testOrgID).Return([]keycloak.Group{ownerGroup()}, nil).Once()
				kc.EXPECT().ListGroupMembers(mock.Anything, apitest.TestRealm, testOrgID, ownerID).
					Return([]keycloak.OrganizationMember{{ID: testUserID}}, nil).Once()
				kc.EXPECT().ListMembers(mock.Anything, apitest.TestRealm, testOrgID).
					Return([]keycloak.OrganizationMember{{ID: testUserID}}, nil).Maybe()
			},
			check: func(t *testing.T, err error) {
				require.ErrorAs(t, err, &ErrOwnerGroupLastMember{})
			},
		},
		"allows removal when other owners remain": {
			setup: func(kc *keycloakMocks.KeyCloak) {
				kc.EXPECT().ListGroups(mock.Anything, apitest.TestRealm, testOrgID).Return([]keycloak.Group{ownerGroup()}, nil).Once()
				kc.EXPECT().ListGroupMembers(mock.Anything, apitest.TestRealm, testOrgID, ownerID).
					Return([]keycloak.OrganizationMember{{ID: testUserID}, {ID: "user-2"}}, nil).Once()
				kc.EXPECT().ListMembers(mock.Anything, apitest.TestRealm, testOrgID).
					Return([]keycloak.OrganizationMember{{ID: testUserID}, {ID: "user-2"}}, nil).Maybe()
			},
			check: func(t *testing.T, err error) {
				require.NoError(t, err)
			},
		},
		"allows removal of a non-owner": {
			setup: func(kc *keycloakMocks.KeyCloak) {
				kc.EXPECT().ListGroups(mock.Anything, apitest.TestRealm, testOrgID).Return([]keycloak.Group{ownerGroup()}, nil).Once()
				kc.EXPECT().ListGroupMembers(mock.Anything, apitest.TestRealm, testOrgID, ownerID).
					Return([]keycloak.OrganizationMember{{ID: "the-owner"}}, nil).Once()
				kc.EXPECT().ListMembers(mock.Anything, apitest.TestRealm, testOrgID).
					Return([]keycloak.OrganizationMember{{ID: "the-owner"}}, nil).Maybe()
			},
			check: func(t *testing.T, err error) {
				require.NoError(t, err)
			},
		},
		"allows removal when there is no owner group": {
			setup: func(kc *keycloakMocks.KeyCloak) {
				kc.EXPECT().ListGroups(mock.Anything, apitest.TestRealm, testOrgID).Return([]keycloak.Group{regularGroup()}, nil).Once()
			},
			check: func(t *testing.T, err error) {
				require.NoError(t, err)
			},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			tt.check(t, newService(t, tt.setup).CheckOrganizationMemberRemovable(context.Background(), testOrgID, testUserID))
		})
	}
}

func TestRemoveMemberFromAllGroups(t *testing.T) {
	tests := map[string]struct {
		setup func(*keycloakMocks.KeyCloak)
		check func(t *testing.T, err error)
	}{
		"removes the user from every group": {
			setup: func(kc *keycloakMocks.KeyCloak) {
				kc.EXPECT().ListGroups(mock.Anything, apitest.TestRealm, testOrgID).
					Return([]keycloak.Group{ownerGroup(), regularGroup()}, nil).Once()
				kc.EXPECT().RemoveGroupMember(mock.Anything, apitest.TestRealm, testOrgID, ownerID, testUserID).Return(nil).Once()
				kc.EXPECT().RemoveGroupMember(mock.Anything, apitest.TestRealm, testOrgID, testGroupID, testUserID).Return(nil).Once()
			},
			check: func(t *testing.T, err error) {
				require.NoError(t, err)
			},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			tt.check(t, newService(t, tt.setup).RemoveMemberFromAllGroups(context.Background(), testOrgID, testUserID))
		})
	}
}

func TestReservedNameIsCaseInsensitive(t *testing.T) {
	tests := map[string]struct{ groupName string }{
		"exact":      {groupName: "Owner"},
		"lower":      {groupName: "owner"},
		"upper":      {groupName: "OWNER"},
		"mixed case": {groupName: "OwNeR"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			s := newService(t, nil)

			_, got := s.Create(context.Background(), testOrgID, tt.groupName)

			var want ErrGroupNameReserved
			require.ErrorAs(t, got, &want)
		})
	}
}

func TestCheckOrganizationMemberRemovableCountsActiveOwners(t *testing.T) {
	tests := map[string]struct {
		ownerGroup []keycloak.OrganizationMember
		orgMembers []keycloak.OrganizationMember
		wantErr    bool
	}{
		"stale entries do not count as owners": {
			ownerGroup: []keycloak.OrganizationMember{{ID: testUserID}, {ID: "left-the-org"}},
			orgMembers: []keycloak.OrganizationMember{{ID: testUserID}},
			wantErr:    true,
		},
		"another active owner remains": {
			ownerGroup: []keycloak.OrganizationMember{{ID: testUserID}, {ID: "co-owner"}},
			orgMembers: []keycloak.OrganizationMember{{ID: testUserID}, {ID: "co-owner"}},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			s := newService(t, func(kc *keycloakMocks.KeyCloak) {
				kc.EXPECT().ListGroups(mock.Anything, apitest.TestRealm, testOrgID).
					Return([]keycloak.Group{ownerGroup()}, nil).Maybe()
				kc.EXPECT().ListGroupMembers(mock.Anything, apitest.TestRealm, testOrgID, ownerID).
					Return(tt.ownerGroup, nil).Maybe()
				kc.EXPECT().ListMembers(mock.Anything, apitest.TestRealm, testOrgID).
					Return(tt.orgMembers, nil).Maybe()
			})

			got := s.CheckOrganizationMemberRemovable(context.Background(), testOrgID, testUserID)

			if tt.wantErr {
				var want ErrOwnerGroupLastMember
				require.ErrorAs(t, got, &want)
				return
			}
			require.NoError(t, got)
		})
	}
}
