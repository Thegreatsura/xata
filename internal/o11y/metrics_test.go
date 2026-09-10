package o11y

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
)

// recordingExporter keeps every export and records whether any of them
// arrived after the exporter was shut down.
type recordingExporter struct {
	mu                  sync.Mutex
	exports             []metricdata.ResourceMetrics
	shutdown            bool
	exportAfterShutdown bool
}

func (e *recordingExporter) Temporality(kind sdkmetric.InstrumentKind) metricdata.Temporality {
	return sdkmetric.DefaultTemporalitySelector(kind)
}

func (e *recordingExporter) Aggregation(kind sdkmetric.InstrumentKind) sdkmetric.Aggregation {
	return sdkmetric.DefaultAggregationSelector(kind)
}

func (e *recordingExporter) Export(_ context.Context, rm *metricdata.ResourceMetrics) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.shutdown {
		e.exportAfterShutdown = true
	}
	e.exports = append(e.exports, *rm)
	return nil
}

func (e *recordingExporter) ForceFlush(context.Context) error { return nil }

func (e *recordingExporter) Shutdown(context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.shutdown = true
	return nil
}

// counterValue sums the exported data points of the given counter.
func (e *recordingExporter) counterValue(name string) int64 {
	e.mu.Lock()
	defer e.mu.Unlock()

	var total int64
	for _, rm := range e.exports {
		for _, sm := range rm.ScopeMetrics {
			for _, metric := range sm.Metrics {
				if metric.Name != name {
					continue
				}
				sum, ok := metric.Data.(metricdata.Sum[int64])
				if !ok {
					continue
				}
				for _, dp := range sum.DataPoints {
					total += dp.Value
				}
			}
		}
	}
	return total
}

// newTestMetrics builds a metrics pipeline whose periodic collection never
// fires during the test, so only the explicit flush paths export data.
func newTestMetrics(t *testing.T, out metricExporter) *metrics {
	t.Helper()

	logger := zerolog.Nop()
	m := newMetrics(&logger, resource.Empty(), out, time.Hour)
	t.Cleanup(m.controllersStop.Cancel)

	return m
}

func TestMetricsFlushPendingData(t *testing.T) {
	const counterName = "test.hits"

	tests := map[string]struct {
		flush func(t *testing.T, m *metrics, o *O)
	}{
		"closing a provider exports what it recorded": {
			flush: func(t *testing.T, _ *metrics, o *O) {
				o.Close(t.Context())
			},
		},
		"shutdown exports before the exporter is shut down": {
			flush: func(t *testing.T, m *metrics, _ *O) {
				require.NoError(t, m.shutdown(context.Background()))
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			exporter := &recordingExporter{}
			m := newTestMetrics(t, exporter)

			provider := m.Provider("test", name)
			o := &O{system: &System{metrics: m}, meterProvider: provider}

			counter, err := provider.Meter("test").Int64Counter(counterName)
			require.NoError(t, err)
			counter.Add(ctx, 3)

			test.flush(t, m, o)

			want := int64(3)
			got := exporter.counterValue(counterName)
			require.Equal(t, want, got)
			require.False(t, exporter.exportAfterShutdown, "export must happen before the exporter shuts down")
		})
	}
}

func TestMetricsShutdownWithoutExporter(t *testing.T) {
	m := newTestMetrics(t, nil)
	m.Provider("test", "no-exporter")

	require.NoError(t, m.shutdown(context.Background()))
}
