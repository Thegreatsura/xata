package postgrescfg

import (
	"reflect"
	"testing"
)

func TestMergeParametersMaps(t *testing.T) {
	tests := []struct {
		name     string
		instance string
		wantErr  bool
	}{
		{
			name:     "xata.micro",
			instance: "xata.micro",
			wantErr:  false,
		},
		{
			name:     "xata.small",
			instance: "xata.small",
			wantErr:  false,
		},
		{
			name:     "xata.medium",
			instance: "xata.medium",
			wantErr:  false,
		},
		{
			name:     "xata.large",
			instance: "xata.large",
			wantErr:  false,
		},
		{
			name:     "xata.xlarge",
			instance: "xata.xlarge",
			wantErr:  false,
		},
		{
			name:     "xata.2xlarge",
			instance: "xata.2xlarge",
			wantErr:  false,
		},
		{
			name:     "xata.4xlarge",
			instance: "xata.4xlarge",
			wantErr:  false,
		},
		{
			name:     "xata.8xlarge",
			instance: "xata.8xlarge",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Get the instance-specific parameters
			instanceParams, err := GetDefaultPostgresConfigByInstanceType(tt.instance)
			if tt.wantErr {
				if err == nil {
					t.Errorf("GetDefaultPostgresConfigByInstanceType() expected error but got none")
				}
				return
			}
			if err != nil {
				t.Errorf("GetDefaultPostgresConfigByInstanceType() error = %v", err)
				return
			}

			baseParams := GetConfigurableParameters(0, "", nil)
			merged := MergeParametersMaps(baseParams, instanceParams)
			verifyMerge(t, baseParams, instanceParams, merged, tt.instance)
		})
	}
}

func TestMergeParametersMaps_EmptyOverride(t *testing.T) {
	baseParams := GetConfigurableParameters(0, "", nil)
	emptyOverride := ParametersMap{}

	merged := MergeParametersMaps(baseParams, emptyOverride)

	// Should return a copy of base params with same values
	// Check a few key parameters to verify they're the same
	keyParams := []string{"max_connections", "shared_buffers", "work_mem"}
	for _, param := range keyParams {
		if merged[param].DefaultValue != baseParams[param].DefaultValue {
			t.Errorf("Parameter '%s' DefaultValue should be the same: expected '%s', got '%s'",
				param, baseParams[param].DefaultValue, merged[param].DefaultValue)
		}
		if merged[param].Description != baseParams[param].Description {
			t.Errorf("Parameter '%s' Description should be the same: expected '%s', got '%s'",
				param, baseParams[param].Description, merged[param].Description)
		}
	}

	// Should be a different map (not the same reference)
	// Verify that modifying the merged map doesn't affect the original
	originalMaxConnections := baseParams["max_connections"].DefaultValue
	merged["max_connections"] = PostgresParameterSpec{DefaultValue: "999999"}
	if baseParams["max_connections"].DefaultValue != originalMaxConnections {
		t.Errorf("MergeParametersMaps() should return a new map, not the same reference")
	}
}

func TestMergeParametersMaps_PartialOverride(t *testing.T) {
	baseParams := GetConfigurableParameters(0, "", nil)

	// Create a partial override with just a few parameters
	partialOverride := ParametersMap{
		"max_connections": {
			ParameterType: ParamTypeInt,
			Description:   "Custom description",
			DefaultValue:  "150",
			MinValue:      "0",
			MaxValue:      "1000",
		},
		"shared_buffers": {
			ParameterType: ParamTypeBytes,
			DefaultValue:  "1GB",
		},
	}

	merged := MergeParametersMaps(baseParams, partialOverride)

	// Verify that overridden parameters are updated
	if merged["max_connections"].DefaultValue != "150" {
		t.Errorf("Expected max_connections DefaultValue to be '150', got '%s'", merged["max_connections"].DefaultValue)
	}
	if merged["max_connections"].Description != "Custom description" {
		t.Errorf("Expected max_connections Description to be 'Custom description', got '%s'", merged["max_connections"].Description)
	}
	if merged["shared_buffers"].DefaultValue != "1GB" {
		t.Errorf("Expected shared_buffers DefaultValue to be '1GB', got '%s'", merged["shared_buffers"].DefaultValue)
	}

	// Verify that non-overridden parameters remain unchanged
	if merged["work_mem"].DefaultValue != baseParams["work_mem"].DefaultValue {
		t.Errorf("Expected work_mem to remain unchanged, got '%s' instead of '%s'",
			merged["work_mem"].DefaultValue, baseParams["work_mem"].DefaultValue)
	}
}

func TestMergeParametersMaps_FieldPreservation(t *testing.T) {
	baseParams := GetConfigurableParameters(0, "", nil)

	// Create override with only some fields set
	override := ParametersMap{
		"max_connections": {
			DefaultValue: "200",
			// Note: Description, MinValue, MaxValue are empty
		},
	}

	merged := MergeParametersMaps(baseParams, override)

	// Verify that only DefaultValue was updated
	if merged["max_connections"].DefaultValue != "200" {
		t.Errorf("Expected DefaultValue to be updated to '200', got '%s'", merged["max_connections"].DefaultValue)
	}

	// Verify other fields from base are preserved
	if merged["max_connections"].Description != baseParams["max_connections"].Description {
		t.Errorf("Expected Description to be preserved, got '%s' instead of '%s'",
			merged["max_connections"].Description, baseParams["max_connections"].Description)
	}
	if merged["max_connections"].MinValue != baseParams["max_connections"].MinValue {
		t.Errorf("Expected MinValue to be preserved, got '%s' instead of '%s'",
			merged["max_connections"].MinValue, baseParams["max_connections"].MinValue)
	}
	if merged["max_connections"].MaxValue != baseParams["max_connections"].MaxValue {
		t.Errorf("Expected MaxValue to be preserved, got '%s' instead of '%s'",
			merged["max_connections"].MaxValue, baseParams["max_connections"].MaxValue)
	}
}

func TestMergeParametersMaps_ValuesArray(t *testing.T) {
	baseParams := GetConfigurableParameters(0, "", nil)

	// Create override with Values array
	override := ParametersMap{
		"huge_pages": {
			DefaultValue: "on",
			Values:       []string{"on", "off", "try", "custom"},
		},
	}

	merged := MergeParametersMaps(baseParams, override)

	// Verify Values array is updated
	expectedValues := []string{"on", "off", "try", "custom"}
	if !reflect.DeepEqual(merged["huge_pages"].Values, expectedValues) {
		t.Errorf("Expected Values to be %v, got %v", expectedValues, merged["huge_pages"].Values)
	}

	// Verify DefaultValue is also updated
	if merged["huge_pages"].DefaultValue != "on" {
		t.Errorf("Expected DefaultValue to be 'on', got '%s'", merged["huge_pages"].DefaultValue)
	}
}

func verifyMerge(t *testing.T, base, override, merged ParametersMap, instanceName string) {
	// Verify all base parameters are present in merged result
	for paramName, baseSpec := range base {
		mergedSpec, exists := merged[paramName]
		if !exists {
			t.Errorf("Parameter '%s' from base is missing in merged result", paramName)
			continue
		}

		// Check if this parameter has an override
		overrideSpec, hasOverride := override[paramName]
		if hasOverride {
			// Verify override values are applied
			if overrideSpec.DefaultValue != "" && mergedSpec.DefaultValue != overrideSpec.DefaultValue {
				t.Errorf("Parameter '%s' DefaultValue not overridden correctly: expected '%s', got '%s'",
					paramName, overrideSpec.DefaultValue, mergedSpec.DefaultValue)
			}
			if overrideSpec.MinValue != "" && mergedSpec.MinValue != overrideSpec.MinValue {
				t.Errorf("Parameter '%s' MinValue not overridden correctly: expected '%s', got '%s'",
					paramName, overrideSpec.MinValue, mergedSpec.MinValue)
			}
			if overrideSpec.MaxValue != "" && mergedSpec.MaxValue != overrideSpec.MaxValue {
				t.Errorf("Parameter '%s' MaxValue not overridden correctly: expected '%s', got '%s'",
					paramName, overrideSpec.MaxValue, mergedSpec.MaxValue)
			}
			if overrideSpec.Description != "" && mergedSpec.Description != overrideSpec.Description {
				t.Errorf("Parameter '%s' Description not overridden correctly: expected '%s', got '%s'",
					paramName, overrideSpec.Description, mergedSpec.Description)
			}
			if len(overrideSpec.Values) > 0 && !reflect.DeepEqual(mergedSpec.Values, overrideSpec.Values) {
				t.Errorf("Parameter '%s' Values not overridden correctly: expected %v, got %v",
					paramName, overrideSpec.Values, mergedSpec.Values)
			}
		} else {
			// Verify base values are preserved for non-overridden parameters
			if mergedSpec.DefaultValue != baseSpec.DefaultValue {
				t.Errorf("Parameter '%s' DefaultValue should be preserved: expected '%s', got '%s'",
					paramName, baseSpec.DefaultValue, mergedSpec.DefaultValue)
			}
			if mergedSpec.MinValue != baseSpec.MinValue {
				t.Errorf("Parameter '%s' MinValue should be preserved: expected '%s', got '%s'",
					paramName, baseSpec.MinValue, mergedSpec.MinValue)
			}
			if mergedSpec.MaxValue != baseSpec.MaxValue {
				t.Errorf("Parameter '%s' MaxValue should be preserved: expected '%s', got '%s'",
					paramName, baseSpec.MaxValue, mergedSpec.MaxValue)
			}
			if mergedSpec.Description != baseSpec.Description {
				t.Errorf("Parameter '%s' Description should be preserved: expected '%s', got '%s'",
					paramName, baseSpec.Description, mergedSpec.Description)
			}
			if !reflect.DeepEqual(mergedSpec.Values, baseSpec.Values) {
				t.Errorf("Parameter '%s' Values should be preserved: expected %v, got %v",
					paramName, baseSpec.Values, mergedSpec.Values)
			}
		}

		// Verify ParameterType is preserved from base
		if mergedSpec.ParameterType != baseSpec.ParameterType {
			t.Errorf("Parameter '%s' ParameterType should be preserved: expected %v, got %v",
				paramName, baseSpec.ParameterType, mergedSpec.ParameterType)
		}
	}

	// Verify no extra parameters in merged result
	for paramName := range merged {
		if _, exists := base[paramName]; !exists {
			t.Errorf("Unexpected parameter '%s' in merged result", paramName)
		}
	}

	// Log some key parameters for verification
	t.Logf("Instance %s - max_connections: %s, shared_buffers: %s, work_mem: %s",
		instanceName,
		merged["max_connections"].DefaultValue,
		merged["shared_buffers"].DefaultValue,
		merged["work_mem"].DefaultValue)
}

func TestDetermineConfigValueType(t *testing.T) {
	tests := []struct {
		name         string
		instanceSize string
		paramName    string
		paramValue   string
		expected     ConfigValueType
		expectError  bool
	}{
		{
			name:         "PostgreSQL default value",
			instanceSize: "xata.small",
			paramName:    "work_mem",
			paramValue:   "1MB", // PostgreSQL default (different from xata.small's 2427kB)
			expected:     ConfigValueDefault,
		},
		{
			name:         "Instance-specific default value",
			instanceSize: "xata.small",
			paramName:    "max_connections",
			paramValue:   "100", // xata.small default
			expected:     ConfigValueInstanceDefault,
		},
		{
			name:         "Custom value",
			instanceSize: "xata.small",
			paramName:    "max_connections",
			paramValue:   "150", // Custom value
			expected:     ConfigValueCustom,
		},
		{
			name:         "Unknown parameter with instance default",
			instanceSize: "xata.small",
			paramName:    "unknown_parameter",
			paramValue:   "512MB", // Some value
			expected:     ConfigValueCustom,
			expectError:  true,
		},
		{
			name:         "Unknown parameter with custom value",
			instanceSize: "xata.small",
			paramName:    "another_unknown_param",
			paramValue:   "1gb", // Some value
			expected:     ConfigValueCustom,
			expectError:  true,
		},
		{
			name:         "Unknown instance size",
			instanceSize: "xata.unknown",
			paramName:    "max_connections",
			paramValue:   "100",
			expected:     ConfigValueCustom,
			expectError:  true,
		},
		{
			name:         "PostgreSQL default for work_mem",
			instanceSize: "xata.small",
			paramName:    "work_mem",
			paramValue:   "1MB", // PostgreSQL default
			expected:     ConfigValueDefault,
		},
		{
			name:         "Instance default for work_mem on xata.small",
			instanceSize: "xata.small",
			paramName:    "work_mem",
			paramValue:   "2427kB", // xata.small default
			expected:     ConfigValueInstanceDefault,
		},
		{
			name:         "Known parameter but not in instance config",
			instanceSize: "xata.small",
			paramName:    "temp_buffers", // Known parameter but not in xata.small config
			paramValue:   "8MB",          // PostgreSQL default
			expected:     ConfigValueDefault,
		},
		{
			name:         "Known parameter with custom value",
			instanceSize: "xata.small",
			paramName:    "temp_buffers", // Known parameter but not in xata.small config
			paramValue:   "16MB",         // Custom value
			expected:     ConfigValueCustom,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := DetermineConfigValueType(tt.instanceSize, tt.paramName, tt.paramValue, 0, "", nil)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}
