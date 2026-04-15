package openebs_test

import (
	"context"
	"testing"

	"xata/services/clusters/internal/connectors/openebs"

	openebsv1beta3 "github.com/openebs/openebs-e2e/common/custom_resources/api/types/v1beta3"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	dfake "k8s.io/client-go/discovery/fake"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ktesting "k8s.io/client-go/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestNewConnector(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		resources    []*metav1.APIResourceList
		expectedType openebs.Connector
	}{
		{
			name: "returns a DefaultConnector if OpenEBS CRDs are present",
			resources: []*metav1.APIResourceList{
				{
					GroupVersion: "openebs.io/v1beta3",
					APIResources: []metav1.APIResource{
						{Name: "diskpools"},
					},
				},
			},
			expectedType: &openebs.DefaultConnector{},
		},
		{
			name:         "returns a NoOpConnector if OpenEBS CRDs are not present",
			resources:    []*metav1.APIResourceList{},
			expectedType: &openebs.NoopConnector{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeDiscovery := &dfake.FakeDiscovery{
				Fake: &ktesting.Fake{
					Resources: tt.resources,
				},
			}

			fakeClient := fake.NewClientBuilder().Build()

			connector, err := openebs.NewConnectorWithClients(fakeClient, fakeDiscovery)
			require.NoError(t, err)
			require.IsType(t, tt.expectedType, connector)
		})
	}
}

func TestAvailableSpaceBytes(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(scheme)
	openebsv1beta3.PoolAddToScheme(scheme)
	ctx := context.Background()

	tests := []struct {
		name     string
		pools    []openebsv1beta3.DiskPool
		expected *uint64
	}{
		{
			name: "one DiskPool",
			pools: []openebsv1beta3.DiskPool{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "diskpool-1",
						Namespace: "openebs",
					},
					Status: openebsv1beta3.DiskPoolStatus{
						Available:  1234,
						Capacity:   5678,
						PoolStatus: string(openebs.PoolStatusOnline),
					},
				},
			},
			expected: proto.Uint64(1234),
		},
		{
			name: "two DiskPools",
			pools: []openebsv1beta3.DiskPool{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "diskpool-1",
						Namespace: "openebs",
					},
					Status: openebsv1beta3.DiskPoolStatus{
						Available:  1500,
						Capacity:   5000,
						PoolStatus: string(openebs.PoolStatusOnline),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "diskpool-2",
						Namespace: "openebs",
					},
					Status: openebsv1beta3.DiskPoolStatus{
						Available:  2500,
						Capacity:   8000,
						PoolStatus: string(openebs.PoolStatusOnline),
					},
				},
			},
			expected: proto.Uint64(4000),
		},
		{
			name: "a non-online DiskPool is ignored",
			pools: []openebsv1beta3.DiskPool{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "diskpool-1",
						Namespace: "openebs",
					},
					Status: openebsv1beta3.DiskPoolStatus{
						Available:  1500,
						Capacity:   5000,
						PoolStatus: string(openebs.PoolStatusOnline),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "diskpool-2",
						Namespace: "openebs",
					},
					Status: openebsv1beta3.DiskPoolStatus{
						Available:  2500,
						Capacity:   8000,
						PoolStatus: "SomeOtherStatus",
					},
				},
			},
			expected: proto.Uint64(1500),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Convert pools to client.Object slice
			poolObjects := make([]client.Object, len(tt.pools))
			for i := range tt.pools {
				poolObjects[i] = &tt.pools[i]
			}

			// Build client with objects already in it
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithStatusSubresource(&openebsv1beta3.DiskPool{}).
				WithObjects(poolObjects...).
				Build()

			connector := &openebs.DefaultConnector{KubernetesClient: fakeClient}

			bytes, err := connector.AvailableSpaceBytes(ctx)
			require.NoError(t, err)

			require.Equal(t, tt.expected, bytes)
		})
	}
}
