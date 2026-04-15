package runtime

import (
	"context"
	goruntime "runtime"
	"sync"
	"time"

	"go.opentelemetry.io/otel"

	"go.opentelemetry.io/otel/metric"
)

type instrumentation struct{}

func Start(meterProvider metric.MeterProvider) error {
	if meterProvider == nil {
		meterProvider = otel.GetMeterProvider()
	}
	if meterProvider == nil {
		return nil
	}

	meter := meterProvider.Meter("xata.runtime")
	mb := newMeterBuilder(meter)
	instr := &instrumentation{}
	instr.register(&mb)
	if mb.err != nil {
		return mb.err
	}

	_, err := meter.RegisterCallback(
		func(ctx context.Context, o metric.Observer) error {
			for _, fn := range mb.fns {
				fn(ctx, o)
			}
			return nil
		},
		mb.observables...,
	)
	return err
}

func (instr *instrumentation) register(mb *meterBuilder) {
	// CPU statistics
	instr.registerCPU(mb)
	instr.registerMemStats(mb)
}

func (instr *instrumentation) registerCPU(mb *meterBuilder) {
	mb.IntGaugeFn("runtime.go.num_cpu", goruntime.NumCPU)
	mb.IntGaugeFn("runtime.go.num_goroutine", goruntime.NumGoroutine)
	mb.Int64CounterFn("runtime.go.num_cgo_call", goruntime.NumCgoCall)
}

func (instr *instrumentation) registerMemStats(mb *meterBuilder) {
	var lastMemStats time.Time
	var memstats goruntime.MemStats
	var mu sync.Mutex

	mu.Lock()
	defer mu.Unlock()

	bb := mb.Batch(func(ctx context.Context, o metric.Observer) {
		mu.Lock()
		defer mu.Unlock()

		now := time.Now()
		if now.Sub(lastMemStats) >= 10*time.Second {
			goruntime.ReadMemStats(&memstats)
			lastMemStats = now
		}
	})

	// General stats
	bb.Uint64GaugeFrom("runtime.go.mem_stats.alloc", &memstats.Alloc)
	bb.Uint64GaugeFrom("runtime.go.mem_stats.total_alloc", &memstats.TotalAlloc)
	bb.Uint64GaugeFrom("runtime.go.mem_stats.sys", &memstats.Sys)
	bb.Uint64GaugeFrom("runtime.go.mem_stats.lookups", &memstats.Lookups)
	bb.Uint64GaugeFrom("runtime.go.mem_stats.mallocs", &memstats.Mallocs)
	bb.Uint64GaugeFrom("runtime.go.mem_stats.frees", &memstats.Frees)
	// Heap memory stats
	bb.Uint64GaugeFrom("runtime.go.mem_stats.heap_alloc", &memstats.HeapAlloc)
	bb.Uint64GaugeFrom("runtime.go.mem_stats.heap_sys", &memstats.HeapSys)
	bb.Uint64GaugeFrom("runtime.go.mem_stats.heap_idle", &memstats.HeapIdle)
	bb.Uint64GaugeFrom("runtime.go.mem_stats.heap_inuse", &memstats.HeapInuse)
	bb.Uint64GaugeFrom("runtime.go.mem_stats.heap_released", &memstats.HeapReleased)
	bb.Uint64GaugeFrom("runtime.go.mem_stats.heap_objects", &memstats.HeapObjects)
	// Stack memory stats
	bb.Uint64GaugeFrom("runtime.go.mem_stats.stack_inuse", &memstats.StackInuse)
	bb.Uint64GaugeFrom("runtime.go.mem_stats.stack_sys", &memstats.StackSys)
	// Off-heap memory statistics
	bb.Uint64GaugeFrom("runtime.go.mem_stats.m_span_inuse", &memstats.MSpanInuse)
	bb.Uint64GaugeFrom("runtime.go.mem_stats.m_span_sys", &memstats.MSpanSys)
	bb.Uint64GaugeFrom("runtime.go.mem_stats.m_cache_inuse", &memstats.MCacheInuse)
	bb.Uint64GaugeFrom("runtime.go.mem_stats.m_cache_sys", &memstats.MCacheSys)
	bb.Uint64GaugeFrom("runtime.go.mem_stats.buck_hash_sys", &memstats.BuckHashSys)
	bb.Uint64GaugeFrom("runtime.go.mem_stats.gc_sys", &memstats.GCSys)
	bb.Uint64GaugeFrom("runtime.go.mem_stats.other_sys", &memstats.OtherSys)
	// Garbage collector statistics
	bb.Uint64GaugeFrom("runtime.go.mem_stats.next_gc", &memstats.NextGC)
	bb.Uint64GaugeFrom("runtime.go.mem_stats.last_gc", &memstats.LastGC)
	bb.Uint64GaugeFrom("runtime.go.mem_stats.pause_total_ns", &memstats.PauseTotalNs)
	bb.Uint32GaugeFrom("runtime.go.mem_stats.num_gc", &memstats.NumGC)
	bb.Uint32GaugeFrom("runtime.go.mem_stats.num_forced_gc", &memstats.NumForcedGC)
	bb.Float64GaugeFrom("runtime.go.mem_stats.gc_cpu_fraction", &memstats.GCCPUFraction)
}
