package cnpg

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func Test_GetClusterCredentials(t *testing.T) {
	scheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(scheme)
	v1.AddToScheme(scheme)

	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       "xata-clusters",
			Name:            "fakeID-superuser",
			ResourceVersion: "1",
		},
		Data: map[string][]byte{
			v1.BasicAuthUsernameKey: []byte("foo"),
			v1.BasicAuthPasswordKey: []byte("bar"),
		},
	}

	tests := []struct {
		name         string
		clusterID    string
		username     string
		fakeClient   client.Client
		wantError    bool
		wantCreds    *Credentials
		errorMessage string
	}{
		{
			name:       "GetClusterCredentials works",
			clusterID:  "fakeID",
			username:   "superuser",
			fakeClient: fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build(),
			wantError:  false,
			wantCreds: &Credentials{
				SecretVersion: "1",
				Username:      "foo",
				Password:      "bar",
			},
		},
		{
			name:      "GetClusterCredentials works with different username",
			clusterID: "fakeID",
			username:  "anotheruser",
			fakeClient: fake.NewClientBuilder().WithScheme(scheme).WithObjects(&v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:       "xata-clusters",
					Name:            "fakeID-anotheruser",
					ResourceVersion: "1",
				},
				Data: map[string][]byte{
					v1.BasicAuthUsernameKey: []byte("anotherusername"),
					v1.BasicAuthPasswordKey: []byte("anotheruserspassword"),
				},
			}).Build(),
			wantError: false,
			wantCreds: &Credentials{
				SecretVersion: "1",
				Username:      "anotherusername",
				Password:      "anotheruserspassword",
			},
		},
		{
			name:         "GetClusterCredentials fails for non-existing cluster",
			clusterID:    "fakeID",
			username:     "superuser",
			fakeClient:   fake.NewClientBuilder().WithScheme(scheme).Build(),
			wantError:    true,
			errorMessage: "secrets \"fakeID-superuser\" not found",
		},
		{
			name:      "GetClusterCredentials fails for other error",
			clusterID: "fakeID",
			username:  "superuser",
			fakeClient: fake.NewClientBuilder().WithInterceptorFuncs(
				interceptor.Funcs{
					Get: func(ctx context.Context, client client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
						return errors.New("some random error")
					},
				},
			).Build(),
			wantError:    true,
			errorMessage: "some random error",
		},
		{
			name:         "GetClusterCredentials fails for non-existing username in secret",
			clusterID:    "fakeID",
			username:     "anotheruser",
			fakeClient:   fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build(),
			wantError:    true,
			errorMessage: "secrets \"fakeID-anotheruser\" not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			connector := &DefaultConnector{
				KubernetesClient: tt.fakeClient,
			}

			creds, err := connector.GetClusterCredentials(context.Background(), tt.clusterID, "xata-clusters", tt.username)
			if tt.wantError {
				assert.Error(t, err)
				assert.Equal(t, tt.errorMessage, err.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantCreds, creds)
			}
		})
	}
}
