package api

import (
	"testing"

	"xata/services/auth/api/spec"
	"xata/services/auth/keycloak"

	"github.com/stretchr/testify/require"
)

func TestToSpecOrganizationMembers(t *testing.T) {
	testCases := map[string]struct {
		members []keycloak.OrganizationMember
		want    []spec.UserWithID
	}{
		"members": {
			members: []keycloak.OrganizationMember{
				{ID: "u1", Email: "one@example.com", Name: "One"},
				{ID: "u2", Email: "two@example.com", Name: "Two"},
			},
			want: []spec.UserWithID{
				{Id: "u1", Email: "one@example.com", Name: "One"},
				{Id: "u2", Email: "two@example.com", Name: "Two"},
			},
		},
		"empty": {
			members: []keycloak.OrganizationMember{},
			want:    []spec.UserWithID{},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got := ToSpecOrganizationMembers(tc.members)
			require.Equal(t, tc.want, got)
		})
	}
}
