package extensions

import (
	"slices"
	"testing"
)

func TestYAMLIntegrity(t *testing.T) {
	// Test that all extensions have valid required fields
	offerings := GetAllOfferings()
	if len(offerings) == 0 {
		t.Fatal("No offerings loaded from YAML")
	}

	for _, offering := range offerings {
		versions := GetVersionsForOffering(offering)
		if len(versions) == 0 {
			t.Errorf("Offering %s has no versions", offering)
			continue
		}

		for _, version := range versions {
			image := offering + ":" + version
			t.Run(image, func(t *testing.T) {
				extensions := GetExtensions(image)
				if len(extensions) == 0 {
					t.Fatalf("No extensions loaded")
				}

				for _, ext := range extensions {
					if ext.Name == "" {
						t.Error("Extension has empty name")
					}
					if ext.Version == "" {
						t.Errorf("Extension %s has empty version", ext.Name)
					}
					if ext.Description == "" {
						t.Errorf("Extension %s has empty description", ext.Name)
					}
				}
			})
		}
	}
}

func TestYAMLConsistency(t *testing.T) {
	// Test that the data is internally consistent
	offerings := GetAllOfferings()

	for _, offering := range offerings {
		versions := GetVersionsForOffering(offering)

		for _, version := range versions {
			image := offering + ":" + version
			t.Run(image, func(t *testing.T) {
				extensions := GetExtensions(image)

				// Test that IsExtensionAvailable matches GetExtensions
				for _, ext := range extensions {
					if !IsExtensionAvailable(image, ext.Name) {
						t.Errorf("IsExtensionAvailable(%s) returned false but extension is in GetExtensions", ext.Name)
					}

					// Test that GetExtension returns the same data
					fetchedExt := GetExtension(image, ext.Name)
					if fetchedExt == nil {
						t.Errorf("GetExtension(%s) returned nil but extension is in GetExtensions", ext.Name)
						continue
					}

					if fetchedExt.Name != ext.Name {
						t.Errorf("GetExtension returned different name: expected %s, got %s",
							ext.Name, fetchedExt.Name)
					}
					if fetchedExt.Version != ext.Version {
						t.Errorf("GetExtension returned different version for %s: expected %s, got %s",
							ext.Name, ext.Version, fetchedExt.Version)
					}
				}

				// Test that GetPreloadRequiredExtensions is a subset of GetExtensions
				preloadExtensions := GetPreloadRequiredExtensions(image)
				for _, preloadExt := range preloadExtensions {
					found := false
					for _, ext := range extensions {
						if ext.Name == preloadExt.Name {
							found = true
							if !ext.PreloadRequired {
								t.Errorf("Extension %s is in GetPreloadRequiredExtensions but PreloadRequired is false",
									preloadExt.Name)
							}
							break
						}
					}
					if !found {
						t.Errorf("Extension %s from GetPreloadRequiredExtensions not found in GetExtensions",
							preloadExt.Name)
					}
				}
			})
		}
	}
}

func TestDefaultExtensionsProvider(t *testing.T) {
	// Test that DefaultExtensionsProvider implements ExtensionsProvider correctly
	var provider ExtensionsProvider = &DefaultExtensionsProvider{}

	t.Run("GetExtensions", func(t *testing.T) {
		testCases := []struct {
			name           string
			image          string
			expectMinCount int
		}{
			{"analytics:17", "analytics:17", 1},
			{"postgres:17", "postgres:17", 1},
			{"experimental:17", "experimental:17", 1},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				extensions := provider.GetExtensions(tc.image)
				if len(extensions) < tc.expectMinCount {
					t.Errorf("Expected at least %d extensions, got %d", tc.expectMinCount, len(extensions))
				}
			})
		}
	})

	t.Run("IsExtensionAvailable", func(t *testing.T) {
		testCases := []struct {
			name          string
			image         string
			extensionName string
			expected      bool
		}{
			{"pg_duckdb in analytics:17", "analytics:17", "pg_duckdb", true},
			{"pg_stat_statements in postgres:17", "postgres:17", "pg_stat_statements", true},
			{"nonexistent extension", "analytics:17", "nonexistent", false},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result := provider.IsExtensionAvailable(tc.image, tc.extensionName)
				if result != tc.expected {
					t.Errorf("Expected %v, got %v", tc.expected, result)
				}
			})
		}
	})

	t.Run("GetExtension", func(t *testing.T) {
		testCases := []struct {
			name          string
			image         string
			extensionName string
			expectNil     bool
		}{
			{"pg_duckdb in analytics:17", "analytics:17", "pg_duckdb", false},
			{"pg_stat_statements in postgres:17", "postgres:17", "pg_stat_statements", false},
			{"nonexistent extension", "analytics:17", "nonexistent", true},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				ext := provider.GetExtension(tc.image, tc.extensionName)
				if tc.expectNil && ext != nil {
					t.Error("Expected nil, got extension")
				}
				if !tc.expectNil && ext == nil {
					t.Error("Expected extension, got nil")
				}
			})
		}
	})

	t.Run("GetPreloadRequiredExtensions", func(t *testing.T) {
		testCases := []struct {
			name           string
			image          string
			expectMinCount int
		}{
			{"analytics:17", "analytics:17", 1},
			{"postgres:17", "postgres:17", 1},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				preloadExts := provider.GetPreloadRequiredExtensions(tc.image)
				if len(preloadExts) < tc.expectMinCount {
					t.Errorf("Expected at least %d extensions, got %d", tc.expectMinCount, len(preloadExts))
				}
			})
		}
	})

	t.Run("GetAllOfferings", func(t *testing.T) {
		offerings := provider.GetAllOfferings()
		if len(offerings) == 0 {
			t.Error("GetAllOfferings returned empty list")
		}

		expectedOfferings := []string{"analytics", "postgres", "experimental"}
		for _, expected := range expectedOfferings {
			found := slices.Contains(offerings, expected)
			if !found {
				t.Errorf("%s should be in offerings list", expected)
			}
		}
	})

	t.Run("GetVersionsForOffering", func(t *testing.T) {
		testCases := []struct {
			name        string
			offering    string
			mustInclude []string
		}{
			{"analytics", "analytics", []string{"17"}},
			{"postgres", "postgres", []string{"14", "15", "16", "17"}},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				versions := provider.GetVersionsForOffering(tc.offering)
				if len(versions) == 0 {
					t.Fatal("GetVersionsForOffering returned empty list")
				}

				for _, expected := range tc.mustInclude {
					found := slices.Contains(versions, expected)
					if !found {
						t.Errorf("Version %s should be in list for %s", expected, tc.offering)
					}
				}
			})
		}
	})
}

func TestExtensionNamesUnique(t *testing.T) {
	// Test that extension names are unique within each offering/version
	offerings := GetAllOfferings()

	for _, offering := range offerings {
		versions := GetVersionsForOffering(offering)

		for _, version := range versions {
			image := offering + ":" + version
			t.Run(image, func(t *testing.T) {
				extensions := GetExtensions(image)
				seen := make(map[string]bool)

				for _, ext := range extensions {
					if seen[ext.Name] {
						t.Errorf("Duplicate extension name: %s", ext.Name)
					}
					seen[ext.Name] = true
				}
			})
		}
	}
}
