package extensions

import (
	"embed"
	"fmt"
	"log"
	"strings"

	"xata/internal/postgresversions"

	"gopkg.in/yaml.v3"
)

//go:embed extensions.yaml
var extensionsFS embed.FS

// ExtensionSpec represents a PostgreSQL extension available in an image
type ExtensionSpec struct {
	Name            string `yaml:"name"`
	Version         string `yaml:"version"`
	Description     string `yaml:"description"`
	DocsURL         string `yaml:"docs_url"`
	PreloadRequired bool   `yaml:"preload_required"`
	Type            string `yaml:"type"`
}

// MajorVersionExtensions represents extensions available for a specific major version
type MajorVersionExtensions struct {
	Extensions []ExtensionSpec `yaml:"extensions"`
}

// ImageExtensions represents the structure of the extensions config
type ImageExtensions struct {
	LastUpdated string                                       `yaml:"last_updated"`
	UpdatedBy   string                                       `yaml:"updated_by,omitempty"`
	Offerings   map[string]map[string]MajorVersionExtensions `yaml:"offerings"`
}

var imageExtensions *ImageExtensions

// init loads the extensions from the embedded YAML file
func init() {
	if err := loadExtensions(); err != nil {
		log.Fatalf("Critical: failed to load PostgreSQL extensions configuration: %v", err)
	}
}

// loadExtensions loads the extensions from the embedded YAML file
func loadExtensions() error {
	yamlData, err := extensionsFS.ReadFile("extensions.yaml")
	if err != nil {
		return fmt.Errorf("failed to read embedded YAML file: %w", err)
	}

	if len(yamlData) == 0 {
		return fmt.Errorf("extensions.yaml file is empty")
	}

	var extensions ImageExtensions
	if err := yaml.Unmarshal(yamlData, &extensions); err != nil {
		return fmt.Errorf("failed to unmarshal YAML: %w", err)
	}

	if len(extensions.Offerings) == 0 {
		return fmt.Errorf("no offerings found in extensions.yaml")
	}

	imageExtensions = &extensions
	return nil
}

// getExtensionsByOfferingVersion returns extensions for a specific offering and major version
func getExtensionsByOfferingVersion(offering string, majorVersion string) []ExtensionSpec {
	if imageExtensions == nil {
		return nil
	}

	offeringVersions, ok := imageExtensions.Offerings[offering]
	if !ok {
		return nil
	}

	versionExtensions, ok := offeringVersions[majorVersion]
	if !ok {
		return nil
	}

	result := make([]ExtensionSpec, len(versionExtensions.Extensions))
	for i, ext := range versionExtensions.Extensions {
		result[i] = ext
		if result[i].Type == "" {
			result[i].Type = "extension"
		}
	}
	return result
}

// GetExtensions returns all extensions available for a specific image (e.g., "analytics:17.7")
func GetExtensions(image string) []ExtensionSpec {
	parts := strings.Split(image, ":")
	if len(parts) != 2 {
		return nil
	}

	offering := parts[0]
	majorVersion := postgresversions.GetMajorForVersion(parts[1])

	return getExtensionsByOfferingVersion(offering, majorVersion)
}

// IsExtensionAvailable checks if a specific extension is available for an image
func IsExtensionAvailable(image string, extensionName string) bool {
	extensions := GetExtensions(image)
	for _, ext := range extensions {
		if ext.Name == extensionName {
			return true
		}
	}
	return false
}

// GetExtension returns the spec for a specific extension and image, or nil if not found
func GetExtension(image string, extensionName string) *ExtensionSpec {
	extensions := GetExtensions(image)
	for _, ext := range extensions {
		if ext.Name == extensionName {
			return &ext
		}
	}
	return nil
}

// GetPreloadRequiredExtensions returns all extensions that require shared_preload_libraries for a given image
func GetPreloadRequiredExtensions(image string) []ExtensionSpec {
	extensions := GetExtensions(image)
	var preloadRequired []ExtensionSpec
	for _, ext := range extensions {
		if ext.PreloadRequired {
			preloadRequired = append(preloadRequired, ext)
		}
	}
	return preloadRequired
}

// GetAllOfferings returns all available offering names
func GetAllOfferings() []string {
	if imageExtensions == nil {
		return nil
	}

	offerings := make([]string, 0, len(imageExtensions.Offerings))
	for offering := range imageExtensions.Offerings {
		offerings = append(offerings, offering)
	}
	return offerings
}

// GetVersionsForOffering returns all major versions available for an offering
func GetVersionsForOffering(offering string) []string {
	if imageExtensions == nil {
		return nil
	}

	offeringVersions, ok := imageExtensions.Offerings[offering]
	if !ok {
		return nil
	}

	versions := make([]string, 0, len(offeringVersions))
	for version := range offeringVersions {
		versions = append(versions, version)
	}
	return versions
}
