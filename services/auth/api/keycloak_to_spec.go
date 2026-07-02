package api

import (
	"xata/services/auth/api/spec"
	"xata/services/auth/keycloak"

	openapi_types "github.com/oapi-codegen/runtime/types"
)

func ToSpecOrganizationMembers(members []keycloak.OrganizationMember) []spec.UserWithID {
	result := make([]spec.UserWithID, len(members))
	for i, member := range members {
		result[i] = spec.UserWithID{
			Email: openapi_types.Email(member.Email),
			Name:  member.Name,
			Id:    member.ID,
		}
	}
	return result
}
