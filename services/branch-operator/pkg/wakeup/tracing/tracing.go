// Package tracing holds the span names and attribute keys emitted by the
// wakeup reconciler, so that call sites and dashboards share one definition.
package tracing

import (
	"context"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"xata/internal/o11y"
)

// TracerName is the instrumentation scope name of the wakeup reconciler's
// spans
const TracerName = "wakeup-reconciler"

// Span names
const (
	SpanReconcile        = "wakeup_reconcile"
	SpanPasswordSyncWait = "wakeup_password_sync_wait"
)

// Span attribute keys
const (
	AttrWakeupRequest       = attribute.Key("wakeuprequest")
	AttrBranch              = attribute.Key("branch")
	AttrCluster             = attribute.Key("cluster")
	AttrXVol                = attribute.Key("xvol")
	AttrPasswordSync        = attribute.Key("password_sync") // Skip or Wait
	AttrPasswordSyncOutcome = attribute.Key("password_sync_outcome")
)

// Values for AttrPasswordSyncOutcome
const (
	PasswordSyncOutcomeSynced   = "synced"
	PasswordSyncOutcomeTimedOut = "timed_out"
)

// WithSpanIDs returns the logger with the span's trace and span IDs added as
// values, so that log lines can be matched to the trace. A span from the noop
// tracer has an invalid span context and the logger is returned unchanged.
func WithSpanIDs(log logr.Logger, span trace.Span) logr.Logger {
	sc := span.SpanContext()
	if !sc.IsValid() {
		return log
	}

	traceKey, traceID := o11y.PlainIDStyle.TraceID(sc.TraceID())
	spanKey, spanID := o11y.PlainIDStyle.SpanID(sc.SpanID())
	return log.WithValues(traceKey, traceID, spanKey, spanID)
}

// Tracer returns the reconciler's Tracer from the o11y on the context
func Tracer(ctx context.Context) trace.Tracer {
	return o11y.Ctx(ctx).Tracer(TracerName)
}
