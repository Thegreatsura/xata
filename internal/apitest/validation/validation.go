package validation

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/gorillamux"
)

// Validator validates HTTP requests and responses against OpenAPI specifications
type Validator struct {
	spec   *openapi3.T
	router routers.Router
}

// NewValidator creates a new validator for the given OpenAPI spec
func NewValidator(spec *openapi3.T) (*Validator, error) {
	router, err := gorillamux.NewRouter(spec)
	if err != nil {
		return nil, fmt.Errorf("create router: %w", err)
	}

	return &Validator{
		spec:   spec,
		router: router,
	}, nil
}

// ValidateRequest validates an HTTP request against the OpenAPI spec.
// It checks:
// - Request body matches schema
// - Required parameters are present
// - Parameter types and formats are correct
// Note: Security validation is skipped by default for test convenience.
func (v *Validator) ValidateRequest(req *http.Request) error {
	// Find the route
	route, pathParams, err := v.router.FindRoute(req)
	if err != nil {
		return fmt.Errorf("find route: %w", err)
	}

	// Build validation input with security checks disabled for test convenience
	requestValidationInput := &openapi3filter.RequestValidationInput{
		Request:    req,
		PathParams: pathParams,
		Route:      route,
		Options: &openapi3filter.Options{
			ExcludeRequestBody:    false,
			ExcludeResponseBody:   false,
			IncludeResponseStatus: true,
			AuthenticationFunc: func(c context.Context, input *openapi3filter.AuthenticationInput) error {
				// Skip all authentication checks in tests
				return nil
			},
		},
	}

	// Clone the route to avoid mutating shared state (prevents race conditions)
	// This is necessary because we need to modify Security without affecting other goroutines
	routeCopy := *requestValidationInput.Route
	routeCopy.Operation = &openapi3.Operation{}
	*routeCopy.Operation = *requestValidationInput.Route.Operation
	routeCopy.Operation.Security = &openapi3.SecurityRequirements{}
	requestValidationInput.Route = &routeCopy

	// Validate request
	if err := openapi3filter.ValidateRequest(req.Context(), requestValidationInput); err != nil {
		return fmt.Errorf("validate request: %w", err)
	}

	return nil
}

// ValidateResponse validates an HTTP response against the OpenAPI spec.
// It checks:
// - Response status code is documented
// - Response body matches schema
// - Response headers match specification
// Note: Security validation is skipped by default for test convenience.
func (v *Validator) ValidateResponse(req *http.Request, resp *http.Response) error {
	// Find the route
	route, pathParams, err := v.router.FindRoute(req)
	if err != nil {
		return fmt.Errorf("find route: %w", err)
	}

	// Build validation input with security checks disabled for test convenience
	requestValidationInput := &openapi3filter.RequestValidationInput{
		Request:    req,
		PathParams: pathParams,
		Route:      route,
		Options: &openapi3filter.Options{
			ExcludeRequestBody:    false,
			ExcludeResponseBody:   false,
			IncludeResponseStatus: true,
			AuthenticationFunc: func(c context.Context, input *openapi3filter.AuthenticationInput) error {
				// Skip all authentication checks in tests
				return nil
			},
		},
	}

	// Clone the route to avoid mutating shared state (prevents race conditions)
	// This is necessary because we need to modify Security without affecting other goroutines
	routeCopy := *requestValidationInput.Route
	routeCopy.Operation = &openapi3.Operation{}
	*routeCopy.Operation = *requestValidationInput.Route.Operation
	routeCopy.Operation.Security = &openapi3.SecurityRequirements{}
	requestValidationInput.Route = &routeCopy

	// Build response validation input
	responseValidationInput := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: requestValidationInput,
		Status:                 resp.StatusCode,
		Header:                 resp.Header,
	}

	// Read response body
	var bodyBytes []byte
	if resp.Body != nil {
		bodyBytes, err = io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("read response body: %w", err)
		}
		resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	}
	responseValidationInput.SetBodyBytes(bodyBytes)

	// Validate response
	if err := openapi3filter.ValidateResponse(req.Context(), responseValidationInput); err != nil {
		return fmt.Errorf("validate response: %w", err)
	}

	return nil
}

// GetOperationPaths returns a list of all operation paths in the spec.
// This is useful for discovering available operations.
func (v *Validator) GetOperationPaths() []string {
	var paths []string
	for path, pathItem := range v.spec.Paths.Map() {
		for method := range pathItem.Operations() {
			paths = append(paths, fmt.Sprintf("%s %s", strings.ToUpper(method), path))
		}
	}
	return paths
}
