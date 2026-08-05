package branchoperator

import (
	"time"

	"xata/internal/envcfg"
)

type Config struct {
	ClustersNamespace string `env:"XATA_CLUSTERS_NAMESPACE" env-default:"xata-clusters" env-description:"namespace where the operator creates managed resources"`
	BackupsBucket     string `env:"XATA_BACKUPS_BUCKET" env-description:"bucket for storing the cluster backups"`
	BackupsEndpoint   string `env:"XATA_BACKUPS_ENDPOINT" env-description:"endpoint for reaching the backups bucket; set for a non-AWS S3-compatible store such as Cloudflare R2"`
	// When BackupsEndpoint is set (non-AWS S3-compatible store), backups
	// authenticate with static credentials from this Secret instead of an IAM
	// role. Defaults match the local-dev MinIO secret.
	BackupsCredentialsSecretName         string                     `env:"XATA_BACKUPS_CREDENTIALS_SECRET_NAME" env-default:"minio-eu" env-description:"Secret (in the clusters namespace) holding static S3 credentials, used when XATA_BACKUPS_ENDPOINT is set"`
	BackupsCredentialsAccessKeyIDKey     string                     `env:"XATA_BACKUPS_CREDENTIALS_ACCESS_KEY_ID_KEY" env-default:"rootUser" env-description:"key in the credentials Secret holding the access key ID"`
	BackupsCredentialsSecretAccessKeyKey string                     `env:"XATA_BACKUPS_CREDENTIALS_SECRET_ACCESS_KEY_KEY" env-default:"rootPassword" env-description:"key in the credentials Secret holding the secret access key"`
	CloudProvider                        string                     `env:"XATA_CLOUD_PROVIDER" env-default:"aws" env-description:"cloud provider for per-Branch identity and backup credentials: aws or gcp"`
	BarmanRegionSecretName               string                     `env:"XATA_BARMAN_REGION_SECRET_NAME" env-default:"barman-dummy-secret" env-description:"chart-managed secret referenced as the barman AWS region"`
	BarmanRegionSecretKey                string                     `env:"XATA_BARMAN_REGION_SECRET_KEY" env-default:"dummy" env-description:"key in the chart-managed barman AWS region secret"`
	Tolerations                          envcfg.TolerationListField `env:"XATA_CLUSTERS_TOLERATIONS" env-default:"xata.io/workload=dataplane:NoSchedule" env-description:"tolerations for cluster pods in the format key=value:effect"`
	EnforceZone                          bool                       `env:"XATA_ENFORCE_ZONE" env-default:"false" env-description:"enable zone-based pod anti-affinity for multi-instance clusters"`
	ImagePullSecrets                     []string                   `env:"XATA_IMAGE_SECRETS" env-default:"ghcr-secret" env-description:"image pull secrets for private PostgreSQL images"`
	XatastorEnabled                      bool                       `env:"XATA_XATASTOR_ENABLED" env-default:"true" env-description:"enable xatastor CSI integration for wakeup requests"`
	CSINodeNamespace                     string                     `env:"XATA_CSI_NODE_NAMESPACE" env-default:"xatastor" env-description:"namespace where CSI node plugin pods run"`
	CSINodePort                          int                        `env:"XATA_CSI_NODE_PORT" env-default:"50061" env-description:"port for the SlotController service on CSI node plugin pods"`
	WakeupRequestTTL                     time.Duration              `env:"XATA_WAKEUP_REQUEST_TTL" env-default:"60s" env-description:"time to keep completed WakeupRequests before deletion"`
	WakeupRPCTimeout                     time.Duration              `env:"XATA_WAKEUP_RPC_TIMEOUT" env-default:"10s" env-description:"timeout for the WakeUp RPC to the SlotController service on CSI node plugin pods"`
	WakeupMaxConcurrent                  int                        `env:"XATA_WAKEUP_MAX_CONCURRENT" env-default:"16" env-description:"maximum concurrent wakeup reconciliations"`
	BranchMaxConcurrent                  int                        `env:"XATA_BRANCH_MAX_CONCURRENT" env-default:"1" env-description:"maximum concurrent Branch reconciliations (default 1 = current behavior)"`
	KubeClientQPS                        float64                    `env:"XATA_KUBE_CLIENT_QPS" env-default:"0" env-description:"Kubernetes API client QPS; 0 keeps the controller-runtime default (20)"`
	KubeClientBurst                      int                        `env:"XATA_KUBE_CLIENT_BURST" env-default:"0" env-description:"Kubernetes API client burst; 0 keeps the controller-runtime default (30)"`
}
