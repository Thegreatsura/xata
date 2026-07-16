package api

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"xata/services/projects/api/spec"
)

func TestValidateIPFiltering(t *testing.T) {
	t.Parallel()

	strPtr := func(s string) *string { return &s }

	cidrs := func(values ...string) []spec.CidrEntry {
		entries := make([]spec.CidrEntry, len(values))
		for i, v := range values {
			entries[i] = spec.CidrEntry{Cidr: v}
		}
		return entries
	}

	tests := map[string]struct {
		cfg        *spec.IPFilteringConfiguration
		wantErr    bool
		wantSubstr string
	}{
		"nil config": {
			cfg: nil,
		},
		"empty config": {
			cfg: &spec.IPFilteringConfiguration{Enabled: true},
		},
		// IPv4
		"valid IPv4 CIDR": {
			cfg: &spec.IPFilteringConfiguration{Enabled: true, Cidr: cidrs("192.168.0.0/24")},
		},
		"valid IPv4 single-host CIDR": {
			cfg: &spec.IPFilteringConfiguration{Enabled: true, Cidr: cidrs("10.0.0.1/32")},
		},
		"valid bare IPv4 address": {
			cfg: &spec.IPFilteringConfiguration{Enabled: true, Cidr: cidrs("203.0.113.7")},
		},
		"IPv4 allow-all rejected": {
			cfg:        &spec.IPFilteringConfiguration{Enabled: true, Cidr: cidrs("0.0.0.0/0")},
			wantErr:    true,
			wantSubstr: "allows all addresses",
		},
		"IPv4 prefix out of range": {
			cfg:        &spec.IPFilteringConfiguration{Enabled: true, Cidr: cidrs("10.0.0.0/33")},
			wantErr:    true,
			wantSubstr: "not a valid",
		},
		"IPv4 octet out of range": {
			cfg:        &spec.IPFilteringConfiguration{Enabled: true, Cidr: cidrs("10.0.0.256/24")},
			wantErr:    true,
			wantSubstr: "not a valid",
		},
		"IPv4 missing octets": {
			cfg:        &spec.IPFilteringConfiguration{Enabled: true, Cidr: cidrs("203.0.113/24")},
			wantErr:    true,
			wantSubstr: "not a valid",
		},
		// IPv6
		"valid IPv6 CIDR": {
			cfg: &spec.IPFilteringConfiguration{Enabled: true, Cidr: cidrs("2001:db8::/32")},
		},
		"valid IPv6 single-host CIDR": {
			cfg: &spec.IPFilteringConfiguration{Enabled: true, Cidr: cidrs("2001:db8::1/128")},
		},
		"valid bare IPv6 address": {
			cfg: &spec.IPFilteringConfiguration{Enabled: true, Cidr: cidrs("2001:db8:85a3::8a2e:370:7334")},
		},
		"valid IPv4-mapped IPv6 address": {
			cfg: &spec.IPFilteringConfiguration{Enabled: true, Cidr: cidrs("::ffff:192.0.2.1")},
		},
		"IPv6 allow-all rejected": {
			cfg:        &spec.IPFilteringConfiguration{Enabled: true, Cidr: cidrs("::/0")},
			wantErr:    true,
			wantSubstr: "allows all addresses",
		},
		"IPv6 prefix out of range": {
			cfg:        &spec.IPFilteringConfiguration{Enabled: true, Cidr: cidrs("2001:db8::/129")},
			wantErr:    true,
			wantSubstr: "not a valid",
		},
		"IPv6 zone identifier rejected": {
			cfg:        &spec.IPFilteringConfiguration{Enabled: true, Cidr: cidrs("fe80::1%eth0")},
			wantErr:    true,
			wantSubstr: "zone identifier",
		},
		"IPv6 too many groups": {
			cfg:        &spec.IPFilteringConfiguration{Enabled: true, Cidr: cidrs("1:2:3:4:5:6:7:8:9")},
			wantErr:    true,
			wantSubstr: "not a valid",
		},
		// mixed and garbage
		"valid mixed IPv4 and IPv6 entries": {
			cfg: &spec.IPFilteringConfiguration{Enabled: true, Cidr: cidrs("192.168.0.0/16", "2001:db8::/48", "10.1.2.3", "::1")},
		},
		"empty CIDR rejected": {
			cfg:        &spec.IPFilteringConfiguration{Enabled: true, Cidr: cidrs("")},
			wantErr:    true,
			wantSubstr: "must not be empty",
		},
		"garbage text rejected": {
			cfg:        &spec.IPFilteringConfiguration{Enabled: true, Cidr: cidrs("not-an-ip")},
			wantErr:    true,
			wantSubstr: "not a valid",
		},
		"second entry invalid reports index": {
			cfg:        &spec.IPFilteringConfiguration{Enabled: true, Cidr: cidrs("10.0.0.0/8", "garbage")},
			wantErr:    true,
			wantSubstr: "ipFiltering.cidr[1]",
		},
		// size limits
		"CIDR string too long": {
			cfg:        &spec.IPFilteringConfiguration{Enabled: true, Cidr: cidrs(strings.Repeat("a", maxCIDRLength+1))},
			wantErr:    true,
			wantSubstr: "too long",
		},
		"too many entries": {
			cfg: &spec.IPFilteringConfiguration{
				Enabled: true,
				Cidr:    make([]spec.CidrEntry, maxIPFilteringCIDRs+1),
			},
			wantErr:    true,
			wantSubstr: "too many CIDR entries",
		},
		"max entries allowed": {
			cfg: func() *spec.IPFilteringConfiguration {
				entries := make([]spec.CidrEntry, maxIPFilteringCIDRs)
				for i := range entries {
					entries[i] = spec.CidrEntry{Cidr: "10.0.0.0/8"}
				}
				return &spec.IPFilteringConfiguration{Enabled: true, Cidr: entries}
			}(),
		},
		"description too long": {
			cfg: &spec.IPFilteringConfiguration{
				Enabled: true,
				Cidr: []spec.CidrEntry{{
					Cidr:        "10.0.0.0/8",
					Description: strPtr(strings.Repeat("x", maxCIDRDescriptionLength+1)),
				}},
			},
			wantErr:    true,
			wantSubstr: "description is too long",
		},
		"description at limit allowed": {
			cfg: &spec.IPFilteringConfiguration{
				Enabled: true,
				Cidr: []spec.CidrEntry{{
					Cidr:        "2001:db8::/64",
					Description: strPtr(strings.Repeat("x", maxCIDRDescriptionLength)),
				}},
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := validateIPFiltering(tt.cfg)
			if !tt.wantErr {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantSubstr)
			var invalidParam ErrorInvalidParam
			require.ErrorAs(t, err, &invalidParam)
			require.Equal(t, 400, invalidParam.StatusCode())
		})
	}
}
