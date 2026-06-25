package clusters

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidate(t *testing.T) {
	tests := map[string]struct {
		cfg     Config
		wantErr string
	}{
		"valid config": {
			cfg: Config{
				ClustersStorageClass:        "xatastor",
				ClustersVolumeSnapshotClass: "xatastor",
				CloudProvider:               CloudProviderAWS,
			},
		},
		"valid gcp config": {
			cfg: Config{
				ClustersStorageClass:        "xatastor",
				ClustersVolumeSnapshotClass: "xatastor",
				CloudProvider:               CloudProviderGCP,
			},
		},
		"invalid cloud provider": {
			cfg: Config{
				ClustersStorageClass:        "xatastor",
				ClustersVolumeSnapshotClass: "xatastor",
				CloudProvider:               "azure",
			},
			wantErr: "cloud provider must be",
		},
		"missing storage class": {
			cfg: Config{
				ClustersVolumeSnapshotClass: "xatastor",
			},
			wantErr: "storage class is required",
		},
		"missing volume snapshot class": {
			cfg: Config{
				ClustersStorageClass: "xatastor",
			},
			wantErr: "volume snapshot class is required",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}
