package devuser

import (
	"context"
	"fmt"

	"github.com/Nerzal/gocloak/v13"
	"github.com/spf13/cobra"

	"xata/internal/envcfg"
	"xata/services/auth/config"
	"xata/services/auth/store"
	"xata/services/auth/store/sqlstore"
)

// DevAPIKeyName names the key this command owns. Every run replaces the key
// under that name, so the value it prints is always the live one and repeated
// runs cannot walk into the per-target key limit.
const DevAPIKeyName = "dev"

// CreateDevAPIKeyCmd mints an API key for the dev user and prints it on stdout,
// so local environments can authenticate without the password grant.
func CreateDevAPIKeyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "create_dev_api_key",
		Short: "Create an API key for the dev user and print it",
		RunE: func(cmd *cobra.Command, args []string) error {
			var cfg struct {
				config.AuthConfig
				SQLStore sqlstore.Config
			}
			if err := envcfg.Read(&cfg); err != nil {
				return err
			}

			client := gocloak.NewClient(cfg.KeycloakURL)
			jwt, err := client.LoginAdmin(cmd.Context(), "temp-admin", cfg.KeycloakAdminPassword, "master")
			if err != nil {
				return fmt.Errorf("login as admin: %w", err)
			}

			userID, err := findDevUser(cmd.Context(), client, jwt.AccessToken, cfg.Realm)
			if err != nil {
				return err
			}
			if userID == "" {
				return fmt.Errorf("user %s does not exist, run create_dev_user first", DevUsername)
			}

			authStore, err := sqlstore.NewSQLAuthStore(cmd.Context(), cfg.SQLStore)
			if err != nil {
				return fmt.Errorf("create store: %w", err)
			}
			defer authStore.Close(cmd.Context())

			apiKey, err := replaceDevAPIKey(cmd.Context(), authStore, userID)
			if err != nil {
				return err
			}

			//nolint:forbidigo
			fmt.Println(apiKey)
			return nil
		},
	}
}

// findDevUser returns the dev user's Keycloak ID, or an empty string when it
// does not exist yet.
func findDevUser(ctx context.Context, client *gocloak.GoCloak, token, realm string) (string, error) {
	users, err := client.GetUsers(ctx, token, realm, gocloak.GetUsersParams{
		Username: new(DevUsername),
	})
	if err != nil {
		return "", fmt.Errorf("get users: %w", err)
	}
	if len(users) == 0 {
		return "", nil
	}
	return *users[0].ID, nil
}

func replaceDevAPIKey(ctx context.Context, authStore store.AuthStore, userID string) (string, error) {
	existing, err := authStore.ListAPIKeys(ctx, store.KeyTargetUser, userID)
	if err != nil {
		return "", fmt.Errorf("list api keys: %w", err)
	}

	var previous []string
	for _, apiKey := range existing {
		if apiKey.Name == DevAPIKeyName {
			previous = append(previous, apiKey.ID)
		}
	}
	if len(previous) > 0 {
		if err := authStore.DeleteAPIKeys(ctx, store.KeyTargetUser, userID, previous); err != nil {
			return "", fmt.Errorf("delete previous api key: %w", err)
		}
	}

	apiKey, _, err := authStore.CreateAPIKey(ctx, store.KeyTargetUser, userID, &store.APIKeyCreate{
		Name: DevAPIKeyName,
	})
	if err != nil {
		return "", fmt.Errorf("create api key: %w", err)
	}

	return apiKey.String(), nil
}
