package main

import (
	"context"
	"fmt"
	"runtime"
	"time"
)

type BenchmarkRunSummary struct {
	Results []BenchmarkResult `json:"results"`
	Summary SummaryResult     `json:"summary"`
}

type SummaryResult struct {
	AvgDuration    time.Duration `json:"avg_duration"`
	MedianDuration time.Duration `json:"median_duration"`
	AvgNsPerOp     time.Duration `json:"avg_ns_per_op"`
	MedianNsPerOp  time.Duration `json:"median_ns_per_op"`
}

type BenchmarkResult struct {
	N      int           `json:"n,omitempty"`      // The number of iterations.
	T      time.Duration `json:"t,omitempty"`      // The total time taken.
	Failed string        `json:"failed,omitempty"` // The reason the benchmark failed.
}

func (r BenchmarkResult) NsPerOp() int64 {
	if r.N <= 0 {
		return 0
	}
	return r.T.Nanoseconds() / int64(r.N)
}

type Benchmark struct {
	benchTime durationOrCountFlag
}

type benchmarkState struct {
	userCtx context.Context

	timerOn   bool
	start     time.Time
	duration  time.Duration
	N         int
	failed    bool
	failedMsg string
	loop      struct {
		n    uint64
		i    uint64
		done bool
	}
	benchTime durationOrCountFlag
}

// The loopPoison constants can be OR'd into B.loop.i to cause it to fall back
// to the slow path.
const (
	loopPoisonTimer = uint64(1 << (63 - iota))
	// If necessary, add more poison bits here.

	// loopPoisonMask is the set of all loop poison bits. (iota-1) is the index
	// of the bit we just set, from which we recreate that bit mask. We subtract
	// 1 to set all of the bits below that bit, then complement the result to
	// get the mask. Sorry, not sorry.
	loopPoisonMask = ^uint64((1 << (63 - (iota - 1))) - 1)
)

type B interface {
	StartTimer()
	StopTimer()
	ResetTimer()
	Loop() bool
	Fail()
	Fatal(msg string)
}

type durationOrCountFlag struct {
	d time.Duration
	n int
}

func NewBenchmark(d time.Duration, n int) *Benchmark {
	return &Benchmark{
		benchTime: durationOrCountFlag{
			d: d,
			n: n,
		},
	}
}

func NewBenchmarkWithDuration(d time.Duration) *Benchmark {
	return NewBenchmark(d, 0)
}

func NewBenchmarkWithCount(n int) *Benchmark {
	return NewBenchmark(0, n)
}

func (b *Benchmark) Run(ctx context.Context, fn func(B)) BenchmarkResult {
	state := &benchmarkState{
		userCtx:   ctx,
		benchTime: b.benchTime,
	}
	err := state.run(fn)
	if err != nil {
		return BenchmarkResult{
			Failed: err.Error(),
		}
	}

	failMsg := ""
	if state.failed {
		if state.failedMsg != "" {
			failMsg = state.failedMsg
		} else {
			failMsg = "benchmark failure"
		}
	}

	return BenchmarkResult{
		N:      state.N,
		T:      state.duration,
		Failed: failMsg,
	}
}

func (b *benchmarkState) StartTimer() {
	if !b.timerOn {
		b.start = time.Now()
		b.timerOn = true
		b.loop.i &^= loopPoisonTimer
	}
}

func (b *benchmarkState) StopTimer() {
	if b.timerOn {
		b.duration += time.Since(b.start)
		b.timerOn = false
		b.loop.i |= loopPoisonTimer
	}
}

func (b *benchmarkState) ResetTimer() {
	if b.timerOn {
		b.start = time.Now()
	}
	b.duration = 0
}

func (b *benchmarkState) Loop() bool {
	if err := b.userCtx.Err(); err != nil {
		return false
	}

	if b.loop.i < b.loop.n {
		b.loop.i++
		return true
	}
	return b.loopSlowPath()
}

func (b *benchmarkState) Fail() {
	b.failed = true
	panic("benchmark failed")
}

func (b *benchmarkState) Fatal(msg string) {
	b.failedMsg = msg
	b.Fail()
}

func (b *benchmarkState) loopSlowPath() bool {
	// Consistency checks
	if !b.timerOn {
		panic("B.Loop called with timer stopped")
	}
	if b.loop.i&loopPoisonMask != 0 {
		panic(fmt.Sprintf("unknown loop stop condition: %#x", b.loop.i))
	}

	if b.loop.n == 0 {
		// If it's the first call to b.Loop() in the benchmark function.
		// Allows more precise measurement of benchmark loop cost counts.
		// Also initialize target to 1 to kick start loop scaling.
		b.loop.n = 1
		// Within a b.Loop loop, we don't use b.N (to avoid confusion).
		b.N = 0
		b.loop.i++
		b.ResetTimer()
		return true
	}
	// Handles fixed iterations case
	if b.benchTime.n > 0 {
		if b.loop.n < uint64(b.benchTime.n) {
			b.loop.n = uint64(b.benchTime.n)
			b.loop.i++
			return true
		}
		b.StopTimer()
		// Commit iteration count
		b.N = int(b.loop.n)
		b.loop.done = true
		return false
	}
	// Handles fixed time case
	return b.stopOrScaleBLoop()
}

func (b *benchmarkState) Elapsed() time.Duration {
	d := b.duration
	if b.timerOn {
		d += time.Since(b.start)
	}
	return d
}

func (b *benchmarkState) stopOrScaleBLoop() bool {
	t := b.Elapsed()
	if t >= b.benchTime.d {
		// Stop the timer so we don't count cleanup time
		b.StopTimer()
		// Commit iteration count
		b.N = int(b.loop.n)
		b.loop.done = true
		return false
	}
	// Loop scaling
	goalns := b.benchTime.d.Nanoseconds()
	prevIters := int64(b.loop.n)
	b.loop.n = uint64(predictN(goalns, prevIters, t.Nanoseconds(), prevIters))
	if b.loop.n&loopPoisonMask != 0 {
		// The iteration count should never get this high, but if it did we'd be
		// in big trouble.
		panic("loop iteration target overflow")
	}
	b.loop.i++
	return true
}

func (b *benchmarkState) run(fn func(B)) error {
	if b.loop.n > 0 {
		return fmt.Errorf("benchmark already run")
	}

	if err := b.userCtx.Err(); err != nil {
		return err
	}

	if b.benchTime.n > 0 {
		b.runN(fn, b.benchTime.n)
	} else {
		d := b.benchTime.d
		for n := int64(1); !b.failed && b.duration < d && n < 1e9; {
			if err := b.userCtx.Err(); err != nil {
				return err
			}

			last := n
			goalns := d.Nanoseconds()
			prevIters := int64(b.N)
			n = int64(predictN(goalns, prevIters, b.duration.Nanoseconds(), last))
			b.runN(fn, int(n))
		}
	}
	return nil
}

func (b *benchmarkState) runN(fn func(B), n int) {
	go func() {
		if r := recover(); r != nil {
			b.failed = true
		}
	}()

	runtime.GC()
	b.N = n
	b.loop.n = 0
	b.loop.i = 0
	b.loop.done = false

	b.ResetTimer()
	b.StartTimer()
	fn(b)
	b.StopTimer()
}

func predictN(goalns int64, prevIters int64, prevns int64, last int64) int {
	if prevns == 0 {
		// Round up to dodge divide by zero. See https://go.dev/issue/70709.
		prevns = 1
	}

	// Order of operations matters.
	// For very fast benchmarks, prevIters ~= prevns.
	// If you divide first, you get 0 or 1,
	// which can hide an order of magnitude in execution time.
	// So multiply first, then divide.
	n := goalns * prevIters / prevns
	// Run more iterations than we think we'll need (1.2x).
	n += n / 5
	// Don't grow too fast in case we had timing errors previously.
	n = min(n, 100*last)
	// Be sure to run at least one more than last time.
	n = max(n, last+1)
	// Don't run more than 1e9 times. (This also keeps n in int range on 32 bit platforms.)
	n = min(n, 1e9)
	return int(n)
}
