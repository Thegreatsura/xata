package openfeature

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"xata/internal/token"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

func TestXataClaimsToEvaluationContext(t *testing.T) {
	tests := []struct {
		name                 string
		claims               *token.Claims
		organizationID       string
		expectedTargetingKey string
		expectedEmail        string
		expectedGroups       map[string]string
	}{
		{
			name:                 "User token with UserID",
			claims:               &token.Claims{ID: "user123", Email: "user@example.com"},
			organizationID:       "org456",
			expectedTargetingKey: "user123",
			expectedEmail:        "user@example.com",
			expectedGroups:       map[string]string{"organization": "org456"},
		},
		{
			name:                 "Organization API key without UserID",
			claims:               &token.Claims{KeyID: "apikey789", Email: ""},
			organizationID:       "org456",
			expectedTargetingKey: "apikey789",
			expectedEmail:        "",
			expectedGroups:       map[string]string{"organization": "org456"},
		},
		{
			name:                 "Organization API key without UserID and no org in URL",
			claims:               &token.Claims{KeyID: "apikey789", Email: ""},
			organizationID:       "",
			expectedTargetingKey: "apikey789",
			expectedEmail:        "",
			expectedGroups:       nil,
		},
		{
			name:                 "User token without organization in URL",
			claims:               &token.Claims{ID: "user123", Email: "user@example.com"},
			organizationID:       "",
			expectedTargetingKey: "user123",
			expectedEmail:        "user@example.com",
			expectedGroups:       nil,
		},
		{
			// This will error out in PostHog since distinct ID is required, but we still create the context
			name:                 "Empty claims fallback",
			claims:               &token.Claims{},
			organizationID:       "org456",
			expectedTargetingKey: "",
			expectedEmail:        "",
			expectedGroups:       map[string]string{"organization": "org456"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create echo context with organization parameter
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			if tt.organizationID != "" {
				c.SetParamNames("organizationID")
				c.SetParamValues(tt.organizationID)
			}

			// Call the function
			evalCtx := xataClaimsToEvaluationContext(c, tt.claims)

			// Verify targeting key (distinct ID for PostHog)
			require.Equal(t, tt.expectedTargetingKey, evalCtx.TargetingKey())

			// Verify email attribute
			require.Equal(t, tt.expectedEmail, evalCtx.Attributes()["email"])

			// Verify groups
			if tt.expectedGroups == nil {
				require.Nil(t, evalCtx.Attributes()["groups"])
			} else {
				require.Equal(t, tt.expectedGroups, evalCtx.Attributes()["groups"])
			}
		})
	}
}
