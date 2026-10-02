package reconciler

import (
	"context"
	"fmt"
	"math/rand/v2"
	"time"

	apiv1 "github.com/xataio/xata-cnpg/api/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"xata/services/branch-operator/api/v1alpha1"
	"xata/services/branch-operator/pkg/reconciler/resources"
)

//nolint:gosec // G101: annotation names, not credentials.
const (
	// PgBackRestTokenKey is the key in the web identity Secret that holds the
	// token.
	PgBackRestTokenKey = "token"
	// PgBackRestTokenRequestAnnotation records the request that issued the
	// token in the Secret. A different request, for example after a
	// ServiceAccount is created again or the lifetime changes, gets a new
	// token at once.
	PgBackRestTokenRequestAnnotation = "xata.io/token-request"
	// PgBackRestTokenExpirationAnnotation records when the token in the Secret
	// expires.
	PgBackRestTokenExpirationAnnotation = "xata.io/token-expiration"
	// PgBackRestTokenRefreshAnnotation records when the operator replaces the
	// token in the Secret, so it does not have to parse the token.
	PgBackRestTokenRefreshAnnotation = "xata.io/token-refresh-after"

	// pgBackRestTokenAudience is the audience AWS STS requires in a web
	// identity token.
	pgBackRestTokenAudience = "sts.amazonaws.com"
	// pgBackRestTokenLifetime is the requested token lifetime, the same as the
	// EKS IRSA webhook default. AWS STS does not check whether the
	// ServiceAccount still exists, so the lifetime is also the time a token
	// stays valid after the Branch no longer uses it. The API server can
	// issue a shorter token; the operator uses the returned expiration.
	pgBackRestTokenLifetime = 24 * time.Hour
	// pgBackRestTokenRefreshFraction is the fraction of the token lifetime
	// after which the operator requests a new token. The kubelet uses the
	// same value for projected ServiceAccount tokens.
	pgBackRestTokenRefreshFraction = 0.8
	// pgBackRestTokenRefreshJitter is the maximum random time by which a
	// refresh comes earlier, so Branches created together do not refresh
	// together.
	pgBackRestTokenRefreshJitter = 30 * time.Minute
	// pgBackRestTokenRetryDelay is the minimum time before the operator tries
	// again after a failed refresh while the current token is still valid. A
	// random time up to the same value is added, so failed refreshes do not
	// retry together.
	pgBackRestTokenRetryDelay = 5 * time.Minute
)

// PgBackRestServiceAccountName is the ServiceAccount whose tokens give
// pgBackRest access to the backups of the Branch. The IAM trust policy accepts
// only ServiceAccounts with this suffix, so the CNPG ServiceAccounts, which
// have the cluster name, cannot assume the role.
func PgBackRestServiceAccountName(branchName string) string {
	return branchName + "-pgbackrest"
}

// PgBackRestWebIdentitySecretName is the Secret that holds the web identity
// token for the pgBackRest ServiceAccount of the Branch that uses the cluster.
// The name comes from the cluster and not from the Branch: pool clusters are
// created before a Branch claims them, and their pod spec cannot change
// afterwards.
func PgBackRestWebIdentitySecretName(clusterName string) string {
	return clusterName + "-pgbackrest-web-identity"
}

// reconcilePgBackRestIdentity keeps a ServiceAccount for the Branch and a
// token for it in a Secret named after the cluster of the Branch. It refreshes
// the token before it expires and returns the time until the next refresh.
//
// The Branch owns the ServiceAccount, so the ServiceAccount follows the Branch
// lifecycle. The Cluster owns the Secret, so Kubernetes deletes the Secret
// with the Cluster: when the Branch hibernates, moves to another cluster, or
// is deleted. A token for the Branch never stays in the Secret of a cluster
// the Branch no longer uses.
//
// TODO(Martin): nothing mounts the web identity Secret yet. pgBackRest
// continues to get credentials from the node role until the instance pods
// mount the Secret and use repo-s3-key-type=web-id with BackupsAWSRoleARN.
func (r *BranchReconciler) reconcilePgBackRestIdentity(
	ctx context.Context,
	branch *v1alpha1.Branch,
) (time.Duration, error) {
	if !r.usesPgBackRestBranchIdentity(branch) || !branch.HasClusterName() {
		return 0, nil
	}

	sa, err := r.reconcilePgBackRestServiceAccount(ctx, branch)
	if err != nil {
		return 0, fmt.Errorf("reconcile pgbackrest ServiceAccount: %w", err)
	}

	// The Cluster was applied earlier in this reconcile, but a new Cluster can
	// be missing from the cache for a short time. The error makes
	// controller-runtime try again with backoff.
	cluster := &apiv1.Cluster{}
	key := client.ObjectKey{Name: branch.ClusterName(), Namespace: r.ClustersNamespace}
	if err := r.Get(ctx, key, cluster); err != nil {
		return 0, fmt.Errorf("get Cluster %s: %w", key.Name, err)
	}

	refreshIn, err := r.reconcilePgBackRestToken(ctx, branch, cluster, sa)
	if err != nil {
		return 0, fmt.Errorf("reconcile pgbackrest web identity Secret: %w", err)
	}
	return refreshIn, nil
}

// usesPgBackRestBranchIdentity reports whether the Branch needs a pgBackRest
// ServiceAccount and token.
func (r *BranchReconciler) usesPgBackRestBranchIdentity(branch *v1alpha1.Branch) bool {
	return r.BackupsAWSRoleARN != "" &&
		r.CloudProvider == "aws" &&
		r.BackupsEndpoint == "" &&
		resources.UsesAWSIAM(branch.Spec.BackupSpec)
}

func (r *BranchReconciler) reconcilePgBackRestServiceAccount(
	ctx context.Context,
	branch *v1alpha1.Branch,
) (*corev1.ServiceAccount, error) {
	sa := &corev1.ServiceAccount{
		Name:      PgBackRestServiceAccountName(branch.Name),
		Namespace: r.ClustersNamespace,
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, sa, func() error {
		if err := controllerutil.SetControllerReference(branch, sa, r.Scheme); err != nil {
			return err
		}
		ensureLabels(sa, branch.Spec.InheritedMetadata)
		// The token is only for pgBackRest, which reads it from the Secret.
		sa.AutomountServiceAccountToken = new(false)
		return nil
	})

	return sa, err
}

// reconcilePgBackRestToken writes a new token to the Secret when the Secret
// has no token, a token from a different request, or a token that is due for
// refresh. When a refresh fails and the current token is still valid, it keeps
// the token and tries again later, as the kubelet does.
func (r *BranchReconciler) reconcilePgBackRestToken(
	ctx context.Context,
	branch *v1alpha1.Branch,
	cluster *apiv1.Cluster,
	sa *corev1.ServiceAccount,
) (time.Duration, error) {
	secret := &corev1.Secret{
		Name:      PgBackRestWebIdentitySecretName(cluster.Name),
		Namespace: r.ClustersNamespace,
	}

	tr := &authenticationv1.TokenRequest{
		// The API server rejects the request when the ServiceAccount was
		// deleted and created again after the operator read it.
		UID: sa.UID,
		Spec: authenticationv1.TokenRequestSpec{
			Audiences:         []string{pgBackRestTokenAudience},
			ExpirationSeconds: new(int64(pgBackRestTokenLifetime.Seconds())),
		},
	}
	request := pgBackRestTokenRequestKey(sa, tr)

	// Request the token before the write and not in the CreateOrUpdate mutate
	// function, so the mutate function has no side effects.
	if err := r.Get(ctx, client.ObjectKeyFromObject(secret), secret); client.IgnoreNotFound(err) != nil {
		return 0, fmt.Errorf("get Secret %s: %w", secret.Name, err)
	}
	now := time.Now()
	current := currentPgBackRestToken(secret, request)
	next := current.refreshAt
	issued := false // true when tr has a new token to write to the secret
	if !current.valid || !now.Before(current.refreshAt) {
		if err := r.SubResource("token").Create(ctx, sa, tr); err != nil {
			if !current.valid || !now.Before(current.expiresAt) {
				return 0, fmt.Errorf("request token for ServiceAccount %s: %w", sa.Name, err)
			}
			ctrl.LoggerFrom(ctx).Error(err, "refreshing pgbackrest token, keeping the current token",
				"serviceAccount", sa.Name, "expiresAt", current.expiresAt)
			next = now.Add(pgBackRestTokenRetryDelay + randDuration(pgBackRestTokenRetryDelay))
			if next.After(current.expiresAt) {
				next = current.expiresAt
			}
		} else {
			issued = true
			next = pgBackRestRefreshTime(now, tr.Status.ExpirationTimestamp.Time)
		}
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		// A plain owner reference and not a controller reference, so the CNPG
		// operator does not reconcile the Cluster when the Secret changes.
		if err := controllerutil.SetOwnerReference(cluster, secret, r.Scheme); err != nil {
			return err
		}
		ensureLabels(secret, branch.Spec.InheritedMetadata)
		if !issued {
			return nil
		}
		if secret.Annotations == nil {
			secret.Annotations = map[string]string{}
		}
		secret.Annotations[PgBackRestTokenRequestAnnotation] = request
		secret.Annotations[PgBackRestTokenExpirationAnnotation] = tr.Status.ExpirationTimestamp.UTC().Format(time.RFC3339)
		secret.Annotations[PgBackRestTokenRefreshAnnotation] = next.UTC().Format(time.RFC3339)
		secret.Data = map[string][]byte{PgBackRestTokenKey: []byte(tr.Status.Token)}
		return nil
	})
	if err != nil {
		return 0, err
	}

	return max(time.Until(next), time.Second), nil
}

// pgBackRestTokenRequestKey identifies the token request, like the cache key
// of the kubelet token manager.
func pgBackRestTokenRequestKey(sa *corev1.ServiceAccount, tr *authenticationv1.TokenRequest) string {
	return fmt.Sprintf("serviceAccount=%s,uid=%s,audiences=%v,expirationSeconds=%d",
		sa.Name, sa.UID, tr.Spec.Audiences, *tr.Spec.ExpirationSeconds)
}

type pgBackRestToken struct {
	// valid is false when the Secret has no token from the same request.
	valid     bool
	expiresAt time.Time
	refreshAt time.Time
}

// currentPgBackRestToken reads the token state from the Secret annotations.
func currentPgBackRestToken(secret *corev1.Secret, request string) pgBackRestToken {
	if len(secret.Data[PgBackRestTokenKey]) == 0 ||
		secret.Annotations[PgBackRestTokenRequestAnnotation] != request {
		return pgBackRestToken{}
	}
	expiresAt, err := time.Parse(time.RFC3339, secret.Annotations[PgBackRestTokenExpirationAnnotation])
	if err != nil {
		return pgBackRestToken{}
	}
	refreshAt, err := time.Parse(time.RFC3339, secret.Annotations[PgBackRestTokenRefreshAnnotation])
	if err != nil {
		return pgBackRestToken{}
	}
	return pgBackRestToken{valid: true, expiresAt: expiresAt, refreshAt: refreshAt}
}

// pgBackRestRefreshTime returns the time after the refresh fraction of the
// token lifetime, less a random jitter.
func pgBackRestRefreshTime(issuedAt, expiresAt time.Time) time.Time {
	lifetime := expiresAt.Sub(issuedAt)
	jitter := min(randDuration(pgBackRestTokenRefreshJitter), lifetime/10)
	return issuedAt.Add(time.Duration(float64(lifetime)*pgBackRestTokenRefreshFraction) - jitter)
}

// randDuration returns a random duration in [0, d).
func randDuration(d time.Duration) time.Duration {
	if d <= 0 {
		return 0
	}
	//nolint:gosec // jitter does not need a secure random source
	return rand.N(d)
}
