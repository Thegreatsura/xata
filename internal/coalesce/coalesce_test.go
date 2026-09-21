package coalesce_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/require"

	"xata/internal/coalesce"
)

// result is what a test hands to a blocked fn call to make it return.
type result struct {
	val string
	err error
}

// call is one invocation of fakeFn. The test controls when it returns by
// sending on release; cancelling ctx also ends it, as a real fn would.
type call struct {
	key     string
	ctx     context.Context
	release chan result
}

// fakeFn records every invocation and blocks each one until the test
// releases it or its context is cancelled.
type fakeFn struct {
	mu    sync.Mutex
	calls []*call
	// ignoreCtx models an fn that is slow to notice cancellation: calls
	// return only when released, even after their context is cancelled.
	ignoreCtx bool
}

func (f *fakeFn) fn(ctx context.Context, key string) (string, error) {
	c := &call{key: key, ctx: ctx, release: make(chan result)}
	f.mu.Lock()
	f.calls = append(f.calls, c)
	f.mu.Unlock()

	done := ctx.Done()
	if f.ignoreCtx {
		done = nil // a nil channel never fires
	}

	select {
	case r := <-c.release:
		return r.val, r.err
	case <-done:
		return "", ctx.Err()
	}
}

func TestDo_ConcurrentCallersShareOneCall(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fake := &fakeFn{}
		c := coalesce.New(fake.fn)

		const waiters = 3
		vals := make([]string, waiters)
		errs := make([]error, waiters)

		// Start waiters concurrently. Each one will block in Do until the test
		// releases the fn call
		for i := range waiters {
			go func() {
				vals[i], errs[i] = c.Do(context.Background(), "k")
			}()
		}

		// Wait for all waiters to be blocked in Do
		synctest.Wait()

		// Assert that there was only one call to fn
		require.Len(t, fake.calls, 1, "concurrent callers should share one fn call")

		// Release the fn call, which should unblock all waiters
		fake.calls[0].release <- result{val: "v"}
		synctest.Wait()

		// Assert that all waiters got the same result and no errors
		for i := range waiters {
			require.NoError(t, errs[i], "waiter %d", i)
			require.Equal(t, "v", vals[i], "waiter %d", i)
		}
	})
}

func TestDo_LastLeaverCancelsFn(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fake := &fakeFn{}
		c := coalesce.New(fake.fn)

		ctxA, cancelA := context.WithCancel(context.Background())
		ctxB, cancelB := context.WithCancel(context.Background())
		var errA, errB error

		// Two waiters join the same flight
		go func() { _, errA = c.Do(ctxA, "k") }()
		go func() { _, errB = c.Do(ctxB, "k") }()
		synctest.Wait()

		// Assert that there was only one call to fn
		require.Len(t, fake.calls, 1)
		fnCtx := fake.calls[0].ctx

		// A gives up. B is still waiting, so fn must keep running
		cancelA()
		synctest.Wait()
		require.ErrorIs(t, errA, context.Canceled)
		require.NoError(t, fnCtx.Err(), "fn should keep running while a waiter remains")

		// B gives up. Nobody wants the result any more, so fn is cancelled
		cancelB()
		synctest.Wait()
		require.ErrorIs(t, errB, context.Canceled)
		require.ErrorIs(t, fnCtx.Err(), context.Canceled, "last leaver should cancel fn")
	})
}

func TestDo_CancelledFlightIsReplacedNotJoined(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fake := &fakeFn{ignoreCtx: true}
		c := coalesce.New(fake.fn)

		// Start a flight for key "k"
		ctxA, cancelA := context.WithCancel(context.Background())
		go func() { _, _ = c.Do(ctxA, "k") }()
		synctest.Wait()

		// Cancel A's context, which should cancel the flight but because fn
		// ignores cancellation, the flight will still be in the coalescer's map
		cancelA()
		synctest.Wait()
		require.Len(t, fake.calls, 1)
		require.ErrorIs(t, fake.calls[0].ctx.Err(), context.Canceled)

		// C arrives while the stale flight is still registered. It must start
		// a fresh flight rather than join one that is already cancelled
		var valC, valD string
		var errC, errD error
		go func() { valC, errC = c.Do(context.Background(), "k") }()
		synctest.Wait()

		// Assert that C started a fresh flight, and did not join the stale one
		require.Len(t, fake.calls, 2, "caller after last leaver should start a fresh flight")
		require.NoError(t, fake.calls[1].ctx.Err(), "fresh flight should not be cancelled")

		// Make the stale flight finally return. Its cleanup must leave C's flight
		// registered
		fake.calls[0].release <- result{err: context.Canceled}
		synctest.Wait()

		// D arrives and must join C's flight, not start a third one
		go func() { valD, errD = c.Do(context.Background(), "k") }()
		synctest.Wait()
		require.Len(t, fake.calls, 2, "stale flight cleanup should not unregister its replacement")

		fake.calls[1].release <- result{val: "v"}
		synctest.Wait()
		require.NoError(t, errC)
		require.Equal(t, "v", valC)
		require.NoError(t, errD)
		require.Equal(t, "v", valD)
	})
}

type ctxKey struct{}

func TestDo_FnInheritsStartingCallerValues(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fake := &fakeFn{}
		c := coalesce.New(fake.fn)

		// The caller that starts the flight carries a value on its context
		ctx := context.WithValue(context.Background(), ctxKey{}, "from-caller")
		go func() { _, _ = c.Do(ctx, "k") }()
		synctest.Wait()

		// fn's context should carry that value
		require.Len(t, fake.calls, 1)
		require.Equal(t, "from-caller", fake.calls[0].ctx.Value(ctxKey{}))

		fake.calls[0].release <- result{val: "v"}
		synctest.Wait()
	})
}

func TestDo_FnErrorReachesAllWaiters(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fake := &fakeFn{}
		c := coalesce.New(fake.fn)

		// Start two waiters for the same key
		var errA, errB error
		go func() { _, errA = c.Do(context.Background(), "k") }()
		go func() { _, errB = c.Do(context.Background(), "k") }()
		synctest.Wait()
		require.Len(t, fake.calls, 1)

		// Make fn fail with an error
		errFn := errors.New("boom")
		fake.calls[0].release <- result{err: errFn}

		// All waiters should see the same error
		synctest.Wait()
		require.ErrorIs(t, errA, errFn)
		require.ErrorIs(t, errB, errFn)
	})
}

func TestDo_DistinctKeysDoNotCoalesce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fake := &fakeFn{}
		c := coalesce.New(fake.fn)

		// Start two waiters for different keys
		var valA, valB string
		go func() { valA, _ = c.Do(context.Background(), "a") }()
		go func() { valB, _ = c.Do(context.Background(), "b") }()
		synctest.Wait()

		require.Len(t, fake.calls, 2, "different keys should each get their own fn call")

		// Release each call with its own key as the value, so each caller can be
		// checked against the key it asked for.
		for _, call := range fake.calls {
			call.release <- result{val: call.key}
		}
		synctest.Wait()
		require.Equal(t, "a", valA)
		require.Equal(t, "b", valB)
	})
}
