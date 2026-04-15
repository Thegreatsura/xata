package ipfiltering

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const testNamespace = "test-ns"

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	return scheme
}

func newConfigMap(data map[string]IPFilteringConfig) *v1.ConfigMap {
	raw, _ := json.Marshal(data)
	return &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            ConfigMapName,
			Namespace:       testNamespace,
			UID:             "test-uid",
			ResourceVersion: "1",
		},
		Data: map[string]string{
			ConfigMapKey: string(raw),
		},
	}
}

func getStoredConfigs(t *testing.T, kubeClient client.Client) map[string]IPFilteringConfig {
	t.Helper()
	cm := &v1.ConfigMap{}
	err := kubeClient.Get(context.Background(), types.NamespacedName{
		Name:      ConfigMapName,
		Namespace: testNamespace,
	}, cm)
	require.NoError(t, err)

	var configs map[string]IPFilteringConfig
	require.NoError(t, json.Unmarshal([]byte(cm.Data[ConfigMapKey]), &configs))
	return configs
}

func TestSetBranchIPFiltering(t *testing.T) {
	tests := map[string]struct {
		existing  []client.Object
		branchID  string
		config    IPFilteringConfig
		wantCount int
		want      IPFilteringConfig
	}{
		"creates ConfigMap when none exists": {
			branchID:  "branch-1",
			config:    IPFilteringConfig{Enabled: true, Allowed: []string{"10.0.0.0/8"}},
			wantCount: 1,
			want:      IPFilteringConfig{Enabled: true, Allowed: []string{"10.0.0.0/8"}},
		},
		"adds branch to existing ConfigMap": {
			existing: []client.Object{newConfigMap(map[string]IPFilteringConfig{
				"branch-1": {Enabled: true, Allowed: []string{"10.0.0.0/8"}},
			})},
			branchID:  "branch-2",
			config:    IPFilteringConfig{Enabled: true, Allowed: []string{"192.168.0.0/16"}},
			wantCount: 2,
			want:      IPFilteringConfig{Enabled: true, Allowed: []string{"192.168.0.0/16"}},
		},
		"overwrites existing branch config": {
			existing: []client.Object{newConfigMap(map[string]IPFilteringConfig{
				"branch-1": {Enabled: true, Allowed: []string{"10.0.0.0/8"}},
			})},
			branchID:  "branch-1",
			config:    IPFilteringConfig{Enabled: false, Allowed: []string{}},
			wantCount: 1,
			want:      IPFilteringConfig{Enabled: false, Allowed: []string{}},
		},
		"disabled config": {
			branchID:  "branch-1",
			config:    IPFilteringConfig{Enabled: false, Allowed: []string{}},
			wantCount: 1,
			want:      IPFilteringConfig{Enabled: false, Allowed: []string{}},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			scheme := newScheme(t)
			kubeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tc.existing...).
				Build()

			err := SetBranchIPFiltering(context.Background(), kubeClient, testNamespace, tc.branchID, tc.config)
			require.NoError(t, err)

			configs := getStoredConfigs(t, kubeClient)
			require.Len(t, configs, tc.wantCount)
			require.Equal(t, tc.want, configs[tc.branchID])
		})
	}
}

func TestSetBranchesIPFiltering(t *testing.T) {
	tests := map[string]struct {
		existing  []client.Object
		branchIDs []string
		config    IPFilteringConfig
		wantCount int
	}{
		"empty branch list is no-op": {
			branchIDs: []string{},
			config:    IPFilteringConfig{Enabled: true, Allowed: []string{"10.0.0.0/8"}},
			wantCount: -1, // signal: don't check ConfigMap
		},
		"creates ConfigMap for multiple branches": {
			branchIDs: []string{"branch-1", "branch-2", "branch-3"},
			config:    IPFilteringConfig{Enabled: true, Allowed: []string{"10.0.0.0/8"}},
			wantCount: 3,
		},
		"adds to existing ConfigMap": {
			existing: []client.Object{newConfigMap(map[string]IPFilteringConfig{
				"existing": {Enabled: true, Allowed: []string{"172.16.0.0/12"}},
			})},
			branchIDs: []string{"branch-1", "branch-2"},
			config:    IPFilteringConfig{Enabled: true, Allowed: []string{"10.0.0.0/8"}},
			wantCount: 3,
		},
		"overwrites existing branches": {
			existing: []client.Object{newConfigMap(map[string]IPFilteringConfig{
				"branch-1": {Enabled: true, Allowed: []string{"172.16.0.0/12"}},
			})},
			branchIDs: []string{"branch-1"},
			config:    IPFilteringConfig{Enabled: false, Allowed: []string{}},
			wantCount: 1,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			scheme := newScheme(t)
			kubeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tc.existing...).
				Build()

			err := SetBranchesIPFiltering(context.Background(), kubeClient, testNamespace, tc.branchIDs, tc.config)
			require.NoError(t, err)

			if tc.wantCount == -1 {
				return
			}

			configs := getStoredConfigs(t, kubeClient)
			require.Len(t, configs, tc.wantCount)
			for _, id := range tc.branchIDs {
				require.Equal(t, tc.config, configs[id])
			}
		})
	}
}

func TestGetBranchIPFiltering(t *testing.T) {
	tests := map[string]struct {
		existing []client.Object
		branchID string
		want     IPFilteringConfig
	}{
		"no ConfigMap returns default": {
			branchID: "branch-1",
			want:     DefaultIPFilteringConfig(),
		},
		"empty ConfigMap returns default": {
			existing: []client.Object{&v1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ConfigMapName,
					Namespace: testNamespace,
				},
			}},
			branchID: "branch-1",
			want:     DefaultIPFilteringConfig(),
		},
		"missing branch returns default": {
			existing: []client.Object{newConfigMap(map[string]IPFilteringConfig{
				"other-branch": {Enabled: true, Allowed: []string{"10.0.0.0/8"}},
			})},
			branchID: "branch-1",
			want:     DefaultIPFilteringConfig(),
		},
		"returns existing config": {
			existing: []client.Object{newConfigMap(map[string]IPFilteringConfig{
				"branch-1": {Enabled: true, Allowed: []string{"10.0.0.0/8", "192.168.1.0/24"}},
			})},
			branchID: "branch-1",
			want:     IPFilteringConfig{Enabled: true, Allowed: []string{"10.0.0.0/8", "192.168.1.0/24"}},
		},
		"returns disabled config": {
			existing: []client.Object{newConfigMap(map[string]IPFilteringConfig{
				"branch-1": {Enabled: false, Allowed: []string{}},
			})},
			branchID: "branch-1",
			want:     IPFilteringConfig{Enabled: false, Allowed: []string{}},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			scheme := newScheme(t)
			kubeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tc.existing...).
				Build()

			got, err := GetBranchIPFiltering(context.Background(), kubeClient, testNamespace, tc.branchID)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestDeleteBranchIPFiltering(t *testing.T) {
	tests := map[string]struct {
		existing      []client.Object
		branchID      string
		wantCount     int
		wantRemaining []string
	}{
		"no ConfigMap is no-op": {
			branchID:  "branch-1",
			wantCount: -1,
		},
		"missing branch is no-op": {
			existing: []client.Object{newConfigMap(map[string]IPFilteringConfig{
				"other-branch": {Enabled: true, Allowed: []string{"10.0.0.0/8"}},
			})},
			branchID:      "branch-1",
			wantCount:     1,
			wantRemaining: []string{"other-branch"},
		},
		"deletes existing branch": {
			existing: []client.Object{newConfigMap(map[string]IPFilteringConfig{
				"branch-1": {Enabled: true, Allowed: []string{"10.0.0.0/8"}},
				"branch-2": {Enabled: true, Allowed: []string{"192.168.0.0/16"}},
			})},
			branchID:      "branch-1",
			wantCount:     1,
			wantRemaining: []string{"branch-2"},
		},
		"deletes last branch": {
			existing: []client.Object{newConfigMap(map[string]IPFilteringConfig{
				"branch-1": {Enabled: true, Allowed: []string{"10.0.0.0/8"}},
			})},
			branchID:  "branch-1",
			wantCount: 0,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			scheme := newScheme(t)
			kubeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tc.existing...).
				Build()

			err := DeleteBranchIPFiltering(context.Background(), kubeClient, testNamespace, tc.branchID)
			require.NoError(t, err)

			if tc.wantCount == -1 {
				return
			}

			configs := getStoredConfigs(t, kubeClient)
			require.Len(t, configs, tc.wantCount)
			for _, id := range tc.wantRemaining {
				_, exists := configs[id]
				require.True(t, exists, "expected branch %s to remain", id)
			}
		})
	}
}

func TestSetThenGetRoundTrip(t *testing.T) {
	scheme := newScheme(t)
	kubeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	ctx := context.Background()

	config := IPFilteringConfig{Enabled: true, Allowed: []string{"10.0.0.0/8", "192.168.1.1/32"}}

	require.NoError(t, SetBranchIPFiltering(ctx, kubeClient, testNamespace, "branch-1", config))

	got, err := GetBranchIPFiltering(ctx, kubeClient, testNamespace, "branch-1")
	require.NoError(t, err)
	require.Equal(t, config, got)
}

func TestSetThenDeleteThenGet(t *testing.T) {
	scheme := newScheme(t)
	kubeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	ctx := context.Background()

	config := IPFilteringConfig{Enabled: true, Allowed: []string{"10.0.0.0/8"}}

	require.NoError(t, SetBranchIPFiltering(ctx, kubeClient, testNamespace, "branch-1", config))

	require.NoError(t, DeleteBranchIPFiltering(ctx, kubeClient, testNamespace, "branch-1"))

	got, err := GetBranchIPFiltering(ctx, kubeClient, testNamespace, "branch-1")
	require.NoError(t, err)
	require.Equal(t, DefaultIPFilteringConfig(), got)
}
