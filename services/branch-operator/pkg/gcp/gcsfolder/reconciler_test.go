package gcsfolder_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/api/option"
	storage "google.golang.org/api/storage/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"xata/internal/pgbackrest"
	"xata/services/branch-operator/pkg/gcp/gcsfolder"
)

const (
	namespace = "xata-clusters"
	bucket    = "backups"
)

// fakeGCS serves the managed folder calls of the GCS JSON API.
type fakeGCS struct {
	mu       sync.Mutex
	policies map[string]*storage.Policy // by managed folder
	calls    []string
}

func (f *fakeGCS) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, req.Method)

	path := strings.TrimPrefix(req.URL.EscapedPath(), "/storage/v1/b/"+bucket+"/managedFolders")
	folder, iam := strings.CutSuffix(strings.TrimPrefix(path, "/"), "/iam")

	switch {
	case req.Method == http.MethodPost && folder == "":
		var mf storage.ManagedFolder
		_ = json.NewDecoder(req.Body).Decode(&mf)
		if _, ok := f.policies[mf.Name]; ok {
			http.Error(w, `{"error":{"code":409}}`, http.StatusConflict)
			return
		}
		f.policies[mf.Name] = &storage.Policy{Etag: "0"}
		_ = json.NewEncoder(w).Encode(mf)
	case req.Method == http.MethodGet && iam:
		policy, ok := f.policies[unescape(folder)]
		if !ok {
			http.Error(w, `{"error":{"code":404}}`, http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(policy)
	case req.Method == http.MethodPut && iam:
		var policy storage.Policy
		_ = json.NewDecoder(req.Body).Decode(&policy)
		f.policies[unescape(folder)] = &policy
		_ = json.NewEncoder(w).Encode(policy)
	case req.Method == http.MethodDelete && !iam:
		if _, ok := f.policies[unescape(folder)]; !ok {
			http.Error(w, `{"error":{"code":404}}`, http.StatusNotFound)
			return
		}
		delete(f.policies, unescape(folder))
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "unexpected request "+req.Method+" "+req.URL.String(), http.StatusBadRequest)
	}
}

func unescape(s string) string {
	return strings.NewReplacer("%2F", "/", "%3A", ":").Replace(s)
}

func setup(t *testing.T, objs ...client.Object) (*gcsfolder.Reconciler, *fakeGCS) {
	t.Helper()

	gcs := &fakeGCS{policies: map[string]*storage.Policy{}}
	srv := httptest.NewServer(gcs)
	t.Cleanup(srv.Close)

	svc, err := storage.NewService(context.Background(),
		option.WithEndpoint(srv.URL+"/storage/v1/"),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
	)
	require.NoError(t, err)

	return &gcsfolder.Reconciler{
		Client:               fake.NewClientBuilder().WithScheme(clientgoscheme.Scheme).WithObjects(objs...).Build(),
		Storage:              svc,
		ProjectNumber:        "123",
		WorkloadIdentityPool: "project.svc.id.goog",
	}, gcs
}

func reconcile(t *testing.T, r *gcsfolder.Reconciler, name string) {
	t.Helper()
	_, err := r.Reconcile(context.Background(), ctrl.Request{Namespace: namespace, Name: name})
	require.NoError(t, err)
}

func serviceAccount(name string) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{Namespace: namespace, Name: name}
}

func branchServiceAccount(branch string) *corev1.ServiceAccount {
	sa := serviceAccount(pgbackrest.ServiceAccountName(branch))
	sa.OwnerReferences = []metav1.OwnerReference{{Kind: "Branch", Name: branch, Controller: new(true)}}
	sa.Annotations = map[string]string{pgbackrest.GCSBucketAnnotation: bucket}
	return sa
}

func TestReconcile(t *testing.T) {
	t.Parallel()

	folder := gcsfolder.Folder("b123")
	ctx := context.Background()

	t.Run("creates folder and grant, then skips GCS", func(t *testing.T) {
		t.Parallel()
		r, gcs := setup(t, branchServiceAccount("b123"))

		reconcile(t, r, "b123-pgbackrest")

		require.Equal(t, "b123/", folder)
		require.Len(t, gcs.policies[folder].Bindings, 1)
		want := &storage.PolicyBindings{
			Role:    gcsfolder.Role,
			Members: []string{"principal://iam.googleapis.com/projects/123/locations/global/workloadIdentityPools/project.svc.id.goog/subject/ns/xata-clusters/sa/b123-pgbackrest"},
		}
		require.Equal(t, want, gcs.policies[folder].Bindings[0])

		got := &corev1.ServiceAccount{}
		require.NoError(t, r.Client.Get(ctx, client.ObjectKeyFromObject(serviceAccount("b123-pgbackrest")), got))
		require.Contains(t, got.Finalizers, gcsfolder.Finalizer)
		require.Equal(t, folder, got.Annotations[gcsfolder.FolderAnnotation])

		calls := len(gcs.calls)
		reconcile(t, r, "b123-pgbackrest")
		require.Len(t, gcs.calls, calls)
	})

	t.Run("existing folder and grant are kept", func(t *testing.T) {
		t.Parallel()
		r, gcs := setup(t, branchServiceAccount("b123"))

		reconcile(t, r, "b123-pgbackrest")
		// Lose the annotation, as if the reconciler stopped before it wrote it.
		sa := &corev1.ServiceAccount{}
		require.NoError(t, r.Client.Get(ctx, client.ObjectKeyFromObject(serviceAccount("b123-pgbackrest")), sa))
		delete(sa.Annotations, gcsfolder.FolderAnnotation)
		require.NoError(t, r.Client.Update(ctx, sa))

		reconcile(t, r, "b123-pgbackrest")
		require.Len(t, gcs.policies[folder].Bindings, 1)
	})

	t.Run("deletes folder before the ServiceAccount", func(t *testing.T) {
		t.Parallel()
		r, gcs := setup(t, branchServiceAccount("b123"))

		reconcile(t, r, "b123-pgbackrest")
		require.NoError(t, r.Client.Delete(ctx, serviceAccount("b123-pgbackrest")))

		reconcile(t, r, "b123-pgbackrest")
		require.NotContains(t, gcs.policies, folder)
		err := r.Client.Get(ctx, client.ObjectKeyFromObject(serviceAccount("b123-pgbackrest")), &corev1.ServiceAccount{})
		require.True(t, apierrors.IsNotFound(err))
	})

	t.Run("ignores other ServiceAccounts", func(t *testing.T) {
		t.Parallel()
		branchID := serviceAccount("b456-pgbackrest")
		branchID.Labels = map[string]string{"xata.io/branchID": "b456"}
		noBucket := branchServiceAccount("b789")
		noBucket.Annotations = nil
		r, gcs := setup(t, serviceAccount("b123"), branchID, noBucket)

		reconcile(t, r, "b123")
		reconcile(t, r, "b456-pgbackrest")
		reconcile(t, r, "b789-pgbackrest")
		require.Empty(t, gcs.calls)
	})
}

func TestReconcileGrantKeepsOtherBindings(t *testing.T) {
	t.Parallel()
	r, gcs := setup(t, branchServiceAccount("b123"))
	folder := gcsfolder.Folder("b123")

	other := &storage.PolicyBindings{Role: "roles/storage.objectViewer", Members: []string{"principal://restore"}}
	gcs.policies[folder] = &storage.Policy{Etag: "0", Bindings: []*storage.PolicyBindings{other}}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := r.Reconcile(ctx, ctrl.Request{Namespace: namespace, Name: "b123-pgbackrest"})
	require.NoError(t, err)

	require.Len(t, gcs.policies[folder].Bindings, 2)
	require.Equal(t, other, gcs.policies[folder].Bindings[0])
}
