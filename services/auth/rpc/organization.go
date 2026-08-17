package rpc

import (
	"xata/services/auth/keycloak"

	authv1 "xata/gen/proto/auth/v1"

	"google.golang.org/protobuf/types/known/timestamppb"
	"k8s.io/utils/ptr"
)

func keycloakOrganizationToProto(org keycloak.Organization) *authv1.Organization {
	resp := &authv1.Organization{
		Id:                      org.ID,
		Status:                  string(org.Status.EffectiveState()),
		DisabledByAdmin:         org.Status.DisabledByAdmin,
		DisabledByAdminReason:   org.Status.AdminReason,
		BillingStatus:           string(org.Status.BillingStatus),
		BillingReason:           org.Status.BillingReason,
		UsageTier:               string(org.Status.UsageTier),
		BillingCollectionMethod: string(org.BillingCollectionMethod),
		Marketplace:             string(ptr.Deref(org.Marketplace, "")),
	}
	if org.Status.CreatedAt != nil {
		resp.CreatedAt = timestamppb.New(*org.Status.CreatedAt)
	}
	if org.Status.DeletedAt != nil {
		resp.DeletedAt = timestamppb.New(*org.Status.DeletedAt)
	}
	return resp
}
