package auth

import (
	"xata/services/auth/config"
	"xata/services/auth/store/sqlstore"
)

type Config struct {
	SQLStore sqlstore.Config

	// AuthGRPCURL is the URL of the auth service gRPC endpoint
	AuthGRPCURL string `env:"AUTH_GRPC_URL" env-default:"auth:5002"`

	// ProjectsGRPCURL is the URL of the projects service gRPC endpoint
	ProjectsGRPCURL string `env:"PROJECTS_GRPC_URL" env-default:"projects:5002"`

	AuthConfig config.AuthConfig

	// DefaultOrgID is the ID of the default organization for OSS deployments
	DefaultOrgID string `env:"DEFAULT_ORG_ID" env-default:"default-org"`

	// DefaultOrgName is the name of the default organization for OSS deployments
	DefaultOrgName string `env:"DEFAULT_ORG_NAME" env-default:"Default Organization"`
}
