package o11y

import (
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// CloseSpan closes a span and records the given error if not nil.
func CloseSpan(span trace.Span, err *error) {
	RecordSpanResult(span, *err)
	span.End()
}

func RecordSpanResult(span trace.Span, err error) {
	if span == nil {
		return
	}

	if err == nil {
		return
	}

	span.RecordError(err)
	span.SetStatus(codes.Error, "")
}
