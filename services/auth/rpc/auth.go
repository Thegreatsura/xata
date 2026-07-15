package rpc

import (
	"context"
	"errors"
	"fmt"
	"log"

	"xata/services/auth/orgs"

	projectsv1 "xata/gen/proto/projects/v1"
	"xata/internal/api/key"
	"xata/internal/token"
	"xata/services/auth/keycloak"

	authv1 "xata/gen/proto/auth/v1"
	"xata/internal/opa"
	"xata/services/auth/store"

	"github.com/Nerzal/gocloak/v13"
	"github.com/open-policy-agent/opa/v1/rego"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"k8s.io/utils/ptr"
)

// githubIdentityProvider is the alias of the GitHub identity provider in Keycloak.
const githubIdentityProvider = "github"

// Ensure AuthService implements GRPCService interface.
var _ authv1.AuthServiceServer = (*AuthService)(nil)

// AuthService is a GRPC service for interacting with auth service.
type AuthService struct {
	authv1.UnsafeAuthServiceServer

	kcRest         keycloak.KeyCloak
	realm          string
	kc             *gocloak.GoCloak
	store          store.AuthStore
	policy         rego.PreparedEvalQuery
	projectsClient projectsv1.ProjectsServiceClient
	orgs           orgs.Organizations
	defaultOrgID   string
}

// NewAuthService creates a new AuthService.
func NewAuthService(store store.AuthStore, kcClient *gocloak.GoCloak, kcRest keycloak.KeyCloak, projectsClient projectsv1.ProjectsServiceClient, orgs orgs.Organizations, realm, defaultOrgID string) *AuthService {
	r := rego.New(
		rego.Query("data.policy.allow"),
		rego.Module("policy.rego", opa.Policy),
	)

	policy, err := r.PrepareForEval(context.Background())
	if err != nil {
		log.Fatalf("failed to prepare OPA policy: %v", err)
	}

	return &AuthService{
		store:          store,
		kc:             kcClient,
		kcRest:         kcRest,
		realm:          realm,
		policy:         policy,
		projectsClient: projectsClient,
		orgs:           orgs,
		defaultOrgID:   defaultOrgID,
	}
}

// validateJWT checks if the provided string is a valid JWT token and returns the user claims.
func (a *AuthService) validateJWT(ctx context.Context, tokenStr string) (*token.Claims, error) {
	jwt, claims, err := a.kc.DecodeAccessToken(ctx, tokenStr, a.realm)
	if err != nil {
		return nil, &store.ErrFailedToDecodeJWT{Err: err}
	}

	if !jwt.Valid {
		return nil, &store.ErrInvalidJWTToken{}
	}

	userID, err := claims.GetSubject()
	if err != nil {
		return nil, fmt.Errorf("failed to get user ID from JWT claims: %w", err)
	}

	return a.buildUserClaims(ctx, userID)
}

// ValidateAccess checks if the given token can call the specified endpoint
func (a *AuthService) ValidateAccess(ctx context.Context, req *authv1.ValidateAccessRequest) (*authv1.ValidateAccessResponse, error) {
	tokenStr := req.GetToken()

	var claims *token.Claims
	var err error
	k := key.Key(tokenStr)
	if !k.IsValid() {
		// Not a valid API key, run JWT validation
		claims, err = a.validateJWT(ctx, tokenStr)
		if err != nil {
			return nil, err
		}
	} else {
		// Get API Key claims
		storeAPIKey, err := a.store.ValidateAPIKey(ctx, k)
		if err != nil {
			return nil, err
		}

		switch storeAPIKey.TargetType {
		case store.KeyTargetOrganization:
			orgClaims, err := a.buildOrgClaims(ctx, storeAPIKey.TargetID)
			if err != nil {
				return nil, err
			}

			orgClaims.Scopes = storeAPIKey.Scopes
			orgClaims.Projects = storeAPIKey.Projects
			orgClaims.Branches = storeAPIKey.Branches
			orgClaims.KeyID = storeAPIKey.ID
			claims = orgClaims
		case store.KeyTargetUser:
			userClaims, err := a.buildUserClaims(ctx, storeAPIKey.TargetID)
			if err != nil {
				return nil, err
			}

			userClaims.Scopes = storeAPIKey.Scopes
			userClaims.Projects = storeAPIKey.Projects
			userClaims.Branches = storeAPIKey.Branches
			userClaims.KeyID = storeAPIKey.ID
			claims = userClaims
		default:
			return nil, &store.ErrInvalidAPIKeyTargetType{TargetType: string(storeAPIKey.TargetType)}
		}
	}

	// Check if the request is allowed by the policy
	policyOrgs := make(map[string]opa.Organization)
	for orgID, orgStatus := range claims.Organizations {
		policyOrgs[orgID] = opa.Organization{
			ID:     orgStatus.ID,
			Status: orgStatus.Status,
		}
	}
	input := opa.PolicyInput{
		Request: opa.RequestInput{
			Method:       req.GetMethod(),
			Path:         req.GetPath(),
			Scopes:       req.GetScopes(),
			Organization: req.GetOrganizationId(),
			Project:      req.GetProjectId(),
			Branch:       req.GetBranchId(),
		},
		Claims: opa.ClaimsInput{
			Scopes:        claims.Scopes,
			Organizations: policyOrgs,
			Projects:      claims.Projects,
			Branches:      claims.Branches,
		},
	}

	allowed := false
	res, err := a.policy.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		return nil, fmt.Errorf("OPA policy evaluation failed: %w", err)
	}

	if len(res) > 0 && len(res[0].Expressions) > 0 {
		if val, ok := res[0].Expressions[0].Value.(bool); ok {
			allowed = val
		}
	}

	specOrgs := make(map[string]*authv1.Organization)
	for orgID, orgStatus := range claims.Organizations {
		specOrgs[orgID] = &authv1.Organization{
			Id:          orgStatus.ID,
			Status:      orgStatus.Status,
			CreatedAt:   timestamppb.New(orgStatus.CreatedAt),
			UsageTier:   orgStatus.UsageTier,
			Marketplace: orgStatus.Marketplace,
		}
	}
	return &authv1.ValidateAccessResponse{
		Allow:         allowed,
		UserId:        claims.UserID(),
		ApiKeyId:      claims.APIKeyID(),
		UserEmail:     claims.UserEmail(),
		Scopes:        claims.Scopes,
		Organizations: specOrgs,
		Projects:      claims.Projects,
		Branches:      claims.Branches,
	}, nil
}

// GetGithubIdentityProviderToken returns the GitHub user access token stored in
// Keycloak for the user identified by the given Keycloak access token.
func (a *AuthService) GetGithubIdentityProviderToken(ctx context.Context, req *authv1.GetGithubIdentityProviderTokenRequest) (*authv1.GetGithubIdentityProviderTokenResponse, error) {
	tokenStr := req.GetToken()
	if tokenStr == "" {
		return nil, status.Error(codes.InvalidArgument, "token is required")
	}
	// API keys have no Keycloak session, so there is no brokered token to retrieve.
	if key.Key(tokenStr).IsValid() {
		return nil, status.Error(codes.Unauthenticated, "a user session is required")
	}

	jwt, _, err := a.kc.DecodeAccessToken(ctx, tokenStr, a.realm)
	if err != nil || !jwt.Valid {
		return nil, status.Error(codes.Unauthenticated, "invalid token")
	}

	githubToken, err := a.kcRest.GetIdentityProviderToken(ctx, a.realm, githubIdentityProvider, tokenStr)
	if err != nil {
		if _, ok := errors.AsType[keycloak.ErrIdentityProviderNotLinked](err); ok {
			return nil, status.Error(codes.FailedPrecondition, "github account is not connected")
		}
		if _, ok := errors.AsType[keycloak.ErrIdentityProviderTokenForbidden](err); ok {
			return nil, status.Error(codes.PermissionDenied, "not allowed to read the github identity provider token")
		}
		return nil, status.Error(codes.Unavailable, "get github identity provider token")
	}

	return &authv1.GetGithubIdentityProviderTokenResponse{AccessToken: githubToken}, nil
}

func (a *AuthService) GetOrganization(ctx context.Context, req *authv1.GetOrganizationRequest) (*authv1.GetOrganizationResponse, error) {
	org, err := a.kcRest.GetOrganization(ctx, a.realm, req.GetOrganizationId())
	if err != nil {
		return nil, fmt.Errorf("get organization: %w", err)
	}
	return &authv1.GetOrganizationResponse{Organization: keycloakOrganizationToProto(org)}, nil
}

func (a *AuthService) UpdateOrganization(ctx context.Context, req *authv1.UpdateOrganizationRequest) (*authv1.UpdateOrganizationResponse, error) {
	org, err := a.orgs.UpdateOrganization(ctx, req.GetOrganizationId(), orgs.UpdateOrganizationOptions{
		DisabledByAdmin:       &req.DisabledByAdmin,
		DisabledByAdminReason: req.DisabledByAdminReason,
	})
	if err != nil {
		return nil, err
	}
	return &authv1.UpdateOrganizationResponse{Organization: keycloakOrganizationToProto(*org)}, nil
}

// buildUserClaims constructs a token.Claims object for a user based on their Keycloak user ID.
func (a *AuthService) buildUserClaims(ctx context.Context, userID string) (*token.Claims, error) {
	if userID == "" {
		return nil, fmt.Errorf("user ID cannot be empty")
	}

	// GetUserRepresentation and ListOrganizations are independent Keycloak calls.
	// Run them concurrently to save a round-trip on this hot path (auth middleware).
	var user keycloak.User
	var orgList []keycloak.Organization
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		var err error
		user, err = a.kcRest.GetUserRepresentation(gctx, a.realm, userID)
		if err != nil {
			return fmt.Errorf("failed to get user from Keycloak: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		var err error
		orgList, err = a.kcRest.ListOrganizations(gctx, a.realm, userID)
		if err != nil {
			return fmt.Errorf("failed to list user organizations: %w", err)
		}
		return nil
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}

	if user.ID == "" {
		return nil, fmt.Errorf("user not found in Keycloak: %s", userID)
	}

	organizations := make(map[string]token.Organization, len(orgList))
	for _, org := range orgList {
		tokenOrg := token.Organization{
			ID:          org.ID,
			Status:      string(org.Status.EffectiveState()),
			UsageTier:   string(org.Status.UsageTier),
			Marketplace: string(ptr.Deref(org.Marketplace, "")),
		}
		if org.Status.CreatedAt != nil {
			tokenOrg.CreatedAt = *org.Status.CreatedAt
		}
		organizations[org.ID] = tokenOrg
	}

	if a.defaultOrgID != "" {
		if _, exists := organizations[a.defaultOrgID]; !exists {
			organizations[a.defaultOrgID] = token.Organization{
				ID:     a.defaultOrgID,
				Status: token.OrgEnabledStatus,
			}
		}
	}

	return &token.Claims{
		ID:            user.ID,
		Email:         user.Email,
		Organizations: organizations,
		Scopes:        []string{"*"},
		Projects:      []string{"*"},
		Branches:      []string{"*"},
	}, nil
}

func (a *AuthService) buildOrgClaims(ctx context.Context, orgID string) (*token.Claims, error) {
	if orgID == "" {
		return nil, fmt.Errorf("organization ID cannot be empty")
	}

	organization, err := a.kcRest.GetOrganization(ctx, a.realm, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to get organization [%s] from Keycloak: %w", orgID, err)
	}

	organizations := make(map[string]token.Organization, 1)
	tokenOrg := token.Organization{
		ID:          organization.ID,
		Status:      string(organization.Status.EffectiveState()),
		UsageTier:   string(organization.Status.UsageTier),
		Marketplace: string(ptr.Deref(organization.Marketplace, "")),
	}
	if organization.Status.CreatedAt != nil {
		tokenOrg.CreatedAt = *organization.Status.CreatedAt
	}
	organizations[organization.ID] = tokenOrg
	return &token.Claims{
		Organizations: organizations,
		Scopes:        []string{"*"},
		Projects:      []string{"*"},
		Branches:      []string{"*"},
	}, nil
}
