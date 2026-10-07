package validation

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"

	"github.com/getkin/kin-openapi/openapi3"
)

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
