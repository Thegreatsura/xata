package api

import (
	"xata/services/auth/api/spec"
	"xata/services/auth/keycloak"
	"xata/services/auth/roles"

	openapi_types "github.com/oapi-codegen/runtime/types"
)

func ToSpecOrganization(org keycloak.Organization) spec.Organization {
	result := spec.Organization{
		Id:     org.ID,
		Name:   org.Name,
		Status: ToSpecOrganizationStatus(org.Status),
	}
	if org.Marketplace != nil {
		marketplace := spec.OrganizationMarketplaceProvider(*org.Marketplace)
		result.Marketplace = &marketplace
	}
	return result
}

func ToSpecOrganizations(orgs []keycloak.Organization) []spec.Organization {
	result := make([]spec.Organization, len(orgs))
	for i, org := range orgs {
		result[i] = ToSpecOrganization(org)
	}
	return result
}

func ToSpecOrganizationStatus(status keycloak.OrganizationStatus) spec.OrganizationStatus {
	return spec.OrganizationStatus{
		DisabledByAdmin: status.DisabledByAdmin,
		BillingStatus:   spec.OrganizationStatusBillingStatus(status.BillingStatus),
		AdminReason:     status.AdminReason,
		BillingReason:   status.BillingReason,
		LastUpdated:     status.LastUpdated,
		CreatedAt:       status.CreatedAt,
		UsageTier:       spec.OrganizationStatusUsageTier(status.UsageTier),
		Status:          spec.OrganizationStatusStatus(status.EffectiveState()),
	}
}

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

func ToSpecOrganizationMembersWithRoles(members []keycloak.OrganizationMember, held map[string]roles.Role) []spec.OrganizationMember {
	result := make([]spec.OrganizationMember, len(members))
	for i, member := range members {
		role := held[member.ID]
		if role == "" {
			role = roles.Default
		}
		result[i] = spec.OrganizationMember{
			Email: openapi_types.Email(member.Email),
			Name:  member.Name,
			Id:    member.ID,
			Role:  spec.OrganizationRoleName(role),
		}
	}
	return result
}

func ToSpecOrganizationRoles(definitions []roles.Definition) []spec.OrganizationRole {
	result := make([]spec.OrganizationRole, len(definitions))
	for i, d := range definitions {
		result[i] = spec.OrganizationRole{
			Id:          spec.OrganizationRoleName(d.Role),
			Name:        d.Name,
			Description: d.Description,
		}
	}
	return result
}
