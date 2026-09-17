package metrics

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// clusterReactivationBuckets are in seconds. Waking a hibernated cluster takes
// seconds to tens of seconds, and the gateway gives up after the reactivate
// timeout (50s by default), so the default OTel boundaries (0, 5, 10, 25, ...
// 10000) would put nearly every sample in the first two buckets.
var clusterReactivationBuckets = []float64{0.5, 0.75, 1, 1.25, 1.5, 2, 2.5, 3, 4, 5, 7.5, 10, 15, 30, 60, 120, 300}

type GatewayMetrics struct {
	connections         metric.Int64UpDownCounter
	connectionDuration  metric.Float64Histogram
	requestsTotal       metric.Int64Counter
	requestDuration     metric.Float64Histogram
	clusterReactivation metric.Float64Histogram
	bytesForwarded      metric.Int64Counter
}

func New(meter metric.Meter) (*GatewayMetrics, error) {
	m := &GatewayMetrics{}
	var err error

	m.connections, err = meter.Int64UpDownCounter("xata.gateway.connections",
		metric.WithDescription("number of active connections"))
	if err != nil {
		return nil, err
	}

	m.connectionDuration, err = meter.Float64Histogram("xata.gateway.connection_duration_seconds",
		metric.WithUnit("s"),
		metric.WithDescription("duration of connections"))
	if err != nil {
		return nil, err
	}

	m.requestsTotal, err = meter.Int64Counter("xata.gateway.requests",
		metric.WithDescription("total number of requests"))
	if err != nil {
		return nil, err
	}

	m.requestDuration, err = meter.Float64Histogram("xata.gateway.request_duration_seconds",
		metric.WithUnit("s"),
		metric.WithDescription("duration of individual requests"))
	if err != nil {
		return nil, err
	}

	m.clusterReactivation, err = meter.Float64Histogram("xata.gateway.cluster.reactivation_duration_seconds",
		metric.WithUnit("s"),
		metric.WithDescription("duration of cluster reactivation"),
		metric.WithExplicitBucketBoundaries(clusterReactivationBuckets...))
	if err != nil {
		return nil, err
	}

	m.bytesForwarded, err = meter.Int64Counter("xata.gateway.bytes_forwarded",
		metric.WithUnit("By"),
		metric.WithDescription("bytes forwarded between client and backend, by direction"))
	if err != nil {
		return nil, err
	}

	return m, nil
}

func (m *GatewayMetrics) ConnectionStart(ctx context.Context, protocol string) {
	m.connections.Add(ctx, 1, metric.WithAttributes(AttrProtocol.String(protocol)))
}

func (m *GatewayMetrics) ConnectionEnd(ctx context.Context, protocol string, duration time.Duration, attrs ...attribute.KeyValue) {
	m.connections.Add(ctx, -1, metric.WithAttributes(AttrProtocol.String(protocol)))
	durationAttrs := make([]attribute.KeyValue, 0, len(attrs)+1)
	durationAttrs = append(durationAttrs, AttrProtocol.String(protocol))
	durationAttrs = append(durationAttrs, attrs...)
	m.connectionDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(durationAttrs...))
}

func (m *GatewayMetrics) RecordRequest(ctx context.Context, protocol string, success bool, duration time.Duration, attrs ...attribute.KeyValue) {
	allAttrs := make([]attribute.KeyValue, 0, len(attrs)+2)
	allAttrs = append(allAttrs, AttrProtocol.String(protocol), AttrSuccess.Bool(success))
	allAttrs = append(allAttrs, attrs...)
	m.requestsTotal.Add(ctx, 1, metric.WithAttributes(allAttrs...))
	m.requestDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(allAttrs...))
}

// RecordClusterReactivation records the duration and outcome of a reactivation.
// A nil receiver is a no-op so a dialer can run without metrics.
func (m *GatewayMetrics) RecordClusterReactivation(ctx context.Context, duration time.Duration, pool bool, success bool, errorType string) {
	if m == nil {
		return
	}
	attrs := make([]attribute.KeyValue, 0, 3)
	attrs = append(attrs, AttrSuccess.Bool(success), AttrPool.Bool(pool))
	if !success && errorType != "" {
		attrs = append(attrs, AttrErrorType.String(errorType))
	}
	m.clusterReactivation.Record(ctx, duration.Seconds(), metric.WithAttributes(attrs...))
}

// RecordBytesForwarded records the bytes copied in one direction of a wire
// session. A nil receiver is a no-op so a session can run without metrics.
func (m *GatewayMetrics) RecordBytesForwarded(ctx context.Context, direction string, n int64) {
	if m == nil || n <= 0 {
		return
	}
	m.bytesForwarded.Add(ctx, n, metric.WithAttributes(AttrDirection.String(direction)))
}
