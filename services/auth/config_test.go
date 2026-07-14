package auth

import (
	"testing"

	"xata/services/auth/config"

	"github.com/stretchr/testify/require"
)

func TestValidate(t *testing.T) {
	validAuthConfig := config.AuthConfig{
		KeycloakURL: "http://localhost:8080/",
		Realm:       "xata",
	}

	tests := map[string]struct {
		cfg     Config
		wantErr string
	}{
		"valid config": {
			cfg: Config{AuthConfig: validAuthConfig},
		},
		"missing keycloak url": {
			cfg: func() Config {
				c := validAuthConfig
				c.KeycloakURL = ""
				return Config{AuthConfig: c}
			}(),
			wantErr: "keycloak url is required",
		},
		"missing keycloak realm": {
			cfg: func() Config {
				c := validAuthConfig
				c.Realm = ""
				return Config{AuthConfig: c}
			}(),
			wantErr: "keycloak realm is required",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}
