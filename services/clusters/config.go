package clusters

import (
	"fmt"
)

const (
	CloudProviderAWS = "aws"
	CloudProviderGCP = "gcp"
)

type Config struct {
	KubeConfig                  string            `env:"KUBECONFIG" env-default:"" env-description:"path to the kube config file"`
	ClustersNamespace           string            `env:"XATA_CLUSTERS_NAMESPACE" env-default:"xata-clusters" env-description:"namespace for creating the clusters"`
	XataNamespace               string            `env:"XATA_NAMESPACE" env-default:"xata" env-description:"namespace for xata services"`
	ClustersNodeSelector        map[string]string `env:"XATA_CLUSTERS_NODE_SELECTOR" env-default:"" env-description:"node selector for the clusters"`
	ClustersStorageRequest      int32             `env:"XATA_CLUSTERS_STORAGE_REQUEST_GB" env-default:"250" env-description:"default storage size of the cluster in Gb"`
	ClustersStorageClass        string            `env:"XATA_CLUSTERS_STORAGE_CLASS" env-description:"storageclass to use for clusters"`
	ClustersVolumeSnapshotClass string            `env:"XATA_CLUSTERS_VOLUME_SNAPSHOT_CLASS" env-description:"volumesnapshotclass to use for clusters"`
	EnablePooler                bool              `env:"XATA_ENABLE_POOLER" env-default:"true" env-description:"enable PgBouncer connection pooler for new branches"`
	XatastorEnabled             bool              `env:"XATA_XATASTOR_ENABLED" env-default:"false" env-description:"whether the xatastor StorageClass is deployed in this cell"`
	CloudProvider               string            `env:"XATA_CLOUD_PROVIDER" env-default:"aws" env-description:"cloud provider for this cell, selects the pgbackrest backend: aws or gcp"`
	PgBackRestBucket            string            `env:"XATA_PGBACKREST_BUCKET" env-default:"" env-description:"bucket for pgbackrest backups (S3 or GCS)"`
	PgBackRestRegion            string            `env:"XATA_BACKUPS_REGION" env-default:"" env-description:"S3 region for pgbackrest backups (aws only)"`
	PgBackRestEndpoint          string            `env:"XATA_PGBACKREST_ENDPOINT" env-default:"" env-description:"S3 endpoint for pgbackrest backups; set for a non-AWS S3-compatible store such as MinIO (local dev) or Cloudflare R2"`
	PgBackRestGCSServiceAccount string            `env:"XATA_PGBACKREST_GCS_SERVICE_ACCOUNT" env-default:"" env-description:"GCP service account email for pgbackrest GCS backups via Workload Identity (gcp only)"`
	PgBackRestEncryptionEnabled bool              `env:"XATA_PGBACKREST_ENCRYPTION_ENABLED" env-default:"false" env-description:"enable client-side encryption for new pgbackrest backup destinations in this cell"`
	UseStorageQoSClasses        bool              `env:"XATA_USE_STORAGE_QOS_CLASSES" env-default:"false" env-description:"whether to use storage QoS classes for new branches"`

	// VictoriaMetricsURL points at the cell-local VictoriaMetrics single-node
	// instance used to back GetBranchMetrics. Empty means no metrics backend
	// is configured and the RPC will fail fast.
	VictoriaMetricsURL string `env:"VICTORIAMETRICS_URL" env-default:"" env-description:"base URL of the cell-local VictoriaMetrics instance"`
	// VictoriaLogsURL points at the cell-local VictoriaLogs instance used to
	// back GetBranchLogs.
	VictoriaLogsURL string `env:"VICTORIALOGS_URL" env-default:"" env-description:"base URL of the cell-local VictoriaLogs instance"`
}

func (cfg *Config) Validate() error {
	if cfg.ClustersStorageClass == "" {
		return fmt.Errorf("storage class is required but not set")
	}
	if cfg.ClustersVolumeSnapshotClass == "" {
		return fmt.Errorf("volume snapshot class is required but not set")
	}
	if cfg.CloudProvider != CloudProviderAWS && cfg.CloudProvider != CloudProviderGCP {
		return fmt.Errorf("cloud provider must be %q or %q, got: %q", CloudProviderAWS, CloudProviderGCP, cfg.CloudProvider)
	}
	return nil
}
