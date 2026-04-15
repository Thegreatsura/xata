package metrics

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func newTestMetrics(t *testing.T) (*GatewayMetrics, *sdkmetric.ManualReader) {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })
	m, err := New(mp.Meter("test"))
	require.NoError(t, err)
	return m, reader
}

func collectMetrics(t *testing.T, reader *sdkmetric.ManualReader) map[string]metricdata.Metrics {
	t.Helper()
	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))
	result := map[string]metricdata.Metrics{}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			result[m.Name] = m
		}
	}
	return result
}

func TestNew(t *testing.T) {
	m, _ := newTestMetrics(t)
	require.NotNil(t, m.connections)
	require.NotNil(t, m.connectionDuration)
	require.NotNil(t, m.requestsTotal)
	require.NotNil(t, m.requestDuration)
	require.NotNil(t, m.clusterReactivation)
}

func TestConnectionTracking(t *testing.T) {
	tests := map[string]struct {
		protocol string
		want     int64
		ops      func(m *GatewayMetrics, ctx context.Context)
	}{
		"single connection": {
			protocol: ProtocolWire,
			want:     1,
			ops: func(m *GatewayMetrics, ctx context.Context) {
				m.ConnectionStart(ctx, ProtocolWire)
			},
		},
		"start and end balanced": {
			protocol: ProtocolWebSocket,
			want:     0,
			ops: func(m *GatewayMetrics, ctx context.Context) {
				m.ConnectionStart(ctx, ProtocolWebSocket)
				m.ConnectionEnd(ctx, ProtocolWebSocket, time.Second)
			},
		},
		"multiple concurrent connections": {
			protocol: ProtocolWire,
			want:     2,
			ops: func(m *GatewayMetrics, ctx context.Context) {
				m.ConnectionStart(ctx, ProtocolWire)
				m.ConnectionStart(ctx, ProtocolWire)
				m.ConnectionStart(ctx, ProtocolWire)
				m.ConnectionEnd(ctx, ProtocolWire, time.Second)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			m, reader := newTestMetrics(t)
			ctx := context.Background()
			tc.ops(m, ctx)

			metrics := collectMetrics(t, reader)
			got := metrics["xata.gateway.connections"]
			sum, ok := got.Data.(metricdata.Sum[int64])
			require.True(t, ok, "expected Sum[int64]")
			require.False(t, sum.IsMonotonic, "connections should be non-monotonic (UpDownCounter)")

			var value int64
			for _, dp := range sum.DataPoints {
				protocolAttr, exists := dp.Attributes.Value(AttrProtocol)
				if exists && protocolAttr.AsString() == tc.protocol {
					value = dp.Value
				}
			}
			require.Equal(t, tc.want, value)
		})
	}
}

func TestConnectionDuration(t *testing.T) {
	m, reader := newTestMetrics(t)
	ctx := context.Background()

	m.ConnectionStart(ctx, ProtocolWire)
	m.ConnectionEnd(ctx, ProtocolWire, 5*time.Second)

	metrics := collectMetrics(t, reader)
	got := metrics["xata.gateway.connection_duration_seconds"]
	hist, ok := got.Data.(metricdata.Histogram[float64])
	require.True(t, ok, "expected Histogram[float64]")
	require.Len(t, hist.DataPoints, 1)
	require.Equal(t, uint64(1), hist.DataPoints[0].Count)
	require.InDelta(t, 5.0, hist.DataPoints[0].Sum, 0.001)
}

func TestConnectionEndExtraAttributes(t *testing.T) {
	m, reader := newTestMetrics(t)
	ctx := context.Background()

	m.ConnectionStart(ctx, ProtocolWebSocket)
	m.ConnectionEnd(ctx, ProtocolWebSocket, 3*time.Second, AttrBranchID.String("branch-123"))

	metrics := collectMetrics(t, reader)
	hist := metrics["xata.gateway.connection_duration_seconds"]
	data, ok := hist.Data.(metricdata.Histogram[float64])
	require.True(t, ok)
	require.Len(t, data.DataPoints, 1)

	wantAttrs := attribute.NewSet(AttrProtocol.String(ProtocolWebSocket), AttrBranchID.String("branch-123"))
	require.True(t, data.DataPoints[0].Attributes.Equals(&wantAttrs))
}

func TestRecordRequest(t *testing.T) {
	tests := map[string]struct {
		success       bool
		wantCount     int64
		wantDuration  float64
		wantSucceeded bool
	}{
		"successful request": {
			success:       true,
			wantCount:     1,
			wantDuration:  0.1,
			wantSucceeded: true,
		},
		"failed request": {
			success:       false,
			wantCount:     1,
			wantDuration:  0.2,
			wantSucceeded: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			m, reader := newTestMetrics(t)
			ctx := context.Background()

			m.RecordRequest(ctx, ProtocolHTTP, tc.success, time.Duration(tc.wantDuration*float64(time.Second)))

			metrics := collectMetrics(t, reader)

			gotCounter := metrics["xata.gateway.requests"]
			sum, ok := gotCounter.Data.(metricdata.Sum[int64])
			require.True(t, ok)
			require.True(t, sum.IsMonotonic, "requests counter should be monotonic")
			require.Len(t, sum.DataPoints, 1)
			require.Equal(t, tc.wantCount, sum.DataPoints[0].Value)

			successAttr, exists := sum.DataPoints[0].Attributes.Value(AttrSuccess)
			require.True(t, exists)
			require.Equal(t, tc.wantSucceeded, successAttr.AsBool())

			gotHist := metrics["xata.gateway.request_duration_seconds"]
			hist, ok := gotHist.Data.(metricdata.Histogram[float64])
			require.True(t, ok)
			require.Len(t, hist.DataPoints, 1)
			require.InDelta(t, tc.wantDuration, hist.DataPoints[0].Sum, 0.001)
		})
	}
}

func TestRecordClusterReactivation(t *testing.T) {
	m, reader := newTestMetrics(t)

	m.RecordClusterReactivation(context.Background(), 3*time.Second)

	metrics := collectMetrics(t, reader)
	got := metrics["xata.gateway.cluster.reactivation_duration_seconds"]
	hist, ok := got.Data.(metricdata.Histogram[float64])
	require.True(t, ok)
	require.Len(t, hist.DataPoints, 1)
	require.Equal(t, uint64(1), hist.DataPoints[0].Count)
	require.InDelta(t, 3.0, hist.DataPoints[0].Sum, 0.001)
}
