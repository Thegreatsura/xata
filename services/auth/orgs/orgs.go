package orgs

import (
	"context"
	"errors"
	"fmt"

	projectsv1 "xata/gen/proto/projects/v1"
	"xata/services/auth/api"
	"xata/services/auth/keycloak"

	"k8s.io/utils/ptr"
)

//go:generate go run github.com/vektra/mockery/v3 --output orgsmock --outpkg orgsmock --with-expecter --name Organizations

type Organizations interface {
	UpdateOrganization(ctx context.Context, organizationID string, request UpdateOrganizationOptions) (*keycloak.Organization, error)
}
type orgsService struct {
	realm          string
	kcRest         keycloak.KeyCloak
	projectsClient projectsv1.ProjectsServiceClient
}

func NewOrganizations(realm string, kcRest keycloak.KeyCloak, projectsClient projectsv1.ProjectsServiceClient) Organizations {
	return &orgsService{
		realm:          realm,
		kcRest:         kcRest,
		projectsClient: projectsClient,
	}
}

type UpdateOrganizationOptions struct {
	DisabledByAdmin         *bool
	DisabledByAdminReason   *string
	BillingStatus           *keycloak.OrganizationBillingStatus
	BillingReason           *string
	UsageTier               *keycloak.OrganizationUsageTier
	BillingCollectionMethod *keycloak.OrganizationBillingCollectionMethod
}

func (o *orgsService) UpdateOrganization(
	ctx context.Context,
	organizationID string,
	req UpdateOrganizationOptions,
) (*keycloak.Organization, error) {
	// Ensure the organization exists. Only a genuine not-found means no access;
	// a degraded Keycloak must surface as an error so callers don't mistake an
	// outage for a missing organization.
	if req.BillingCollectionMethod != nil && !req.BillingCollectionMethod.Valid() {
		return nil, fmt.Errorf("unsupported billing collection method %q", *req.BillingCollectionMethod)
	}

	// Ensure the organization exists
	organization, err := o.kcRest.GetOrganization(ctx, o.realm, organizationID, keycloak.GetOrganizationOptions{IncludeDeleted: false})
	if err != nil {
		if _, ok := errors.AsType[keycloak.ErrOrganizationNotFound](err); ok {
			return nil, api.ErrorNoOrganizationAccess{OrganizationID: organizationID}
		}
		return nil, fmt.Errorf("get organization %s: %w", organizationID, err)
	}

	// Desired state
	shouldDisableByAdmin := ptr.Deref(req.DisabledByAdmin, organization.Status.DisabledByAdmin)
	shouldBillingStatus := ptr.Deref(req.BillingStatus, organization.Status.BillingStatus)

	update := keycloak.OrganizationUpdate{}

	if organization.Status.DisabledByAdmin != shouldDisableByAdmin {
		update.DisabledByAdmin = &shouldDisableByAdmin
		if req.DisabledByAdminReason != nil {
			update.AdminReason = req.DisabledByAdminReason
		}
	}

	if organization.Status.BillingStatus != shouldBillingStatus {
		update.BillingStatus = &shouldBillingStatus
		if req.BillingReason != nil {
			update.BillingReason = req.BillingReason
		}
	}

	if req.UsageTier != nil && *req.UsageTier != organization.Status.UsageTier {
		update.UsageTier = req.UsageTier
	}

	if req.BillingCollectionMethod != nil && *req.BillingCollectionMethod != organization.BillingCollectionMethod {
		update.BillingCollectionMethod = req.BillingCollectionMethod
	}

	targetStatus := keycloak.OrganizationStatus{
		DisabledByAdmin: shouldDisableByAdmin,
		BillingStatus:   shouldBillingStatus,
	}
	targetState := targetStatus.EffectiveState()
	currentState := organization.Status.EffectiveState()

	// Only write Keycloak if flags actually changed (reasons alone do nothing)
	result := &organization
	if update.DisabledByAdmin != nil || update.BillingStatus != nil || update.UsageTier != nil || update.BillingCollectionMethod != nil {
		if targetState == keycloak.OrganizationStateEnabled && currentState == keycloak.OrganizationStateDisabled {
			update.ResourcesCleanedAt = new("")
		}

		org, err := o.kcRest.UpdateOrganization(ctx, o.realm, organizationID, update)
		if err != nil {
			return nil, fmt.Errorf("update organization in Keycloak: %w", err)
		}
		result = &org
	}

	// A deletion request is only accepted for an organization with no projects,
	// so there is nothing to disable.
	statusIsDeletion := shouldBillingStatus == keycloak.OrganizationBillingStatusDeletionRequested
	if (req.DisabledByAdmin != nil || req.BillingStatus != nil) && !statusIsDeletion {
		if _, err := o.projectsClient.UpdateOrganizationStatus(ctx, &projectsv1.UpdateOrganizationStatusRequest{
			OrganizationId: organizationID,
			Disabled:       targetState != keycloak.OrganizationStateEnabled,
		}); err != nil {
			return nil, fmt.Errorf("propagate organization status to projects: %w", err)
		}
	}

	return result, nil
}
