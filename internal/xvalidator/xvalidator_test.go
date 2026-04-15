package xvalidator

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsValidIdentifier(t *testing.T) {
	const wantOK = ""

	t.Parallel()
	tests := map[string]string{
		"test":      wantOK,
		"base1":     wantOK,
		"_base":     wantOK,
		"base$":     "offset 4: invalid symbol [$], only alphanumerics and '_', or '~' are allowed",
		"base_test": wantOK,
		"base-test": "offset 4: invalid symbol [-], only alphanumerics and '_', or '~' are allowed",
		"base~test": wantOK,
		"~test":     wantOK,
		"unicöde":   "offset 4: unicode characters are not allowed",
		"12abc":     wantOK,
		"test!":     "offset 4: invalid symbol [!], only alphanumerics and '_', or '~' are allowed",
	}

	for id, want := range tests {
		t.Run(id, func(t *testing.T) {
			got := IsValidIdentifier(id)
			if want == "" {
				assert.NoError(t, got)
			} else {
				assert.Error(t, got)
				assert.Equal(t, want, got.Error())
			}
		})
	}
}

func TestIsDurationValid(t *testing.T) {
	t.Parallel()

	tests := map[string]bool{
		"1d":        true,
		"1h":        true,
		"1m":        true,
		"1s":        true,
		"1ms":       true,
		"1w":        false,
		"1y":        false,
		"1M":        false,
		"1212312ms": true,
		"1212312s":  true,
		"1212312d":  true,
		"-1212m":    false,
		"60d":       true,
	}

	for d, want := range tests {
		t.Run(d, func(t *testing.T) {
			got := IsDurationValid(d)
			assert.Equal(t, want, got)
		})
	}
}
