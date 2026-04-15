package sqlstore

import (
	"context"
	"errors"
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"xata/internal/api/key"
	"xata/services/auth/store"
)

func TestSQLAuthStore(t *testing.T) {
	ctx := context.Background()
	sqlStore := setupSQLStore(ctx, t)

	t.Run("api_keys", func(t *testing.T) {
		// Test creating API keys
		t.Run("create_api_key", func(t *testing.T) {
			// Use UTC time to avoid timezone issues in tests
			expiry := time.Now().UTC().Add(24 * time.Hour)

			tests := []struct {
				name         string
				targetType   store.KeyTargetType
				targetID     string
				keyCreate    *store.APIKeyCreate
				wantError    bool
				errorChecker func(error) bool
			}{
				{
					name:       "create organization API key",
					targetType: store.KeyTargetOrganization,
					targetID:   "org123",
					keyCreate: &store.APIKeyCreate{
						Name: "test-org-key",
					},
					wantError: false,
				},
				{
					name:       "create user API key",
					targetType: store.KeyTargetUser,
					targetID:   "user456",
					keyCreate: &store.APIKeyCreate{
						Name: "test-user-key",
					},
					wantError: false,
				},
				{
					name:       "create API key with expiry",
					targetType: store.KeyTargetOrganization,
					targetID:   "org123",
					keyCreate: &store.APIKeyCreate{
						Name:   "test-expiry-key",
						Expiry: &expiry,
					},
					wantError: false,
				},
				{
					name:       "create duplicate key name",
					targetType: store.KeyTargetOrganization,
					targetID:   "org123",
					keyCreate: &store.APIKeyCreate{
						Name: "test-org-key", // Same name as first test
					},
					wantError: true,
					errorChecker: func(err error) bool {
						var apiKeyErr *store.ErrAPIKeyAlreadyExists
						return errors.As(err, &apiKeyErr)
					},
				},
				{
					name:       "create key fails when limit reached",
					targetType: store.KeyTargetOrganization,
					targetID:   "org-limit",
					keyCreate: &store.APIKeyCreate{
						Name: "limit-key",
					},
					wantError: true,
					errorChecker: func(err error) bool {
						var limitErr *store.ErrAPIKeyLimitReached
						return errors.As(err, &limitErr)
					},
				},
				{
					name:       "create key with invalid target type",
					targetType: store.KeyTargetType("invalid-target"),
					targetID:   "123",
					keyCreate: &store.APIKeyCreate{
						Name: "test-invalid-key",
					},
					wantError: true,
					errorChecker: func(err error) bool {
						var targetTypeErr *store.ErrUnsupportedTargetType
						return errors.As(err, &targetTypeErr)
					},
				},
				{
					name:       "create user API key with scopes",
					targetType: store.KeyTargetUser,
					targetID:   "user-scopes",
					keyCreate: &store.APIKeyCreate{
						Name:   "user-key-with-scopes",
						Scopes: []string{"org:read", "project:write"},
					},
					wantError: false,
				},
				{
					name:       "create organization API key with scopes and restrictions",
					targetType: store.KeyTargetOrganization,
					targetID:   "org-restricted",
					keyCreate: &store.APIKeyCreate{
						Name:     "org-key-restricted",
						Scopes:   []string{"project:read", "branch:read"},
						Projects: []string{"proj1", "proj2"},
						Branches: []string{"main", "dev"},
					},
					wantError: false,
				},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					if tt.name == "create key fails when limit reached" {
						for i := range store.MaxAPIKeysPerTarget {
							_, _, err := sqlStore.CreateAPIKey(ctx, tt.targetType, tt.targetID, &store.APIKeyCreate{Name: fmt.Sprintf("pre-%d", i)})
							assert.NoError(t, err)
						}
					}

					apiKeyStr, apiKey, err := sqlStore.CreateAPIKey(ctx, tt.targetType, tt.targetID, tt.keyCreate)

					if tt.wantError {
						assert.Error(t, err)
						if tt.errorChecker != nil {
							assert.True(t, tt.errorChecker(err), "expected specific error type")
						}
					} else {
						assert.NoError(t, err)
						assert.NotEmpty(t, apiKeyStr)
						assert.NotEmpty(t, apiKey.ID)
						assert.Equal(t, tt.keyCreate.Name, apiKey.Name)
						assert.Equal(t, tt.targetType, apiKey.TargetType)
						assert.Equal(t, tt.targetID, apiKey.TargetID)
						assert.NotEmpty(t, apiKey.KeyHash)
						assert.Equal(t, apiKeyStr.Obfuscate(key.DefaultObfuscateCharsCount), apiKey.KeyPreview)

						if tt.keyCreate.Expiry != nil {
							assert.NotNil(t, apiKey.Expiry)
							assert.WithinDuration(t, *tt.keyCreate.Expiry, *apiKey.Expiry, time.Second)
						}

						// Validate scopes and restrictions
						assert.Equal(t, tt.keyCreate.Scopes, apiKey.Scopes)
						assert.Equal(t, tt.keyCreate.Projects, apiKey.Projects)
						assert.Equal(t, tt.keyCreate.Branches, apiKey.Branches)
					}
				})
			}
		})

		// Test listing API keys
		t.Run("list_api_keys", func(t *testing.T) {
			tests := []struct {
				name         string
				targetType   store.KeyTargetType
				targetID     string
				expectEmpty  bool
				expectedType store.KeyTargetType
				expectedID   string
			}{
				{
					name:         "list organization API keys",
					targetType:   store.KeyTargetOrganization,
					targetID:     "org123",
					expectEmpty:  false,
					expectedType: store.KeyTargetOrganization,
					expectedID:   "org123",
				},
				{
					name:         "list user API keys",
					targetType:   store.KeyTargetUser,
					targetID:     "user456",
					expectEmpty:  false,
					expectedType: store.KeyTargetUser,
					expectedID:   "user456",
				},
				{
					name:        "list non-existent target",
					targetType:  store.KeyTargetOrganization,
					targetID:    "non-existent",
					expectEmpty: true,
				},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					keys, err := sqlStore.ListAPIKeys(ctx, tt.targetType, tt.targetID)
					assert.NoError(t, err)

					if tt.expectEmpty {
						assert.Empty(t, keys)
					} else {
						assert.NotEmpty(t, keys)
						for _, key := range keys {
							assert.Equal(t, tt.expectedType, key.TargetType)
							assert.Equal(t, tt.expectedID, key.TargetID)
						}
					}
				})
			}
		})

		// Test deleting API keys
		t.Run("delete_api_keys", func(t *testing.T) {
			// Create keys that we'll use in our tests
			_, deleteTestKey, err := sqlStore.CreateAPIKey(ctx, store.KeyTargetOrganization, "org-to-delete", &store.APIKeyCreate{
				Name: "key-to-delete-test",
			})
			assert.NoError(t, err)

			tests := []struct {
				name       string
				targetType store.KeyTargetType
				targetID   string
				keyIDs     []string
				verify     bool // Whether to verify deletion by listing
			}{
				{
					name:       "delete existing key",
					targetType: store.KeyTargetOrganization,
					targetID:   "org-to-delete",
					keyIDs:     []string{deleteTestKey.ID},
					verify:     true,
				},
				{
					name:       "delete non-existent key",
					targetType: store.KeyTargetOrganization,
					targetID:   "org-to-delete",
					keyIDs:     []string{"non-existent-id"},
					verify:     false,
				},
				{
					name:       "delete with empty slice",
					targetType: store.KeyTargetOrganization,
					targetID:   "org-to-delete",
					keyIDs:     []string{},
					verify:     false,
				},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					err := sqlStore.DeleteAPIKeys(ctx, tt.targetType, tt.targetID, tt.keyIDs)
					assert.NoError(t, err)

					if tt.verify {
						keys, err := sqlStore.ListAPIKeys(ctx, tt.targetType, tt.targetID)
						assert.NoError(t, err)

						// Verify the key was deleted
						for _, keyID := range tt.keyIDs {
							for _, key := range keys {
								assert.NotEqual(t, keyID, key.ID, "Key should have been deleted but is still present")
							}
						}
					}
				})
			}
		})
	})
}

func setupSQLStore(ctx context.Context, t *testing.T) *sqlAuthStore {
	// launch postgres container with testcontainers (TODO abstract this with a helper)
	postgresContainer, err := postgres.Run(ctx,
		"postgres:16-alpine", // TODO parametrize version
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second)),
	)
	if err != nil {
		t.Fatalf("failed to start container: %s", err)
	}

	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(postgresContainer); err != nil {
			log.Printf("failed to terminate container: %s", err)
		}
	})

	// create a new SQL sqlStore
	config, err := ConfigFromConnectionString(postgresContainer.MustConnectionString(ctx, "sslmode=disable"))
	assert.NoError(t, err)
	sqlStore, err := NewSQLAuthStore(ctx, config)
	if err != nil {
		t.Fatalf("failed to create store: %s", err)
	}
	t.Cleanup(func() {
		if err := sqlStore.Close(ctx); err != nil {
			log.Printf("failed to close store: %s", err)
		}
	})

	// run migrations
	err = sqlStore.Setup(ctx)
	assert.NoError(t, err)

	return sqlStore
}
