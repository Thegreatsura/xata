package flags

import (
	"testing"

	"github.com/stretchr/testify/require"

	"xata/internal/openfeature"
)

// TestPgMajorFlag guards the flag names: the flags for existing hidden majors
// are configured in the provider under these names.
func TestPgMajorFlag(t *testing.T) {
	for major, name := range map[string]string{
		"14": "pgMajor14",
		"15": "pgMajor15",
		"19": "pgMajor19",
	} {
		require.Equal(t, openfeature.FeatureFlag{Name: name, DefaultEnabled: false}, PgMajorFlag(major))
	}
}
