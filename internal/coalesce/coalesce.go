// Package coalesce collapses concurrent calls for the same key into a single
// in-flight operation.
//
// The operation is given a context that is cancelled only when the last
// waiter has left. It has no deadline of its own; each waiter honours only
// its own context. This suits polling-style work that should keep going for
// as long as anyone still wants the answer.
//
// The operation runs under a span of its own, rooted in a new trace rather
// than parented to whichever caller happened to start it. Every caller's
// active span and the operation's span link to each other, so a caller that
// only waited still records what it was waiting on, and the operation
// records who waited on it. See WithTracer.
package coalesce

import (
	"context"
	"sync"

	"xata/internal/o11y"

	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

const instrumentationName = "xata/internal/coalesce"

// Coalescer runs fn at most once per key at any given time.
type Coalescer[K comparable, V any] struct {
	config

	mu      sync.Mutex
	flights map[K]*flight[V]
	fn      func(ctx context.Context, key K) (V, error)
}

type config struct {
	tracer   trace.Tracer
	spanName string
}

type flight[V any] struct {
	ctx        context.Context
	cancel     context.CancelFunc
	done       chan struct{}
	val        V
	err        error
	numWaiters int
	span       trace.Span
}

// Option configures a Coalescer.
type Option func(*config)

// WithTracer records each flight as a span called spanName, which should
// describe what fn does. Without it the Coalescer traces nothing.
func WithTracer(tracer trace.Tracer, spanName string) Option {
	return func(cfg *config) {
		cfg.tracer = tracer
		cfg.spanName = spanName
	}
}

// New returns a Coalescer that runs fn for each distinct key.
//
// fn must respect ctx: when ctx is cancelled it should return promptly.
// fn's context is derived from the starting caller's context with
// context.WithoutCancel, so it carries that caller's values but is cancelled
// only when the last waiter leaves. It also carries the flight's own span.
func New[K comparable, V any](fn func(ctx context.Context, key K) (V, error), opts ...Option) *Coalescer[K, V] {
	cfg := config{
		tracer: noop.NewTracerProvider().Tracer(instrumentationName),
	}

	for _, opt := range opts {
		opt(&cfg)
	}

	return &Coalescer[K, V]{
		config:  cfg,
		flights: make(map[K]*flight[V]),
		fn:      fn,
	}
}

// Do returns the result of fn for key, joining an in-flight call if one
// exists. It returns ctx.Err() if ctx is done before the result is ready.
// Leaving does not stop fn unless this was the last waiter.
func (c *Coalescer[K, V]) Do(ctx context.Context, key K) (V, error) {
	c.mu.Lock()
	f, ok := c.flights[key]
	if !ok || f.ctx.Err() != nil {
		// No flight, or the existing one was cancelled by its last waiter
		// and hasn't unregistered yet. Start fresh rather than join it.
		f = c.start(ctx, key)
	}
	f.numWaiters++
	c.mu.Unlock()

	// Link the waiter's current span and the flight's span to each other; the
	// flight is shared, and it outlives any one waiter. The waiter's trace
	// then shows what it waited on, and the flight's trace shows who waited.
	if span := trace.SpanFromContext(ctx); span.IsRecording() {
		span.AddLink(trace.Link{SpanContext: f.span.SpanContext()})
		f.span.AddLink(trace.Link{SpanContext: span.SpanContext()})
	}

	select {
	case <-f.done:
		return f.val, f.err
	case <-ctx.Done():
		c.leave(f)
		var zero V
		return zero, ctx.Err()
	}
}

// start registers and launches a new flight for key. The flight's context
// inherits ctx's values but not its cancellation, and carries a span of the
// flight's own rather than the starting caller's.
func (c *Coalescer[K, V]) start(ctx context.Context, key K) *flight[V] {
	// Start a new root span so that the flight is not parented to the caller's
	// span. Remove any deadline from the caller's context so that the flight can
	// run for as long as any waiter is still waiting
	fctx, span := c.tracer.Start(context.WithoutCancel(ctx), c.spanName, trace.WithNewRoot())
	// The flight's context is cancelled only when the last waiter leaves
	fctx, cancel := context.WithCancel(fctx)

	f := &flight[V]{
		ctx:    fctx,
		cancel: cancel,
		done:   make(chan struct{}),
		span:   span,
	}
	c.flights[key] = f

	go func() {
		f.val, f.err = c.fn(fctx, key)
		o11y.RecordSpanResult(span, f.err)
		span.End()

		c.mu.Lock()
		// don't remove a replacement flight that may have started after fn's
		// context was cancelled but before it returned
		if c.flights[key] == f {
			delete(c.flights, key)
		}
		c.mu.Unlock()

		cancel()
		close(f.done)
	}()

	return f
}

// leave records that a waiter has given up. The last one out cancels fn.
func (c *Coalescer[K, V]) leave(f *flight[V]) {
	c.mu.Lock()
	f.numWaiters--
	if f.numWaiters == 0 {
		f.cancel()
	}
	c.mu.Unlock()
}
