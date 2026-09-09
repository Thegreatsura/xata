package api

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"xata/internal/xvalidator"
	"xata/services/projects/store"
)

func TestErrorStatusCodes(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		err        error
		wantCode   int
		wantSubstr string
	}{
		"ErrorBranchConflict": {
			err:        ErrorBranchConflict{BranchID: "br-1"},
			wantCode:   409,
			wantSubstr: "modified concurrently",
		},
		"ErrorBranchNotFound": {
			err:        ErrorBranchNotFound{BranchID: "br-1"},
			wantCode:   404,
			wantSubstr: "not found",
		},
		"ErrorInvalidParam": {
			err:        ErrorInvalidParam{BranchName: "br-1", Param: "name", Message: "too long"},
			wantCode:   400,
			wantSubstr: "invalid parameter",
		},
		"ErrorBranchUpdateForbidden": {
			err:        ErrorBranchUpdateForbidden{BranchID: "br-1"},
			wantCode:   403,
			wantSubstr: "temporarily unavailable",
		},
		"ErrorParentBranchUnhealthy": {
			err:        ErrorParentBranchUnhealthy{ParentID: "br-1"},
			wantCode:   412,
			wantSubstr: "not healthy",
		},
		"ErrorBranchCreationDisabled": {
			err:        ErrorBranchCreationDisabled{},
			wantCode:   503,
			wantSubstr: "temporarily disabled",
		},
		"ErrorCredentialsForBranchNotFound": {
			err:        ErrorCredentialsForBranchNotFound{BranchID: "br-1", Username: "xata"},
			wantCode:   404,
			wantSubstr: "not found",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			type statusCoder interface {
				StatusCode() int
			}
			sc, ok := tt.err.(statusCoder)
			require.True(t, ok)
			require.Equal(t, tt.wantCode, sc.StatusCode())
			require.Contains(t, tt.err.Error(), tt.wantSubstr)
		})
	}
}

func TestIsDescriptionValid(t *testing.T) {
	t.Parallel()

	atLimitDescription := strings.Repeat("a", store.DefaultMaxDescriptionLength)
	overLimitDescription := strings.Repeat("a", store.DefaultMaxDescriptionLength+1)

	tests := []struct {
		name        string
		description string
		wantError   error
	}{
		{
			name:        "at the maximum length",
			description: atLimitDescription,
		},
		{
			name:        "one character over the maximum length",
			description: overLimitDescription,
			wantError:   xvalidator.ErrorMaxLength{Limit: store.DefaultMaxDescriptionLength},
		},
		{
			name:        "plain description",
			description: "shortokdescription-09",
		},
		{
			name:        "empty description clears the value",
			description: "",
		},
		{
			name:        "underscore",
			description: "managed_by_terraform",
		},
		{
			name:        "dot",
			description: "release.2026.09.09",
		},
		{
			name:        "slash",
			description: "company/infra/managed-by-x",
		},
		{
			name:        "colon",
			description: "owner:platform-team",
		},
		{
			name:        "uuid",
			description: "3f2504e0-4f89-11d3-9a0c-0305e82c3301",
		},
		{
			name:        "every allowed separator together",
			description: "a-b_c.d/e:f g",
		},
		{
			name:        "leading dash",
			description: "-description",
			wantError: ErrorInvalidDescription{
				Message:     "invalid branch description -description",
				Description: "-description",
			},
		},
		{
			name:        "leading slash",
			description: "/company/infra",
			wantError: ErrorInvalidDescription{
				Message:     "invalid branch description /company/infra",
				Description: "/company/infra",
			},
		},
		{
			name:        "disallowed character",
			description: "owner@example.com",
			wantError: ErrorInvalidDescription{
				Message:     "invalid branch description owner@example.com",
				Description: "owner@example.com",
			},
		},
		{
			name:        "newline",
			description: "first line\nsecond line",
			wantError: ErrorInvalidDescription{
				Message:     "invalid branch description first line\nsecond line",
				Description: "first line\nsecond line",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := IsBranchDescriptionValid(&tt.description, store.DefaultMaxDescriptionLength)
			if tt.wantError != nil {
				require.Error(t, got)
				assert.Equal(t, tt.wantError.Error(), got.Error())
				return
			}
			assert.NoError(t, got)
		})
	}

	t.Run("nil description is not provided", func(t *testing.T) {
		t.Parallel()

		assert.NoError(t, IsBranchDescriptionValid(nil, store.DefaultMaxDescriptionLength))
	})
}
