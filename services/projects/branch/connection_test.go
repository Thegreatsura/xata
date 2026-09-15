package branch

import (
	"testing"

	"xata/services/projects/store"

	"github.com/stretchr/testify/require"
)

func TestBranchEndpoint(t *testing.T) {
	tests := map[string]struct {
		regionHostPort  string
		defaultHostPort string
		subdomain       string // "" means no cell subdomain (region-only hostname)
		wantHostname    string
		wantPort        int
		wantError       bool
	}{
		"region host:port": {
			regionHostPort: "eu-central-1.xata.tech:7654",
			wantHostname:   "br-1.eu-central-1.xata.tech",
			wantPort:       7654,
		},
		"host-only region uses the default postgres port": {
			regionHostPort: "us-east-1.xata.tech",
			wantHostname:   "br-1.us-east-1.xata.tech",
			wantPort:       5432,
		},
		"empty region falls back to the handler default": {
			defaultHostPort: "testdomain:5432",
			wantHostname:    "br-1.testdomain",
			wantPort:        5432,
		},
		"subdomain qualifies a host-only region": {
			regionHostPort: "us-east-1.xata.tech",
			subdomain:      "cell-2",
			wantHostname:   "br-1.cell-2.us-east-1.xata.tech",
			wantPort:       5432,
		},
		"subdomain qualifies a host:port region": {
			regionHostPort: "eu-central-1.xata.tech:7654",
			subdomain:      "cell-2",
			wantHostname:   "br-1.cell-2.eu-central-1.xata.tech",
			wantPort:       7654,
		},
		"subdomain qualifies the handler default": {
			defaultHostPort: "testdomain:5432",
			subdomain:       "cell-2",
			wantHostname:    "br-1.cell-2.testdomain",
			wantPort:        5432,
		},
		"no gateway configured at all fails": {
			wantError: true,
		},
		"no gateway configured fails even with a subdomain": {
			subdomain: "cell-2",
			wantError: true,
		},
		"non-numeric port fails": {
			regionHostPort: "us-east-1.xata.tech:sql",
			wantError:      true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var subdomain *string
			if tt.subdomain != "" {
				subdomain = &tt.subdomain
			}
			bs := New(nil, nil, nil, nil, tt.defaultHostPort, nil, nil, nil)
			gotHostname, gotPort, err := bs.BranchEndpoint(&store.Region{GatewayHostPort: tt.regionHostPort}, subdomain, "br-1")
			if tt.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantHostname, gotHostname)
			require.Equal(t, tt.wantPort, gotPort)
		})
	}
}
