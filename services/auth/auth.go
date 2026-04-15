package auth

import (
	"context"

	"xata/services/auth/orgs"

	"xata/services/auth/rpc"

	"xata/services/auth/billing"
	"xata/services/auth/keycloak"
	"xata/services/auth/store"
	"xata/services/auth/store/sqlstore"

	authv1 "xata/gen/proto/auth/v1"
	projectsv1 "xata/gen/proto/projects/v1"
	"xata/internal/analytics"
	capi "xata/internal/api"
	"xata/internal/envcfg"
	internalgrpc "xata/internal/grpc"
	"xata/internal/o11y"
	"xata/internal/openfeature"
	"xata/internal/service"
	"xata/services/auth/api"
	"xata/services/auth/api/spec"

	"github.com/Nerzal/gocloak/v13"
	"github.com/labstack/echo/v4"
	"google.golang.org/grpc"
)

// ensure AuthService implements HTTPService interface
var _ service.HTTPService = (*AuthService)(nil)

type AuthService struct {
	config       Config
	store        store.AuthStore
	feat         openfeature.Client
	analytics    analytics.Client
	authConn     *internalgrpc.ClientConnection
	projectsConn *internalgrpc.ClientConnection
}

func NewAuthService() *AuthService {
	return &AuthService{}
}

func (s *AuthService) Name() string {
	return "auth"
}

func (s *AuthService) ReadConfig(ctx context.Context) error {
	return envcfg.Read(&s.config)
}

func (s *AuthService) Setup(ctx context.Context) error {
	// setup the store
	err := s.store.Setup(ctx)
	if err != nil {
		return err
	}

	return nil
}

func (s *AuthService) Close(ctx context.Context) error {
	if err := s.store.Close(ctx); err != nil {
		return err
	}

	// Close the analytics client
	if err := s.analytics.Close(ctx); err != nil {
		return err
	}

	// Close the gRPC connection to the auth service
	if err := s.authConn.Close(); err != nil {
		return err
	}

	// Close the gRPC connection to the projects service
	if err := s.projectsConn.Close(); err != nil {
		return err
	}

	return nil
}

func (s *AuthService) Init(ctx context.Context) error {
	var err error
	o := o11y.Ctx(ctx)

	// Initialize the store with the projects client
	s.store, err = sqlstore.NewSQLAuthStore(ctx, s.config.SQLStore)
	if err != nil {
		return err
	}

	// Initialize the OpenFeature client (if not already set)
	if s.feat == nil {
		s.feat, err = openfeature.NewClient(ctx, "auth-service")
		if err != nil {
			return err
		}
	}

	// Initialize the analytics client (if not already set)
	if s.analytics == nil {
		s.analytics, err = analytics.NewClient(ctx)
		if err != nil {
			return err
		}
	}

	// Initialize the gRPC connection to the auth service
	s.authConn, err = internalgrpc.NewClient(o, s.config.AuthGRPCURL)
	if err != nil {
		return err
	}

	// Initialize the gRPC connection to the projects service
	s.projectsConn, err = internalgrpc.NewClient(o, s.config.ProjectsGRPCURL)
	if err != nil {
		return err
	}

	return nil
}

func (s *AuthService) Store() store.AuthStore {
	return s.store
}

func (s *AuthService) SetFeat(feat openfeature.Client) {
	s.feat = feat
}

func (s *AuthService) SetAnalytics(analytics analytics.Client) {
	s.analytics = analytics
}

func (s *AuthService) Config() Config {
	return s.config
}

func (s *AuthService) RegisterHTTPHandlers(o *o11y.O, router *echo.Group) error {
	// require auth for all regular routes
	group := router.Group("", capi.AuthMiddleware(s.authConn), openfeature.Middleware())

	client := gocloak.NewClient(s.config.AuthConfig.KeycloakURL)
	keyCloak := keycloak.NewRestKC(client, s.config.AuthConfig)
	projectsClient := projectsv1.NewProjectsServiceClient(s.projectsConn)
	billingClient, err := s.createBillingClient()
	if err != nil {
		return err
	}

	// API endpoints
	spec.RegisterHandlers(group, api.NewPublicAPIHandler(s.feat, keyCloak, s.config.AuthConfig.Realm, s.store, projectsClient, billingClient, s.analytics, s.config.DefaultOrgID, s.config.DefaultOrgName))

	return nil
}

// RegisterGRPCHandlers implements service.GRPCService.
func (s *AuthService) RegisterGRPCHandlers(o *o11y.O, server *grpc.Server) {
	client := gocloak.NewClient(s.config.AuthConfig.KeycloakURL)
	kcRest := keycloak.NewRestKC(client, s.config.AuthConfig)
	projectsClient := projectsv1.NewProjectsServiceClient(s.projectsConn)
	orgs := orgs.NewOrganizations(s.config.AuthConfig.Realm, kcRest, projectsClient)
	authv1.RegisterAuthServiceServer(server, rpc.NewAuthService(s.store, client, kcRest, projectsClient, orgs, s.config.AuthConfig.Realm, s.config.DefaultOrgID))
}

func (s *AuthService) createBillingClient() (billing.Client, error) {
	return &billing.NoopBilling{}, nil
}
