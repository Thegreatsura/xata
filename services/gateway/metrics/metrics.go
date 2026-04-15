package metrics

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type GatewayMetrics struct {
	connections         metric.Int64UpDownCounter
	connectionDuration  metric.Float64Histogram
	requestsTotal       metric.Int64Counter
	requestDuration     metric.Float64Histogram
	clusterReactivation metric.Float64Histogram
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
		metric.WithDescription("duration of cluster reactivation"))
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

func (m *GatewayMetrics) RecordClusterReactivation(ctx context.Context, duration time.Duration) {
	m.clusterReactivation.Record(ctx, duration.Seconds())
}
