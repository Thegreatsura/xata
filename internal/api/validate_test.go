package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidator(t *testing.T) {
	validator := newEchoValidator()

	for name, test := range map[string]struct {
		value   any
		wantErr bool
	}{
		"ok struct": {
			struct {
				A string `validate:"required"`
			}{"test"},
			false,
		},
		"struct with missing value": {
			struct {
				A string `validate:"required"`
			}{""},
			true,
		},
		"pointer to struct with missing value": {
			&struct {
				A string `validate:"required"`
			}{""},
			true,
		},
		"map[string]struct with missign value": {
			map[string]struct {
				A string `validate:"required"`
			}{"field": {""}},
			true,
		},
		"array of struct with missing value": {
			[]struct {
				A string `validate:"required"`
			}{{""}},
			true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := validator.Validate(test.value)
			t.Log(err)
			if test.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
