package validation

import (
	"bytes"
	"context"
	"encoding/json"
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

// ValidateRequestBody validates just the request body against the schema for a specific operation.
// This is useful when you want to validate a request body without having a full HTTP request.
func (v *Validator) ValidateRequestBody(method, path string, body any) error {
	// Find the operation
	pathItem := v.spec.Paths.Find(path)
	if pathItem == nil {
		return fmt.Errorf("path not found: %s", path)
	}

	operation := pathItem.GetOperation(method)
	if operation == nil {
		return fmt.Errorf("operation not found: %s %s", method, path)
	}

	// Check if operation has request body
	if operation.RequestBody == nil {
		return fmt.Errorf("operation %s %s does not accept a request body", method, path)
	}

	// Get the JSON schema
	mediaType := operation.RequestBody.Value.Content.Get("application/json")
	if mediaType == nil {
		return fmt.Errorf("operation %s %s does not accept application/json", method, path)
	}

	if mediaType.Schema == nil {
		return fmt.Errorf("no schema defined for request body: %s %s", method, path)
	}

	// Convert body to JSON if it's not already a string/[]byte
	var jsonData any
	switch v := body.(type) {
	case string:
		if err := json.Unmarshal([]byte(v), &jsonData); err != nil {
			return fmt.Errorf("unmarshal request body: %w", err)
		}
	case []byte:
		if err := json.Unmarshal(v, &jsonData); err != nil {
			return fmt.Errorf("unmarshal request body: %w", err)
		}
	default:
		jsonData = body
	}

	// Validate against schema
	if err := mediaType.Schema.Value.VisitJSON(jsonData); err != nil {
		return fmt.Errorf("validate request body schema: %w", err)
	}

	return nil
}

// ValidateResponseBody validates just the response body against the schema for a specific operation and status code.
// This is useful when you want to validate a response body without having a full HTTP response.
func (v *Validator) ValidateResponseBody(method, path string, statusCode int, body any) error {
	// Find the operation
	pathItem := v.spec.Paths.Find(path)
	if pathItem == nil {
		return fmt.Errorf("path not found: %s", path)
	}

	operation := pathItem.GetOperation(method)
	if operation == nil {
		return fmt.Errorf("operation not found: %s %s", method, path)
	}

	// Get the response for the status code
	statusStr := fmt.Sprintf("%d", statusCode)
	response := operation.Responses.Status(statusCode)
	if response == nil {
		// Try default response
		response = operation.Responses.Default()
		if response == nil {
			return fmt.Errorf("no response defined for status %d in %s %s", statusCode, method, path)
		}
	}

	// Get the JSON schema
	mediaType := response.Value.Content.Get("application/json")
	if mediaType == nil {
		// If no content is defined, the response should be empty
		return nil
	}

	if mediaType.Schema == nil {
		return fmt.Errorf("no schema defined for response %s: %s %s", statusStr, method, path)
	}

	// Convert body to JSON if it's not already a string/[]byte
	var jsonData any
	switch v := body.(type) {
	case string:
		if err := json.Unmarshal([]byte(v), &jsonData); err != nil {
			return fmt.Errorf("unmarshal response body: %w", err)
		}
	case []byte:
		if err := json.Unmarshal(v, &jsonData); err != nil {
			return fmt.Errorf("unmarshal response body: %w", err)
		}
	default:
		jsonData = body
	}

	// Validate against schema
	if err := mediaType.Schema.Value.VisitJSON(jsonData); err != nil {
		return fmt.Errorf("validate response body schema: %w", err)
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
