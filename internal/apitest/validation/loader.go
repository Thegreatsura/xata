package validation

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/getkin/kin-openapi/openapi3"
)

// SpecCache caches loaded OpenAPI specifications
type SpecCache struct {
	mu    sync.RWMutex
	specs map[string]*openapi3.T
}

// newSpecCache creates a new spec cache
func newSpecCache() *SpecCache {
	return &SpecCache{
		specs: make(map[string]*openapi3.T),
	}
}

var globalCache = newSpecCache()

// LoadSpec loads an OpenAPI specification from a file path.
// The spec is cached for subsequent calls.
func LoadSpec(specPath string) (*openapi3.T, error) {
	globalCache.mu.RLock()
	if spec, ok := globalCache.specs[specPath]; ok {
		globalCache.mu.RUnlock()
		return spec, nil
	}
	globalCache.mu.RUnlock()

	// Load the spec
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true

	spec, err := loader.LoadFromFile(specPath)
	if err != nil {
		return nil, fmt.Errorf("load spec from file %s: %w", specPath, err)
	}

	// Validate the spec itself
	if err := spec.Validate(loader.Context); err != nil {
		return nil, fmt.Errorf("validate spec %s: %w", specPath, err)
	}

	// Cache it
	globalCache.mu.Lock()
	globalCache.specs[specPath] = spec
	globalCache.mu.Unlock()

	return spec, nil
}

// LoadAuthSpec loads the auth service OpenAPI specification.
// It looks for the bundled spec in the expected location.
func LoadAuthSpec() (*openapi3.T, error) {
	return loadServiceSpec("auth")
}

// LoadProjectsSpec loads the projects service OpenAPI specification.
// It looks for the bundled spec in the expected location.
func LoadProjectsSpec() (*openapi3.T, error) {
	return loadServiceSpec("projects")
}

func loadServiceSpec(service string) (*openapi3.T, error) {
	// Try to find the spec file relative to the working directory
	// This handles both running from repo root and from subdirectories
	possiblePaths := []string{
		filepath.Join("openapi", "gen", "bundled", service+".yaml"),
		filepath.Join("..", "..", "..", "openapi", "gen", "bundled", service+".yaml"),
		filepath.Join("..", "..", "..", "..", "openapi", "gen", "bundled", service+".yaml"),
	}

	for _, specPath := range possiblePaths {
		if _, err := os.Stat(specPath); err == nil {
			return LoadSpec(specPath)
		}
	}

	return nil, fmt.Errorf("spec file not found for service '%s' (make sure to run 'make generate' first)", service)
}

// ClearCache clears the global spec cache.
// This is mainly useful for testing.
func ClearCache() {
	globalCache.mu.Lock()
	defer globalCache.mu.Unlock()
	globalCache.specs = make(map[string]*openapi3.T)
}
