package flags

import (
	"testing"

	"github.com/stretchr/testify/require"

	"xata/internal/postgresversions"
)

// TestPgMajorFlagsCoverHiddenMajors guards against marking a major hidden in
// versions.yaml without adding its flag: such a major would be hidden for every
// organization, with no way to enable it.
func TestPgMajorFlagsCoverHiddenMajors(t *testing.T) {
	for image, major := range postgresversions.HiddenMajorImages() {
		require.Contains(t, PgMajorFlags, major,
			"major %s is hidden (image %s) but has no feature flag", major, image)
	}
}
