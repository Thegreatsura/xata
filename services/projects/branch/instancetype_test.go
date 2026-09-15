package branch

import (
	"context"
	"fmt"
	"testing"

	"xata/services/projects/store"
	mocks "xata/services/projects/store/mocks"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestParseCPUResource(t *testing.T) {
	tests := []struct {
		name     string
		cpuSpec  string
		expected int
	}{
		{
			name:     "250 millicores",
			cpuSpec:  "250m",
			expected: 250,
		},
		{
			name:     "500 millicores",
			cpuSpec:  "500m",
			expected: 500,
		},
		{
			name:     "1000 millicores",
			cpuSpec:  "1000m",
			expected: 1000,
		},
		{
			name:     "1 core",
			cpuSpec:  "1",
			expected: 1000,
		},
		{
			name:     "2 cores",
			cpuSpec:  "2",
			expected: 2000,
		},
		{
			name:     "2.5 cores",
			cpuSpec:  "2.5",
			expected: 2500,
		},
		{
			name:     "0.5 cores",
			cpuSpec:  "0.5",
			expected: 500,
		},
		{
			name:     "zero cores",
			cpuSpec:  "0",
			expected: 0,
		},
		{
			name:     "zero millicores",
			cpuSpec:  "0m",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseCPUResource(tt.cpuSpec)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Test round-trip conversion: milliCPUs -> format -> parse -> milliCPUs
func TestCPUResourceRoundTrip(t *testing.T) {
	testCases := []int{250, 500, 999, 1000, 2000, 3500}

	for _, milliCPUs := range testCases {
		t.Run(fmt.Sprintf("%d_millicores", milliCPUs), func(t *testing.T) {
			formatted := store.InstanceType{VCPUsRequest: milliCPUs}.CPURequest()
			parsed, err := ParseCPUResource(formatted)
			assert.NoError(t, err)

			// For values >= 1000, formatting truncates fractional cores
			if milliCPUs >= 1000 {
				expectedParsed := (milliCPUs / 1000) * 1000
				assert.Equal(t, expectedParsed, parsed)
			} else {
				assert.Equal(t, milliCPUs, parsed)
			}
		})
	}
}

func TestGetInstanceTypeByName(t *testing.T) {
	t.Parallel()

	// Limit is meant to apply to VCPUsRequest (what we show users on the
	// pricing page), not VCPUsLimit. xata.large has a request of 2000 and a
	// limit of 4000, so a 2000-millicore ceiling must still allow it.
	instanceTypes := []store.InstanceType{
		{Name: "xata.micro", VCPUsRequest: 250, VCPUsLimit: 2000, RAM: 1, Region: "us-east-1"},
		{Name: "xata.large", VCPUsRequest: 2000, VCPUsLimit: 4000, RAM: 8, Region: "us-east-1"},
		{Name: "xata.xlarge", VCPUsRequest: 4000, VCPUsLimit: 8000, RAM: 16, Region: "us-east-1"},
	}

	tests := map[string]struct {
		name                   string
		maxAllowedInstanceType int
		want                   store.InstanceType
		wantErr                bool
		wantErrContains        string
	}{
		"request at the limit is allowed": {
			name:                   "xata.large",
			maxAllowedInstanceType: 2000,
			want:                   store.InstanceType{Name: "xata.large", VCPUsRequest: 2000, VCPUsLimit: 4000, RAM: 8, Region: "us-east-1"},
		},
		"request above the limit is rejected": {
			name:                   "xata.xlarge",
			maxAllowedInstanceType: 2000,
			wantErr:                true,
			wantErrContains:        "not available on your current plan",
		},
		"limit above ceiling but request below is allowed": {
			// xata.large has VCPUsLimit 4000 > 4000? no, and request 2000 < 4000.
			name:                   "xata.large",
			maxAllowedInstanceType: 4000,
			want:                   store.InstanceType{Name: "xata.large", VCPUsRequest: 2000, VCPUsLimit: 4000, RAM: 8, Region: "us-east-1"},
		},
		"zero ceiling disables enforcement": {
			name:                   "xata.xlarge",
			maxAllowedInstanceType: 0,
			want:                   store.InstanceType{Name: "xata.xlarge", VCPUsRequest: 4000, VCPUsLimit: 8000, RAM: 16, Region: "us-east-1"},
		},
		"unknown instance type errors": {
			name:                   "xata.unknown",
			maxAllowedInstanceType: 0,
			wantErr:                true,
			wantErrContains:        "is not found",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			mockStore := mocks.NewProjectsStore(t)
			mockStore.EXPECT().ListInstanceTypes(mock.Anything, testOrg, "us-east-1").Return(instanceTypes, nil)
			bs := New(mockStore, nil, nil, nil, "", nil, nil, nil)

			got, err := bs.InstanceTypeByName(context.Background(), testOrg, "us-east-1", tc.name, tc.maxAllowedInstanceType)
			if tc.wantErr {
				require.Error(t, err)
				if tc.wantErrContains != "" {
					require.ErrorContains(t, err, tc.wantErrContains)
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}
