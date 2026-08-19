package wakeup

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAdoptableImage(t *testing.T) {
	t.Parallel()

	const repo = "ghcr.io/xataio/postgres-images/"

	tests := map[string]struct {
		branchImage  string
		clusterImage string
		want         string
	}{
		"newer minor of the same major is adopted": {
			branchImage:  repo + "cnpg-postgres-plus:17.10",
			clusterImage: repo + "cnpg-postgres-plus:17.11",
			want:         repo + "cnpg-postgres-plus:17.11",
		},
		"minor several versions ahead is adopted": {
			branchImage:  repo + "cnpg-postgres-plus:18.1",
			clusterImage: repo + "cnpg-postgres-plus:18.6",
			want:         repo + "cnpg-postgres-plus:18.6",
		},
		"equal image is not adopted": {
			branchImage:  repo + "cnpg-postgres-plus:17.11",
			clusterImage: repo + "cnpg-postgres-plus:17.11",
			want:         "",
		},
		"older minor is not adopted": {
			branchImage:  repo + "cnpg-postgres-plus:17.11",
			clusterImage: repo + "cnpg-postgres-plus:17.10",
			want:         "",
		},
		"different major is not adopted": {
			branchImage:  repo + "cnpg-postgres-plus:17.10",
			clusterImage: repo + "cnpg-postgres-plus:18.6",
			want:         "",
		},
		"different offering is not adopted": {
			branchImage:  repo + "xata-analytics:17.10",
			clusterImage: repo + "cnpg-postgres-plus:17.11",
			want:         "",
		},
		"date suffixed tags compare on version": {
			branchImage:  repo + "cnpg-postgres-plus:17.10",
			clusterImage: repo + "cnpg-postgres-plus:17.11-20260814",
			want:         repo + "cnpg-postgres-plus:17.11-20260814",
		},
		"unparsable branch image is not adopted": {
			branchImage:  repo + "cnpg-postgres-plus:latest",
			clusterImage: repo + "cnpg-postgres-plus:17.11",
			want:         "",
		},
		"unparsable cluster image is not adopted": {
			branchImage:  repo + "cnpg-postgres-plus:17.10",
			clusterImage: repo + "cnpg-postgres-plus:18rc1",
			want:         "",
		},
		"empty branch image is not adopted": {
			branchImage:  "",
			clusterImage: repo + "cnpg-postgres-plus:17.11",
			want:         "",
		},
		"empty cluster image is not adopted": {
			branchImage:  repo + "cnpg-postgres-plus:17.10",
			clusterImage: "",
			want:         "",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := adoptableImage(tc.branchImage, tc.clusterImage)
			require.Equal(t, tc.want, got)
		})
	}
}
