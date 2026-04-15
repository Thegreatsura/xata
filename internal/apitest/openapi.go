package apitest

import (
	"github.com/getkin/kin-openapi/openapi3"

	"xata/internal/apitest/validation"
)

// ValidateAgainstOpenAPI validates both the request and response against an OpenAPI specification.
// This is the most complete validation, checking that both the request that was made and the
// response that was returned conform to the OpenAPI spec.
func (r *ResponseRecorder) ValidateAgainstOpenAPI(spec *openapi3.T) error {
	r.t.Helper()
	if r.req == nil {
		r.t.Fatal("cannot validate: request not set in ResponseRecorder")
		return nil
	}
	return validation.ValidateAgainstSpec(spec, r.req, r.ResponseRecorder, r.reqBodyBytes)
}

// MustValidateAgainstOpenAPI validates both request and response against an OpenAPI specification
// and fails the test if validation fails.
func (r *ResponseRecorder) MustValidateAgainstOpenAPI(spec *openapi3.T) {
	r.t.Helper()
	if err := r.ValidateAgainstOpenAPI(spec); err != nil {
		r.t.Fatalf("OpenAPI validation failed: %v", err)
	}
}

// ValidateResponseAgainstOpenAPI validates only the response against an OpenAPI specification.
// This is useful when you don't need to validate the request.
func (r *ResponseRecorder) ValidateResponseAgainstOpenAPI(spec *openapi3.T) error {
	r.t.Helper()
	if r.req == nil {
		r.t.Fatal("cannot validate: request not set in ResponseRecorder")
		return nil
	}
	return validation.ValidateResponseOnly(spec, r.req, r.ResponseRecorder)
}

// MustValidateResponseAgainstOpenAPI validates the response against an OpenAPI specification
// and fails the test if validation fails.
func (r *ResponseRecorder) MustValidateResponseAgainstOpenAPI(spec *openapi3.T) {
	r.t.Helper()
	if err := r.ValidateResponseAgainstOpenAPI(spec); err != nil {
		r.t.Fatalf("OpenAPI response validation failed: %v", err)
	}
}
