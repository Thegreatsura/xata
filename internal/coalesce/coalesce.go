// Package coalesce collapses concurrent calls for the same key into a single
// in-flight operation.
//
// The operation is given a context that is cancelled only when the last
// waiter has left. It has no deadline of its own; each waiter honours only
// its own context. This suits polling-style work that should keep going for
// as long as anyone still wants the answer.
//
// The operation's context carries the values (trace span, logger, etc.) of
// the caller that started it, but not that caller's cancellation or
// deadline. Callers that join an existing operation contribute nothing to
// its context.
package coalesce

import (
	"context"
	"sync"
)

// Coalescer runs fn at most once per key at any given time.
type Coalescer[K comparable, V any] struct {
	mu      sync.Mutex
	flights map[K]*flight[V]
	fn      func(ctx context.Context, key K) (V, error)
}

type flight[V any] struct {
	ctx        context.Context
	cancel     context.CancelFunc
	done       chan struct{}
	val        V
	err        error
	numWaiters int
}

// New returns a Coalescer that runs fn for each distinct key.
//
// fn must respect ctx: when ctx is cancelled it should return promptly.
// fn's context is derived from the starting caller's context with
// context.WithoutCancel, so it carries that caller's values but is cancelled
// only when the last waiter leaves.
func New[K comparable, V any](fn func(ctx context.Context, key K) (V, error)) *Coalescer[K, V] {
	return &Coalescer[K, V]{
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
// inherits ctx's values but not its cancellation
func (c *Coalescer[K, V]) start(ctx context.Context, key K) *flight[V] {
	fctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	f := &flight[V]{ctx: fctx, cancel: cancel, done: make(chan struct{})}
	c.flights[key] = f

	go func() {
		f.val, f.err = c.fn(fctx, key)

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
