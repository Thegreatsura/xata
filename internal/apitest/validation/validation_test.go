package validation

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadSpec(t *testing.T) {
	tests := map[string]struct {
		loadFunc func() (*openapi3.T, error)
		wantErr  bool
	}{
		"load auth spec succeeds": {
			loadFunc: LoadAuthSpec,
			wantErr:  false,
		},
		"load projects spec succeeds": {
			loadFunc: LoadProjectsSpec,
			wantErr:  false,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			spec, err := tt.loadFunc()
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, spec)
			assert.NotNil(t, spec.Paths)
		})
	}
}

func TestLoadSpec_Caching(t *testing.T) {
	// Clear cache before test
	ClearCache()

	// Load spec first time
	spec1, err := LoadAuthSpec()
	require.NoError(t, err)

	// Load spec second time - should come from cache
	spec2, err := LoadAuthSpec()
	require.NoError(t, err)

	// Should be the same pointer (cached)
	assert.Same(t, spec1, spec2)
}

func TestValidator_ValidateRequest(t *testing.T) {
	spec, err := LoadAuthSpec()
	require.NoError(t, err)

	validator, err := NewValidator(spec)
	require.NoError(t, err)

	tests := map[string]struct {
		method      string
		path        string
		body        string
		contentType string
		wantErr     bool
	}{
		"valid POST request with body": {
			method:      "POST",
			path:        "/organizations",
			body:        `{"name":"test-org"}`,
			contentType: "application/json",
			wantErr:     false,
		},
		"valid GET request without body": {
			method:      "GET",
			path:        "/organizations",
			body:        "",
			contentType: "",
			wantErr:     false,
		},
		"invalid request - missing required field": {
			method:      "POST",
			path:        "/organizations",
			body:        `{}`,
			contentType: "application/json",
			wantErr:     true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var body io.Reader
			if tt.body != "" {
				body = bytes.NewBufferString(tt.body)
			}

			req := httptest.NewRequest(tt.method, tt.path, body)
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}

			err := validator.ValidateRequest(req)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_ValidateResponse(t *testing.T) {
	spec, err := LoadAuthSpec()
	require.NoError(t, err)

	validator, err := NewValidator(spec)
	require.NoError(t, err)

	tests := map[string]struct {
		method     string
		path       string
		statusCode int
		body       string
		wantErr    bool
	}{
		"valid response with correct schema": {
			method:     "GET",
			path:       "/organizations",
			statusCode: 200,
			body:       `{"organizations":[]}`,
			wantErr:    false,
		},
		"valid error response": {
			method:     "POST",
			path:       "/organizations",
			statusCode: 400,
			body:       `{"message":"bad request"}`,
			wantErr:    false,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)

			resp := &http.Response{
				StatusCode: tt.statusCode,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(bytes.NewBufferString(tt.body)),
			}

			err := validator.ValidateResponse(req, resp)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_GetOperationPaths(t *testing.T) {
	spec, err := LoadAuthSpec()
	require.NoError(t, err)

	validator, err := NewValidator(spec)
	require.NoError(t, err)

	paths := validator.GetOperationPaths()
	assert.NotEmpty(t, paths)

	// Check that at least some expected operations are present
	assert.Contains(t, paths, "GET /organizations")
	assert.Contains(t, paths, "POST /organizations")
}
