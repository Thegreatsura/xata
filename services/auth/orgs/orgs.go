package orgs

import (
	"context"
	"fmt"
	"time"

	projectsv1 "xata/gen/proto/projects/v1"
	"xata/services/auth/api"
	"xata/services/auth/keycloak"

	"github.com/cenkalti/backoff/v4"
	"github.com/rs/zerolog/log"
	"k8s.io/utils/ptr"
)

//go:generate go run github.com/vektra/mockery/v3 --output orgsmock --outpkg orgsmock --with-expecter --name Organizations

// Max number of retries for updating organization status in projects service
const projectsMaxRetries = uint64(5)

type Organizations interface {
	UpdateOrganization(ctx context.Context, organizationID string, request UpdateOrganizationOptions) (*keycloak.Organization, error)
}
type orgsService struct {
	realm          string
	kcRest         keycloak.KeyCloak
	projectsClient projectsv1.ProjectsServiceClient
	newBackoff     func() backoff.BackOff
}

func NewOrganizations(realm string, kcRest keycloak.KeyCloak, projectsClient projectsv1.ProjectsServiceClient) Organizations {
	return &orgsService{
		realm:          realm,
		kcRest:         kcRest,
		projectsClient: projectsClient,
		newBackoff: func() backoff.BackOff {
			return backoff.NewExponentialBackOff(backoff.WithMaxElapsedTime(30*time.Second), backoff.WithMaxInterval(3*time.Second))
		},
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
	if req.BillingCollectionMethod != nil && !req.BillingCollectionMethod.Valid() {
		return nil, fmt.Errorf("unsupported billing collection method %q", *req.BillingCollectionMethod)
	}

	// Ensure the organization exists
	organization, err := o.kcRest.GetOrganization(ctx, o.realm, organizationID, keycloak.GetOrganizationOptions{IncludeDeleted: false})
	if err != nil {
		return nil, api.ErrorNoOrganizationAccess{OrganizationID: organizationID}
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

	// Only update if flags actually changed (reasons alone do nothing)
	if update.DisabledByAdmin != nil || update.BillingStatus != nil || update.UsageTier != nil || update.BillingCollectionMethod != nil {
		targetStatus := keycloak.OrganizationStatus{
			DisabledByAdmin: shouldDisableByAdmin,
			BillingStatus:   shouldBillingStatus,
		}
		targetState := targetStatus.EffectiveState()
		currentState := organization.Status.EffectiveState()

		if targetState == keycloak.OrganizationStateEnabled && currentState == keycloak.OrganizationStateDisabled {
			update.ResourcesCleanedAt = new("")
		}

		org, err := o.kcRest.UpdateOrganization(ctx, o.realm, organizationID, update)
		if err != nil {
			return nil, fmt.Errorf("update organization in Keycloak: %w", err)
		}

		// Then trigger the change in the projects service, but only if general status changed
		if targetState != currentState {
			_, err := o.retryWithBackoff(ctx, o.projectsClient, &projectsv1.UpdateOrganizationStatusRequest{
				OrganizationId: organizationID,
				Disabled:       targetState != keycloak.OrganizationStateEnabled,
			})
			if err != nil {
				return nil, fmt.Errorf("propagate organization status to projects: %w", err)
			}
		} else {
			log.Ctx(ctx).Debug().Msg("general organization status hasn't changed; skipping projects service update")
		}

		return &org, nil
	}

	// Nothing changed at all (flags unchanged, reasons ignored)
	return &organization, nil
}

// retryWithBackoff provides retry logic with backoff for updating organization status in the projects service
func (o *orgsService) retryWithBackoff(ctx context.Context, client projectsv1.ProjectsServiceClient, req *projectsv1.UpdateOrganizationStatusRequest) (*projectsv1.UpdateOrganizationStatusResponse, error) {
	var result *projectsv1.UpdateOrganizationStatusResponse
	op := func() error {
		var err error
		result, err = client.UpdateOrganizationStatus(ctx, req)
		return err
	}

	bo := backoff.WithMaxRetries(o.newBackoff(), projectsMaxRetries)
	err := backoff.RetryNotify(op, backoff.WithContext(bo, ctx),
		func(err error, d time.Duration) {
			log.Ctx(ctx).Warn().Err(err).Dur("retry_in", d).Msg("update organization status in projects service; retrying")
		})

	return result, err
}
