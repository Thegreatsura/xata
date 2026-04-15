package o11y

import (
	"context"

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

// Wrap is a helper that wraps a function call in a span. This is useful for cases where you don't
// want to create a tonne of specific-wrappers in order to instrument a function, or when you don't
// want to modify a function to report itself.
func Wrap[T any](
	ctx context.Context,
	tracer trace.Tracer,
	name string,
	f func(ctx context.Context) (T, error),
) (t T, err error) {
	ctx, span := tracer.Start(ctx, name)
	defer CloseSpan(span, &err)

	return f(ctx)
}
