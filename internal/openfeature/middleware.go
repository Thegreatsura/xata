package openfeature

import (
	"xata/internal/api"
	"xata/internal/token"

	"github.com/labstack/echo/v4"
	"github.com/open-feature/go-sdk/openfeature"
)

// Middleware sets up the openfeature evaluation context for an incoming
// HTTP request by extracting user details from JWT claims.
func Middleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			claims := api.GetUserClaims(c)
			if claims == nil {
				return next(c)
			}

			ec := xataClaimsToEvaluationContext(c, claims)

			req := c.Request().WithContext(openfeature.WithTransactionContext(c.Request().Context(), ec))
			c.SetRequest(req)

			return next(c)
		}
	}
}

// xataClaimsToEvaluationContext converts a XataClaims object to an openfeature.EvaluationContext
func xataClaimsToEvaluationContext(c echo.Context, claims *token.Claims) openfeature.EvaluationContext {
	// Extract the organization ID from the request context if available
	organizationID := c.Param("organizationID")
	var groups map[string]string
	if organizationID != "" {
		groups = map[string]string{
			"organization": organizationID,
		}
	}

	// Use UserID as the distinct ID, fallback to API key ID for Organization API Keys.
	distinctID := claims.UserID()
	if distinctID == "" {
		distinctID = claims.APIKeyID()
	}

	return openfeature.NewEvaluationContext(distinctID, map[string]any{
		"email":  claims.UserEmail(),
		"groups": groups,
	})
}
