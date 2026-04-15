package gateway

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTimedWaitGroup_New(t *testing.T) {
	timeout := 5 * time.Second
	d := newTimedWaitGroup(timeout)

	assert.NotNil(t, d)
	assert.Equal(t, int64(0), d.GetCount())
}

func TestTimedWaitGroup_AddAndDone_BasicOperations(t *testing.T) {
	d := newTimedWaitGroup(time.Second)

	// Add a connection
	err := d.Add(1)
	require.NoError(t, err)
	assert.Equal(t, int64(1), d.GetCount())

	// Add another connection
	err = d.Add(1)
	require.NoError(t, err)
	assert.Equal(t, int64(2), d.GetCount())

	// Mark one as done
	d.Done()
	assert.Equal(t, int64(1), d.GetCount())

	// Mark the other as done
	d.Done()
	assert.Equal(t, int64(0), d.GetCount())
}

func TestTimedWaitGroup_AddAfterWait_ShouldError(t *testing.T) {
	d := newTimedWaitGroup(time.Second)

	// Close the drainer by calling Wait
	// No connections, so it returns immediately.
	err := d.Wait(context.Background())
	require.NoError(t, err)

	// Try to add a connection - should fail
	err = d.Add(1)
	assert.ErrorIs(t, err, errDrainerClosed)
	assert.Equal(t, int64(0), d.GetCount())
}

func TestTimedWaitGroup_WaitWithNoConnections_ShouldReturnImmediately(t *testing.T) {
	d := newTimedWaitGroup(time.Second)

	start := time.Now()
	ctx := context.Background()
	err := d.Wait(ctx)
	elapsed := time.Since(start)

	assert.NoError(t, err)
	assert.Less(t, elapsed, 100*time.Millisecond, "Should return immediately when no connections")
}

func TestTimedWaitGroup_WaitWithConnectionsCompletingBeforeTimeout(t *testing.T) {
	d := newTimedWaitGroup(2 * time.Second)

	// Add a connection
	err := d.Add(1)
	require.NoError(t, err)

	// Start waiting in a goroutine
	ctx := context.Background()
	waitDone := make(chan error, 1)
	go func() {
		waitDone <- d.Wait(ctx)
	}()

	// Complete the connection after a short delay
	time.Sleep(100 * time.Millisecond)
	d.Done()

	// Wait should complete quickly
	select {
	case err := <-waitDone:
		assert.NoError(t, err)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Wait took too long when connection completed")
	}
}

func TestTimedWaitGroup_WaitWithTimeout(t *testing.T) {
	timeout := 200 * time.Millisecond
	d := newTimedWaitGroup(timeout)

	// Add a connection that won't complete
	err := d.Add(1)
	require.NoError(t, err)

	start := time.Now()
	ctx := context.Background()
	err = d.Wait(ctx)
	elapsed := time.Since(start)

	// Should timeout
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.True(t, elapsed >= timeout && elapsed < timeout+100*time.Millisecond,
		"Expected timeout around %v, got %v", timeout, elapsed)
}

func TestTimedWaitGroup_WaitWithZeroTimeout(t *testing.T) {
	d := newTimedWaitGroup(0) // No timeout, should return immediately

	// Add a connection (it won't matter as Wait returns immediately)
	err := d.Add(1)
	require.NoError(t, err)

	start := time.Now()
	ctx := context.Background()
	err = d.Wait(ctx)
	elapsed := time.Since(start)

	// Should return immediately with no error
	assert.NoError(t, err)
	assert.Less(t, elapsed, 10*time.Millisecond, "Expected immediate return when drainingTimeout is 0")
}

func TestTimedWaitGroup_WaitWithContextCancellation(t *testing.T) {
	d := newTimedWaitGroup(5 * time.Second) // Long timeout

	// Add a connection
	err := d.Add(1)
	require.NoError(t, err)

	// Create a context that will be cancelled
	ctx, cancel := context.WithCancel(context.Background())

	// Start waiting
	waitDone := make(chan error, 1)
	go func() {
		waitDone <- d.Wait(ctx)
	}()

	// Cancel the context after a short delay
	time.Sleep(50 * time.Millisecond)
	cancel()

	// Should return quickly with context error
	select {
	case err := <-waitDone:
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Wait should have returned quickly after context cancellation")
	}
}

func TestTimedWaitGroup_WaitWithNegativeTimeout_CompletesOnDone(t *testing.T) {
	d := newTimedWaitGroup(-1 * time.Second) // Negative timeout, should wait indefinitely

	// Add a connection
	err := d.Add(1)
	require.NoError(t, err)

	// Start waiting in a goroutine with a context that will not be cancelled
	ctx := context.Background()
	waitDone := make(chan error, 1)
	go func() {
		waitDone <- d.Wait(ctx)
	}()

	// Simulate work and then complete the connection after a delay
	time.Sleep(100 * time.Millisecond)
	d.Done()

	// Wait should complete quickly after connection done
	select {
	case err := <-waitDone:
		assert.NoError(t, err, "Expected Wait to return nil when connections are done")
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Wait took too long when connection completed")
	}

	assert.Equal(t, int64(0), d.GetCount(), "Connection count should be 0 after draining")
}

func TestTimedWaitGroup_ConcurrentAddDone(t *testing.T) {
	d := newTimedWaitGroup(time.Second)

	const numGoroutines = 100
	const operationsPerGoroutine = 10

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Launch many goroutines doing Add/Done operations
	for range numGoroutines {
		go func() {
			defer wg.Done()
			for range operationsPerGoroutine {
				err := d.Add(1)
				if err == nil { // Only call Done if Add succeeded
					time.Sleep(time.Microsecond)
					d.Done()
				}
			}
		}()
	}

	wg.Wait()

	// Connection count should be 0 before calling wait
	assert.Equal(t, int64(0), d.GetCount())
}

func TestTimedWaitGroup_DrainConcurrentConnections(t *testing.T) {
	d := newTimedWaitGroup(1 * time.Second)

	const numConnections = 50
	connectionDone := make(chan struct{}, numConnections)
	var wg sync.WaitGroup

	// Simulate many concurrent connections
	wg.Add(numConnections)
	for range numConnections {
		go func() {
			defer wg.Done()
			err := d.Add(1)
			if err != nil {
				return // Connection rejected
			}

			// Simulate some work
			time.Sleep(10 * time.Millisecond)
			d.Done()
			connectionDone <- struct{}{}
		}()
	}

	// Let some connections start
	time.Sleep(5 * time.Millisecond)

	// Wait for all connections to complete
	ctx := context.Background()
	start := time.Now()
	err := d.Wait(ctx)
	elapsed := time.Since(start)

	wg.Wait()

	// Should complete without timeout
	assert.NoError(t, err)
	assert.Less(t, elapsed, 500*time.Millisecond, "Should complete quickly when all connections finish")

	// Final connection count should be 0
	assert.Equal(t, int64(0), d.GetCount())
}

func TestTimedWaitGroup_MultipleWaitCallsAreSafe(t *testing.T) {
	d := newTimedWaitGroup(time.Second)
	ctx := context.Background()

	// Multiple waits should be safe
	err1 := d.Wait(ctx)
	err2 := d.Wait(ctx)
	err3 := d.Wait(ctx)

	assert.NoError(t, err1)
	assert.NoError(t, err2)
	assert.NoError(t, err3)
}
