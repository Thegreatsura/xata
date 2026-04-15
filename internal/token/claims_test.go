package token

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClaims_UserID(t *testing.T) {
	tests := []struct {
		name   string
		claims *Claims
		want   string
	}{
		{"nil claims", nil, ""},
		{"empty ID", &Claims{}, ""},
		{"ID set", &Claims{ID: "abc123"}, "abc123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.claims.UserID())
		})
	}
}

func TestClaims_UserEmail(t *testing.T) {
	tests := []struct {
		name   string
		claims *Claims
		want   string
	}{
		{"nil claims", nil, ""},
		{"empty Email", &Claims{}, ""},
		{"Email set", &Claims{Email: "a@b.com"}, "a@b.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.claims.UserEmail())
		})
	}
}

func TestClaims_APIKeyID(t *testing.T) {
	tests := []struct {
		name   string
		claims *Claims
		want   string
	}{
		{"nil claims", nil, ""},
		{"KeyID nil", &Claims{KeyID: ""}, ""},
		{"KeyID set", &Claims{KeyID: "key42"}, "key42"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.claims.APIKeyID())
		})
	}
}

func TestClaims_HasAccessToOrganization(t *testing.T) {
	tests := []struct {
		name       string
		claims     *Claims
		orgID      string
		wantAccess bool
	}{
		{"nil claims", nil, "org1", false},
		{"empty org id", &Claims{Organizations: map[string]Organization{"org1": {}}}, "", false},
		{"no organizations", &Claims{Organizations: map[string]Organization{}}, "org1", false},
		{"organization present", &Claims{Organizations: map[string]Organization{"org1": {ID: "org1", Status: "enabled"}, "org2": {ID: "org2", Status: "enabled"}}}, "org1", true},
		{"organization absent", &Claims{Organizations: map[string]Organization{"org2": {ID: "org2", Status: "enabled"}}}, "org1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.claims.HasAccessToOrganization(tt.orgID)
			require.Equal(t, tt.wantAccess, got)
		})
	}
}

func TestClaims_HasWriteAccess(t *testing.T) {
	tests := []struct {
		name       string
		claims     *Claims
		orgID      string
		wantAccess bool
	}{
		{"nil claims", nil, "org1", false},
		{"empty org id", &Claims{Organizations: map[string]Organization{"org1": {ID: "org1", Status: OrgEnabledStatus}}}, "", false},
		{"no organizations (empty map)", &Claims{Organizations: map[string]Organization{}}, "org1", false},
		{"no organizations (nil map)", &Claims{}, "org1", false},
		{"organization present and enabled", &Claims{Organizations: map[string]Organization{
			"org1": {ID: "org1", Status: OrgEnabledStatus},
			"org2": {ID: "org2", Status: OrgEnabledStatus},
		}}, "org1", true},
		{"organization present but disabled", &Claims{Organizations: map[string]Organization{
			"org1": {ID: "org1", Status: "disabled"},
		}}, "org1", false},
		{"organization present but unrecognized status", &Claims{Organizations: map[string]Organization{
			"org1": {ID: "org1", Status: "suspended"},
		}}, "org1", false},
		{"organization absent", &Claims{Organizations: map[string]Organization{
			"org2": {ID: "org2", Status: OrgEnabledStatus},
		}}, "org1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.claims.IsEnabledOrganization(tt.orgID)
			require.Equal(t, tt.wantAccess, got)
		})
	}
}
