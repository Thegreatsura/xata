// Package metrics holds the metrics of the Branch reconciler.
package metrics

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// MeterName is the instrumentation scope name of the Branch reconciler
// metrics.
const MeterName = "branch-reconciler"

// AttrSuccess is true when the operation returns no error.
const AttrSuccess = attribute.Key("success")

// pgBackRestIdentityDurationBuckets are the histogram boundaries in seconds.
var pgBackRestIdentityDurationBuckets = []float64{
	0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30,
}

// Metrics records the metrics of the Branch reconciler. A nil Metrics records
// nothing.
type Metrics struct {
	pgBackRestIdentityDuration metric.Float64Histogram
}

// New creates the instruments of the reconciler on the meter.
func New(meter metric.Meter) (*Metrics, error) {
	pgBackRestIdentityDuration, err := meter.Float64Histogram("xata.branch_operator.pgbackrest_identity.duration_seconds",
		metric.WithUnit("s"),
		metric.WithDescription("duration of the pgBackRest identity step in a Branch reconcile"),
		metric.WithExplicitBucketBoundaries(pgBackRestIdentityDurationBuckets...))
	if err != nil {
		return nil, err
	}
	return &Metrics{pgBackRestIdentityDuration: pgBackRestIdentityDuration}, nil
}

// RecordPgBackRestIdentity records the duration and the result of the
// pgBackRest identity step
func (m *Metrics) RecordPgBackRestIdentity(ctx context.Context, duration time.Duration, success bool) {
	if m == nil {
		return
	}
	m.pgBackRestIdentityDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(AttrSuccess.Bool(success)))
}
