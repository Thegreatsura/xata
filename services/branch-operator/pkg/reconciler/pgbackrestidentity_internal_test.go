package reconciler

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	apiv1 "github.com/xataio/xata-cnpg/api/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"xata/services/branch-operator/api/v1alpha1"
)

func TestReconcilePgBackRestToken(t *testing.T) {
	t.Parallel()

	sa := &corev1.ServiceAccount{
		Name: "branch-pgbackrest", Namespace: "xata-clusters", UID: types.UID("sa-uid"),
	}
	request := pgBackRestTokenRequestKey(sa, &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{
			Audiences:         []string{pgBackRestTokenAudience},
			ExpirationSeconds: new(int64(pgBackRestTokenLifetime.Seconds())),
		},
	})
	now := time.Now()
	stamp := func(t time.Time) string { return t.UTC().Format(time.RFC3339) }

	testcases := map[string]struct {
		annotations  map[string]string
		requestErr   error
		wantErr      bool
		wantRequests int
		wantToken    string
		wantNextMin  time.Duration
		wantNextMax  time.Duration
	}{
		"requests a token when the Secret has none": {
			wantRequests: 1,
			wantToken:    "new",
			wantNextMin:  pgBackRestTokenLifetime*8/10 - pgBackRestTokenRefreshJitter - time.Minute,
			wantNextMax:  pgBackRestTokenLifetime * 8 / 10,
		},
		"keeps the token before the refresh time": {
			annotations: map[string]string{
				PgBackRestTokenRequestAnnotation:    request,
				PgBackRestTokenExpirationAnnotation: stamp(now.Add(5 * time.Hour)),
				PgBackRestTokenRefreshAnnotation:    stamp(now.Add(time.Hour)),
			},
			wantToken:   "old",
			wantNextMin: time.Hour - time.Minute,
			wantNextMax: time.Hour,
		},
		"requests a token for a different request": {
			annotations: map[string]string{
				PgBackRestTokenRequestAnnotation:    "serviceAccount=other",
				PgBackRestTokenExpirationAnnotation: stamp(now.Add(5 * time.Hour)),
				PgBackRestTokenRefreshAnnotation:    stamp(now.Add(time.Hour)),
			},
			wantRequests: 1,
			wantToken:    "new",
			wantNextMin:  pgBackRestTokenLifetime*8/10 - pgBackRestTokenRefreshJitter - time.Minute,
			wantNextMax:  pgBackRestTokenLifetime * 8 / 10,
		},
		"keeps a valid token when the refresh fails": {
			annotations: map[string]string{
				PgBackRestTokenRequestAnnotation:    request,
				PgBackRestTokenExpirationAnnotation: stamp(now.Add(5 * time.Hour)),
				PgBackRestTokenRefreshAnnotation:    stamp(now.Add(-time.Minute)),
			},
			requestErr:   errors.New("unavailable"),
			wantRequests: 1,
			wantToken:    "old",
			wantNextMin:  pgBackRestTokenRetryDelay - time.Minute,
			wantNextMax:  2 * pgBackRestTokenRetryDelay,
		},
		"retries no later than the expiration": {
			annotations: map[string]string{
				PgBackRestTokenRequestAnnotation:    request,
				PgBackRestTokenExpirationAnnotation: stamp(now.Add(2 * time.Minute)),
				PgBackRestTokenRefreshAnnotation:    stamp(now.Add(-time.Hour)),
			},
			requestErr:   errors.New("unavailable"),
			wantRequests: 1,
			wantToken:    "old",
			wantNextMin:  time.Second,
			wantNextMax:  2 * time.Minute,
		},
		"fails when the refresh fails and the token expired": {
			annotations: map[string]string{
				PgBackRestTokenRequestAnnotation:    request,
				PgBackRestTokenExpirationAnnotation: stamp(now.Add(-time.Minute)),
				PgBackRestTokenRefreshAnnotation:    stamp(now.Add(-time.Hour)),
			},
			requestErr:   errors.New("unavailable"),
			wantRequests: 1,
			wantErr:      true,
			wantToken:    "old",
		},
	}

	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()

			scheme := runtime.NewScheme()
			require.NoError(t, corev1.AddToScheme(scheme))
			require.NoError(t, v1alpha1.AddToScheme(scheme))
			require.NoError(t, apiv1.AddToScheme(scheme))

			branch := &v1alpha1.Branch{Name: "branch", UID: types.UID("branch-uid")}
			secret := &corev1.Secret{
				Name: "cluster-pgbackrest-web-identity", Namespace: "xata-clusters",
				Annotations: tc.annotations,
			}
			if tc.annotations != nil {
				secret.Data = map[string][]byte{PgBackRestTokenKey: []byte("old")}
			}

			// The fake client writes to the objects it is given
			sa := sa.DeepCopy()
			requests := 0
			cluster := &apiv1.Cluster{
				Name: "cluster", Namespace: "xata-clusters", UID: types.UID("cluster-uid"),
			}
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(branch, cluster, secret, sa).
				WithInterceptorFuncs(interceptor.Funcs{
					SubResourceCreate: func(_ context.Context, _ client.Client, _ string, _ client.Object, sub client.Object, _ ...client.SubResourceCreateOption) error {
						requests++
						if tc.requestErr != nil {
							return tc.requestErr
						}
						tr, ok := sub.(*authenticationv1.TokenRequest)
						if !ok {
							return fmt.Errorf("unexpected subresource %T", sub)
						}
						tr.Status.Token = "new"
						tr.Status.ExpirationTimestamp = metav1.NewTime(time.Now().Add(time.Duration(*tr.Spec.ExpirationSeconds) * time.Second))
						return nil
					},
				}).Build()
			r := &BranchReconciler{Client: c, Scheme: scheme, ClustersNamespace: "xata-clusters"}

			got, err := r.reconcilePgBackRestToken(ctx, branch, cluster, sa)
			require.Equal(t, tc.wantRequests, requests)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.GreaterOrEqual(t, got, tc.wantNextMin)
				require.LessOrEqual(t, got, tc.wantNextMax)
			}

			stored := &corev1.Secret{}
			require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(secret), stored))
			require.Equal(t, tc.wantToken, string(stored.Data[PgBackRestTokenKey]))
		})
	}
}
