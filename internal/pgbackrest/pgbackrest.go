// Package pgbackrest holds the pgBackRest settings that the branch operator
// and the clusterpool operator must agree on.
package pgbackrest

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path"

	corev1 "k8s.io/api/core/v1"
)

const (
	// CipherSecretSuffix is appended to the Branch name to form the Secret
	// that holds the pgBackRest repository cipher passphrases of the Branch.
	CipherSecretSuffix = "-pgbackrest"

	// CipherPassphraseKey is the Secret data key holding the passphrase that
	// encrypts the repository of the Branch. It names a key, not a credential.
	//nolint:gosec
	CipherPassphraseKey = "cipher-passphrase"

	// RestoreCipherPassphraseKey is the Secret data key holding the passphrase
	// of the repository a Branch restores from, copied from the source Branch.
	RestoreCipherPassphraseKey = "restore-cipher-passphrase"

	// cipherPassphraseBytes is the entropy of a generated passphrase.
	cipherPassphraseBytes = 32

	// KeyTypeWebID is the s3.keyType and gcs.keyType that makes pgBackRest get
	// cloud credentials with the web identity token of the Branch
	// ServiceAccount.
	KeyTypeWebID = "web-id"

	// GCSBucketAnnotation is the annotation with the backup bucket of the
	// Branch on the pgBackRest ServiceAccount. The branch-operator gives the
	// ServiceAccount access to the managed folder of the Branch in that bucket.
	GCSBucketAnnotation = "xata.io/gcs-bucket"

	// TokenKey is the key in the web identity Secret that holds the token.
	TokenKey = "token"

	// GCSCredentialsKey is the key in the web identity Secret that holds the
	// external account credential file for gcs.keyType=web-id.
	GCSCredentialsKey = "gcs-credentials.json"

	// tokenDir is the directory of the token in the projected volume.
	tokenDir = "pgbackrest"
	// projectedDir is the path where CNPG mounts the projected volume.
	projectedDir = "/projected"
)

// CipherSecretName is the Secret that holds the repository cipher passphrases
// of a Branch. It shares its name with the pgBackRest ServiceAccount of the
// Branch; the two are kept as separate functions so that the shared name stays
// visible.
func CipherSecretName(branchName string) string {
	return branchName + CipherSecretSuffix
}

// GeneratePassphrase returns a new repository cipher passphrase: 32 random
// bytes, base64url-encoded without padding so it is safe in config files.
func GeneratePassphrase() ([]byte, error) {
	raw := make([]byte, cipherPassphraseBytes)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	encoded := make([]byte, base64.RawURLEncoding.EncodedLen(len(raw)))
	base64.RawURLEncoding.Encode(encoded, raw)
	return encoded, nil
}

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
	env := []corev1.EnvVar{
		{Name: "AWS_ROLE_ARN", Value: roleARN},
		{Name: "AWS_WEB_IDENTITY_TOKEN_FILE", Value: projectedPath(TokenKey)},
	}
	return webIdentityVolume(clusterName, TokenKey), env
}

// GCSWebIdentity returns the projected volume and the environment variable
// that pgBackRest needs for gcs.keyType=web-id.
func GCSWebIdentity(clusterName string) (corev1.ProjectedVolumeSource, []corev1.EnvVar) {
	env := []corev1.EnvVar{
		{Name: "PGBACKREST_REPO1_GCS_KEY", Value: projectedPath(GCSCredentialsKey)},
	}
	return webIdentityVolume(clusterName, TokenKey, GCSCredentialsKey), env
}

// GCSCredentials returns the external account credential file that makes
// pgBackRest exchange the mounted token for a Google access token. The issuer
// is the `iss` claim of the token, which is also the name of the provider in
// the GKE Workload Identity pool.
func GCSCredentials(pool, issuer string) ([]byte, error) {
	//nolint:gosec // G101: the file names a token path and holds no secret.
	credentials := map[string]any{
		"type":               "external_account",
		"audience":           fmt.Sprintf("identitynamespace:%s:%s", pool, issuer),
		"subject_token_type": "urn:ietf:params:oauth:token-type:jwt",
		"token_url":          "https://sts.googleapis.com/v1/token",
		"credential_source": map[string]any{
			"file":   projectedPath(TokenKey),
			"format": map[string]string{"type": "text"},
		},
	}
	return json.Marshal(credentials)
}

// projectedPath is the path in the pod of a key of the web identity Secret.
func projectedPath(key string) string {
	return path.Join(projectedDir, tokenDir, key)
}

func webIdentityVolume(clusterName string, keys ...string) corev1.ProjectedVolumeSource {
	items := make([]corev1.KeyToPath, 0, len(keys))
	for _, key := range keys {
		items = append(items, corev1.KeyToPath{Key: key, Path: path.Join(tokenDir, key)})
	}
	return corev1.ProjectedVolumeSource{
		Sources: []corev1.VolumeProjection{{
			Secret: &corev1.SecretProjection{
				Name:     WebIdentitySecretName(clusterName),
				Items:    items,
				Optional: new(true),
			},
		}},
	}
}
