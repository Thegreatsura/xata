package api

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"xata/internal/xvalidator"
)

func TestIsDescriptionValid(t *testing.T) {
	t.Parallel()

	longDescription := "averylongdescriptionmadefortestingaverylongdescriptionmadefortesting"
	shortDescription := "shortokdescription-09"
	invalidCharsDescription := "-shortinvalid"

	errorMaxLength := xvalidator.ErrorMaxLength{Limit: MaxBranchDescriptionLength}
	errorInvalid := ErrorInvalidDescription{
		Message:     fmt.Sprintf("invalid branch description %s", invalidCharsDescription),
		Description: invalidCharsDescription,
	}

	tests := []struct {
		name         string
		description  string
		wantError    bool
		errorMessage string
	}{
		{
			name:         "tooLong",
			description:  longDescription,
			wantError:    true,
			errorMessage: errorMaxLength.Error(),
		},
		{
			name:        "ok",
			description: shortDescription,
			wantError:   false,
		},
		{
			name:         "invalid",
			description:  invalidCharsDescription,
			wantError:    true,
			errorMessage: errorInvalid.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsBranchDescriptionValid(tt.description)
			if tt.wantError == true {
				assert.Error(t, got)
				assert.Equal(t, tt.errorMessage, got.Error())

			} else {
				assert.NoError(t, got)
			}
		})
	}
}
