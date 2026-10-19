package pgbackrest_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"xata/internal/pgbackrest"
)

func TestGCSFolderURI(t *testing.T) {
	t.Parallel()
	for name, folder := range map[string]string{"branch": "branch", "nested": "team/branch/", "escaped": "branch #1"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			bucket, got, err := pgbackrest.ParseGCSFolderURI(pgbackrest.GCSFolderURI("backups", folder))
			require.NoError(t, err)
			require.Equal(t, "backups", bucket)
			want := folder
			if want[len(want)-1] != '/' {
				want += "/"
			}
			require.Equal(t, want, got)
		})
	}
}

func TestParseGCSFolderURIInvalid(t *testing.T) {
	t.Parallel()
	for _, location := range []string{"", "backups/branch/", "s3://backups/branch/", "gs:///branch/", "gs://backups/", "gs://user@backups/branch/", "gs://backups:443/branch/", "gs://backups/branch/?query", "gs://backups/branch/#fragment"} {
		t.Run(location, func(t *testing.T) {
			t.Parallel()
			_, _, err := pgbackrest.ParseGCSFolderURI(location)
			require.Error(t, err)
		})
	}
}
