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
//
// The observed distribution is bimodal: ~90% of wakeups land between 0.5s
// and 3s with the mode around 1-1.25s, there is a near-empty valley between
// 5s and 10s, and a second mode between 10s and 30s. Boundaries are 100ms
// apart around the first mode, coarser through the valley, 5s apart across
// the second mode, and end at the timeout region (45s, 60s, 120s) so the
// +Inf bucket only holds pathological cases.
var clusterReactivationBuckets = []float64{
	0.25, 0.5, 0.75, 0.9, 1, 1.1, 1.2, 1.3, 1.4, 1.5, 1.75, 2, 2.5, 3,
	4, 5, 7.5, 10, 12.5, 15, 20, 25, 30, 45, 60, 120,
}

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
// instanceSize is the size of the reactivated cluster and is omitted from the
// attributes when empty. A nil receiver is a no-op so a dialer can run without
// metrics.
func (m *GatewayMetrics) RecordClusterReactivation(ctx context.Context, duration time.Duration, pool bool, instanceSize string, success bool, errorType string) {
	if m == nil {
		return
	}
	attrs := make([]attribute.KeyValue, 0, 4)
	attrs = append(attrs, AttrSuccess.Bool(success), AttrPool.Bool(pool))
	if instanceSize != "" {
		attrs = append(attrs, AttrInstanceSize.String(instanceSize))
	}
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
