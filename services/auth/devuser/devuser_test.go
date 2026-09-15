package devuser

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"xata/services/auth/keycloak"
	keycloakMocks "xata/services/auth/keycloak/mocks"
)

func TestSetUpRoles(t *testing.T) {
	const (
		realm  = "xata"
		userID = "user-1"
	)
	reserved := []keycloak.Group{
		{ID: "group-admin", Name: "Admin"},
		{ID: "group-editor", Name: "Editor"},
		{ID: "group-viewer", Name: "Viewer"},
	}

	tests := map[string]struct {
		expect  func(*keycloakMocks.KeyCloak)
		wantErr bool
	}{
		"a new dev organization gets the reserved groups with the dev user as Admin": {
			expect: func(kc *keycloakMocks.KeyCloak) {
				kc.EXPECT().ListGroups(mock.Anything, realm, DevOrganization).Return(nil, nil).Once()
				for _, g := range reserved {
					kc.EXPECT().CreateGroup(mock.Anything, realm, DevOrganization, g.Name).Return(g, nil).Once()
				}
				kc.EXPECT().AddGroupMember(mock.Anything, realm, DevOrganization, "group-admin", userID).Return(nil).Once()
			},
		},
		"a rerun reuses the groups and keeps the dev user Admin": {
			expect: func(kc *keycloakMocks.KeyCloak) {
				kc.EXPECT().ListGroups(mock.Anything, realm, DevOrganization).Return(reserved, nil).Once()
				kc.EXPECT().AddGroupMember(mock.Anything, realm, DevOrganization, "group-admin", userID).Return(nil).Once()
			},
		},
		"a failed grant fails the command": {
			expect: func(kc *keycloakMocks.KeyCloak) {
				kc.EXPECT().ListGroups(mock.Anything, realm, DevOrganization).Return(reserved, nil).Once()
				kc.EXPECT().AddGroupMember(mock.Anything, realm, DevOrganization, "group-admin", userID).
					Return(errors.New("keycloak unavailable")).Once()
			},
			wantErr: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			kc := keycloakMocks.NewKeyCloak(t)
			test.expect(kc)

			got := setUpRoles(context.Background(), kc, realm, userID)

			if test.wantErr {
				require.Error(t, got)
				return
			}
			require.NoError(t, got)
		})
	}
}
