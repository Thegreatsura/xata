// Package pgbackrest holds the pgBackRest settings that the branch operator
// and the clusterpool operator must agree on.
package pgbackrest

import (
	"path"

	corev1 "k8s.io/api/core/v1"
)

const (
	// KeyTypeWebID is the s3.keyType that makes pgBackRest get AWS credentials
	// with the web identity token of the Branch ServiceAccount.
	KeyTypeWebID = "web-id"

	// TokenKey is the key in the web identity Secret that holds the token.
	TokenKey = "token"

	// tokenDir is the directory of the token in the projected volume.
	tokenDir = "pgbackrest"
	// projectedDir is the path where CNPG mounts the projected volume.
	projectedDir = "/projected"
)

// ServiceAccountName is the ServiceAccount whose tokens give pgBackRest access
// to the backups of the Branch. The IAM trust policy accepts only
// ServiceAccounts with this suffix, so the CNPG ServiceAccounts, which have the
// cluster name, cannot assume the role.
func ServiceAccountName(branchName string) string {
	return branchName + "-pgbackrest"
}

// RepoPath is the repo path of a Branch that uses the web identity token. The
// IAM policy gives the token access only to the objects below its `sub` claim,
// and IAM cannot take a part of a claim, so the repo path is the full claim.
func RepoPath(namespace, branchName string) string {
	return "system:serviceaccount:" + namespace + ":" + ServiceAccountName(branchName)
}

// WebIdentitySecretName is the Secret that holds the web identity token for the
// pgBackRest ServiceAccount of the Branch that uses the cluster. The name comes
// from the cluster and not from the Branch: pool clusters are created before a
// Branch claims them, and their pod spec cannot change afterwards.
func WebIdentitySecretName(clusterName string) string {
	return clusterName + "-pgbackrest-web-identity"
}

// WebIdentity returns the projected volume and the environment variables that
// pgBackRest needs for s3.keyType=web-id. The branch operator and the
// clusterpool operator both use this function. Thus, a pool cluster has the
// same pod spec as the Cluster of the Branch that claims it.
//
// The Secret is optional. Thus, the pods can start before the Secret exists.
// The kubelet adds the token to the volume when the Secret exists.
func WebIdentity(clusterName, roleARN string) (corev1.ProjectedVolumeSource, []corev1.EnvVar) {
	optional := true
	volume := corev1.ProjectedVolumeSource{
		Sources: []corev1.VolumeProjection{{
			Secret: &corev1.SecretProjection{
				Name: WebIdentitySecretName(clusterName),
				Items: []corev1.KeyToPath{{
					Key:  TokenKey,
					Path: path.Join(tokenDir, TokenKey),
				}},
				Optional: &optional,
			},
		}},
	}
	env := []corev1.EnvVar{
		{Name: "AWS_ROLE_ARN", Value: roleARN},
		{
			Name:  "AWS_WEB_IDENTITY_TOKEN_FILE",
			Value: path.Join(projectedDir, tokenDir, TokenKey),
		},
	}
	return volume, env
}
