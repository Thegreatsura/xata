package validation

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"

	"github.com/getkin/kin-openapi/openapi3"
)

// ResponseValidator provides convenient methods to validate HTTP responses
// against OpenAPI specifications. It wraps the Validator to provide a simpler
// interface for test code.
type ResponseValidator struct {
	validator  *Validator
	statusCode int
	body       []byte
	headers    http.Header
}

// NewResponseValidator creates a new response validator from an httptest.ResponseRecorder
func NewResponseValidator(spec *openapi3.T, rec *httptest.ResponseRecorder) (*ResponseValidator, error) {
	validator, err := NewValidator(spec)
	if err != nil {
		return nil, fmt.Errorf("create validator: %w", err)
	}

	return &ResponseValidator{
		validator:  validator,
		statusCode: rec.Code,
		body:       rec.Body.Bytes(),
		headers:    rec.Header(),
	}, nil
}

// ValidateResponse validates the response against the OpenAPI spec for the given method and path.
// The original request is needed to match the route and validate the response in context.
func (rv *ResponseValidator) ValidateResponse(req *http.Request) error {
	// Create a response object
	resp := &http.Response{
		StatusCode: rv.statusCode,
		Header:     rv.headers,
		Body:       io.NopCloser(bytes.NewReader(rv.body)),
	}

	return rv.validator.ValidateResponse(req, resp)
}

// ValidateResponseBody validates only the response body against the schema for the given operation.
// This is a simpler validation that doesn't require the full request context.
func (rv *ResponseValidator) ValidateResponseBody(method, path string) error {
	return rv.validator.ValidateResponseBody(method, path, rv.statusCode, rv.body)
}

// ValidateAgainstSpec is a convenient helper that validates both request and response
// against the OpenAPI spec. This is the simplest way to add OpenAPI validation to tests.
// If reqBodyBytes is provided, it will be used to restore the request body for validation.
func ValidateAgainstSpec(spec *openapi3.T, req *http.Request, rec *httptest.ResponseRecorder, reqBodyBytes []byte) error {
	validator, err := NewValidator(spec)
	if err != nil {
		return fmt.Errorf("create validator: %w", err)
	}

	// Restore request body if we have the cached bytes
	if reqBodyBytes != nil && req.Body != nil {
		req.Body = io.NopCloser(bytes.NewReader(reqBodyBytes))
	}

	// Validate request
	if err := validator.ValidateRequest(req); err != nil {
		return fmt.Errorf("request validation: %w", err)
	}

	// Validate response
	resp := &http.Response{
		StatusCode: rec.Code,
		Header:     rec.Header(),
		Body:       io.NopCloser(bytes.NewReader(rec.Body.Bytes())),
	}

	if err := validator.ValidateResponse(req, resp); err != nil {
		return fmt.Errorf("response validation: %w", err)
	}

	return nil
}

// ValidateResponseOnly validates only the response against the OpenAPI spec.
// This is useful when you don't need to validate the request.
func ValidateResponseOnly(spec *openapi3.T, req *http.Request, rec *httptest.ResponseRecorder) error {
	validator, err := NewValidator(spec)
	if err != nil {
		return fmt.Errorf("create validator: %w", err)
	}

	resp := &http.Response{
		StatusCode: rec.Code,
		Header:     rec.Header(),
		Body:       io.NopCloser(bytes.NewReader(rec.Body.Bytes())),
	}

	if err := validator.ValidateResponse(req, resp); err != nil {
		return fmt.Errorf("response validation: %w", err)
	}

	return nil
}

// ValidateRequestOnly validates only the request against the OpenAPI spec.
// This is useful when you want to validate requests before they're processed.
func ValidateRequestOnly(spec *openapi3.T, req *http.Request) error {
	validator, err := NewValidator(spec)
	if err != nil {
		return fmt.Errorf("create validator: %w", err)
	}

	if err := validator.ValidateRequest(req); err != nil {
		return fmt.Errorf("request validation: %w", err)
	}

	return nil
}
