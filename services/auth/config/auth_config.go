package config

// AuthConfig contains configuration for Keycloak authentication
type AuthConfig struct {
	KeycloakURL           string `env:"KEYCLOAK_URL" env-default:"http://localhost:8080/"`
	Realm                 string `env:"KEYCLOAK_REALM" env-default:"xata"`
	KeycloakAdminUsername string `env:"KEYCLOAK_ADMIN_USERNAME"`
	KeycloakAdminPassword string `env:"KEYCLOAK_ADMIN_PASSWORD"`
	FrontendURL           string `env:"FRONTEND_URL"`
	BillingRequired       bool   `env:"BILLING_REQUIRED" env-default:"false"`
}
