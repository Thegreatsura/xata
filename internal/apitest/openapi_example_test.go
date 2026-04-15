package apitest_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"xata/internal/apitest"
	"xata/internal/apitest/validation"
)

// Example_basicValidation demonstrates automatic OpenAPI validation in tests.
// This example shows how OpenAPI validation happens automatically.
func Example_basicValidation() {
	// This is a simplified example showing the validation pattern.
	// In real tests, you would have actual handler implementations.

	t := &testing.T{} // In real tests, this comes from the test function

	// Load the OpenAPI spec for your service
	spec, err := validation.LoadAuthSpec()
	require.NoError(t, err)

	// Configure apitest with the OpenAPI spec
	// Validation will now happen automatically on every request
	e := apitest.New(t).
		WithOpenAPISpec(spec).
		WithClaims(apitest.TestClaims)

	c, rec := e.POST("/organizations").
		WithJSONBody(map[string]string{"name": "test-org"}).
		Context()

	// Call your handler (example - replace with actual handler)
	// err = handler.CreateOrganization(c)
	_ = c // use the context

	// Validation happens automatically when you call MustCode!
	// No need to call rec.MustValidateAgainstOpenAPI(spec)
	rec.MustCode(http.StatusCreated)

	// Continue with your assertions
	var resp map[string]string
	rec.ReadBody(&resp)
	// assert.Equal(t, "test-org", resp["name"])
}

// Example_manualValidation shows how to manually validate if needed.
func Example_manualValidation() {
	t := &testing.T{}

	spec, err := validation.LoadAuthSpec()
	require.NoError(t, err)

	// Don't configure spec in apitest - validation is manual
	e := apitest.New(t).WithClaims(apitest.TestClaims)
	c, rec := e.POST("/organizations").
		WithJSONBody(map[string]string{"name": "test-org"}).
		Context()

	_ = c

	rec.MustCode(http.StatusCreated)

	// Manually validate when you want
	rec.MustValidateAgainstOpenAPI(spec)
}

// Example_tableTestValidation shows how to integrate automatic OpenAPI validation
// into table-driven tests.
func Example_tableTestValidation() {
	t := &testing.T{}

	spec, err := validation.LoadAuthSpec()
	require.NoError(t, err)

	tests := map[string]struct {
		name           string
		jsonBody       any
		expectedStatus int
		skipValidation bool // Use this flag to skip validation for specific test cases
	}{
		"create organization succeeds": {
			jsonBody:       map[string]string{"name": "new-org"},
			expectedStatus: http.StatusCreated,
			skipValidation: false,
		},
		"create organization with invalid name fails": {
			jsonBody:       map[string]string{"name": ""},
			expectedStatus: http.StatusBadRequest,
			skipValidation: false,
		},
		"test case that intentionally violates schema": {
			jsonBody:       map[string]int{"invalid": 123},
			expectedStatus: http.StatusBadRequest,
			skipValidation: true, // Skip validation for this test
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			// Configure with OpenAPI spec for automatic validation
			e := apitest.New(t).
				WithOpenAPISpec(spec).
				WithClaims(apitest.TestClaims)

			// Build request, optionally skipping validation for specific cases
			req := e.POST("/organizations").WithJSONBody(tt.jsonBody)
			if tt.skipValidation {
				req = req.SkipOpenAPIValidation()
			}

			c, rec := req.Context()

			// Call handler
			// err := handler.CreateOrganization(c)
			_ = c

			// Validation happens automatically when you call MustCode
			// (unless skipValidation was set)
			rec.MustCode(tt.expectedStatus)
		})
	}
}

// Example_advancedValidation demonstrates more advanced validation scenarios.
func Example_advancedValidation() {
	t := &testing.T{}

	spec, err := validation.LoadAuthSpec()
	require.NoError(t, err)

	// Create a validator for more control
	validator, err := validation.NewValidator(spec)
	require.NoError(t, err)

	// Example: Validate just a request body without making a full HTTP request
	requestBody := map[string]string{"name": "test-org"}
	err = validator.ValidateRequestBody("POST", "/organizations", requestBody)
	require.NoError(t, err)

	// Example: Get all available operations in the spec
	operations := validator.GetOperationPaths()
	_ = operations // Use for documentation or test discovery

	// Example: Validate just a response body
	responseBody := map[string]any{
		"id":     "org123",
		"name":   "test-org",
		"status": "enabled",
	}
	err = validator.ValidateResponseBody("POST", "/organizations", http.StatusCreated, responseBody)
	require.NoError(t, err)
}
