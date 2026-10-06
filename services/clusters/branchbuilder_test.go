package clusters

import (
	"testing"

	"github.com/stretchr/testify/require"

	"xata/internal/pgbackrest"
	"xata/services/branch-operator/api/v1alpha1"
)

// webIdentityKeyType is the S3 key type of a new branch on AWS S3.
const webIdentityKeyType = pgbackrest.KeyTypeWebID

// webIdentityRepoPath is the repo path of a new branch on AWS S3, in the
// clusters namespace of the tests.
func webIdentityRepoPath(branchName string) string {
	return pgbackrest.RepoPath("xata-clusters", branchName)
}

func TestWithPgBackRestS3(t *testing.T) {
	t.Parallel()

	testcases := map[string]struct {
		provider     string
		endpoint     string
		wantKeyType  string
		wantRepoPath string
	}{
		"aws s3 uses the web identity token": {
			provider:     CloudProviderAWS,
			wantKeyType:  pgbackrest.KeyTypeWebID,
			wantRepoPath: "system:serviceaccount:xata-clusters:branch-id-pgbackrest",
		},
		"s3-compatible endpoint keeps the default repo": {
			provider: CloudProviderAWS,
			endpoint: "http://rustfs:9000",
		},
		"s3 outside aws keeps the default repo": {},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			b := &BranchBuilder{branch: &v1alpha1.Branch{
				Name: "branch-id",
				Spec: v1alpha1.BranchSpec{
					BackupSpec: &v1alpha1.BackupSpec{
						Method:     v1alpha1.BackupMethodPgBackRest,
						PgBackRest: &v1alpha1.PgBackRestSpec{},
					},
				},
			}}

			got := b.WithPgBackRest(tc.provider, "bucket", "us-east-1", tc.endpoint, "", "", "", "", "xata-clusters").
				Build().Spec.BackupSpec.PgBackRest

			require.Equal(t, tc.wantKeyType, got.S3.KeyType)
			require.Equal(t, tc.wantRepoPath, got.RepoPath)
		})
	}
}
