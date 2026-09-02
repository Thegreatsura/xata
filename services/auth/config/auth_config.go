package config

// AuthConfig contains configuration for Keycloak authentication
type AuthConfig struct {
	KeycloakURL           string `env:"KEYCLOAK_URL" env-default:"http://localhost:8080/"`
	Realm                 string `env:"KEYCLOAK_REALM" env-default:"xata"`
	KeycloakAdminUsername string `env:"KEYCLOAK_ADMIN_USERNAME" env-default:"temp-admin"`
	KeycloakAdminPassword string `env:"KEYCLOAK_ADMIN_PASSWORD"`
	FrontendURL           string `env:"FRONTEND_URL"`
	BillingRequired       bool   `env:"BILLING_REQUIRED" env-default:"false"`

	// SeedInvitedUsers creates the Keycloak account for an invitee before asking
	// Keycloak to send the invitation. Keycloak picks the shape of the invite link
	// from whether that account exists at mint time, and only the "account exists"
	// shape can be accepted by someone who signs in through an identity provider.
	// The cost is that an invitee who wants a password has to reset one rather
	// than register, so this is opted into per environment.
	//
	// Temporary, until we run a Keycloak carrying keycloak/keycloak#52181.
	SeedInvitedUsers bool `env:"AUTH_SEED_INVITED_USERS" env-default:"false"`
}
