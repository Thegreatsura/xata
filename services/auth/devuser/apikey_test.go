package devuser

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"xata/internal/api/key"
	"xata/services/auth/store"
	"xata/services/auth/store/mocks"
)

func TestReplaceDevAPIKey(t *testing.T) {
	const userID = "user-1"

	tests := map[string]struct {
		existing []store.APIKey
		want     []string
	}{
		"no keys yet": {
			existing: nil,
			want:     nil,
		},
		"replaces the key it owns": {
			existing: []store.APIKey{{ID: "a", Name: DevAPIKeyName}},
			want:     []string{"a"},
		},
		"leaves keys it does not own": {
			existing: []store.APIKey{{ID: "a", Name: "personal"}, {ID: "b", Name: DevAPIKeyName}},
			want:     []string{"b"},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			authStore := mocks.NewAuthStore(t)
			authStore.EXPECT().ListAPIKeys(ctx, store.KeyTargetUser, userID).Return(test.existing, nil)
			if test.want != nil {
				authStore.EXPECT().DeleteAPIKeys(ctx, store.KeyTargetUser, userID, test.want).Return(nil)
			}
			authStore.EXPECT().
				CreateAPIKey(ctx, store.KeyTargetUser, userID, &store.APIKeyCreate{Name: DevAPIKeyName}).
				Return(key.Key("xau_test"), nil, nil)

			got, err := replaceDevAPIKey(ctx, authStore, userID)
			require.NoError(t, err)
			require.Equal(t, "xau_test", got)
		})
	}
}
