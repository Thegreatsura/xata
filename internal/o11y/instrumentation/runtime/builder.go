package runtime

import (
	"context"
	"math"

	"go.opentelemetry.io/otel/metric"
)

type meterBuilder struct {
	instrumentRegistry
}

func newMeterBuilder(meter metric.Meter) meterBuilder {
	return meterBuilder{
		instrumentRegistry: instrumentRegistry{
			meter: meter,
		},
	}
}

func (m *meterBuilder) Batch(snapshot func(context.Context, metric.Observer)) batchBuilder {
	bb := batchBuilder{&m.instrumentRegistry}
	m.fns = append(m.fns, snapshot)
	return bb
}

type instrumentRegistry struct {
	meter metric.Meter
	err   error

	observables []metric.Observable
	fns         []func(context.Context, metric.Observer)
}

func (m *instrumentRegistry) IntCounterFn(name string, fn func() int, opts ...metric.Int64ObservableCounterOption) {
	m.Int64CounterFn(name, func() int64 { return int64(fn()) }, opts...)
}

func (m *instrumentRegistry) Int64CounterFn(name string, fn func() int64, opts ...metric.Int64ObservableCounterOption) {
	if m.err != nil {
		return
	}

	c, err := m.meter.Int64ObservableCounter(name, opts...)
	if err != nil {
		m.err = err
		return
	}

	m.observables = append(m.observables, c)
	m.fns = append(m.fns, func(ctx context.Context, o metric.Observer) {
		o.ObserveInt64(c, fn())
	})
}

func (m *instrumentRegistry) IntGaugeFn(name string, fn func() int, opts ...metric.Int64ObservableGaugeOption) {
	m.Int64GaugeFn(name, func() int64 { return int64(fn()) }, opts...)
}

func (m *instrumentRegistry) Int64GaugeFn(name string, fn func() int64, opts ...metric.Int64ObservableGaugeOption) {
	if m.err != nil {
		return
	}

	g, err := m.meter.Int64ObservableGauge(name, opts...)
	if err != nil {
		m.err = err
		return
	}

	m.observables = append(m.observables, g)
	m.fns = append(m.fns, func(ctx context.Context, o metric.Observer) {
		o.ObserveInt64(g, fn())
	})
}

func (m *instrumentRegistry) Uint64CounterFn(name string, fn func() uint64, opts ...metric.Int64ObservableCounterOption) {
	m.Int64CounterFn(name, func() int64 { return int64(fn() & math.MaxInt64) }, opts...)
}

func (m *instrumentRegistry) Float64GaugeFn(name string, fn func() float64, opts ...metric.Float64ObservableGaugeOption) {
	if m.err != nil {
		return
	}

	c, err := m.meter.Float64ObservableGauge(name, opts...)
	if err != nil {
		m.err = err
		return
	}

	m.observables = append(m.observables, c)
	m.fns = append(m.fns, func(ctx context.Context, o metric.Observer) {
		o.ObserveFloat64(c, fn())
	})
}

func (m *instrumentRegistry) Uint64GaugeFn(name string, fn func() uint64, opts ...metric.Int64ObservableGaugeOption) {
	m.Int64GaugeFn(name, func() int64 { return int64(fn() & math.MaxInt64) }, opts...)
}

func (m *instrumentRegistry) Uint32GaugeFn(name string, fn func() uint32, opts ...metric.Int64ObservableGaugeOption) {
	m.Int64GaugeFn(name, func() int64 { return int64(fn()) }, opts...)
}

type batchBuilder struct {
	*instrumentRegistry
}

func (bb *batchBuilder) Int64CounterFrom(name string, v *int64, opts ...metric.Int64ObservableCounterOption) {
	bb.Int64CounterFn(name, func() int64 { return *v }, opts...)
}

func (bb *batchBuilder) Uint64CounterFrom(name string, v *uint64, opts ...metric.Int64ObservableCounterOption) {
	bb.Uint64CounterFn(name, func() uint64 { return *v }, opts...)
}

func (bb *batchBuilder) Uint32GaugeFrom(name string, v *uint32, opts ...metric.Int64ObservableGaugeOption) {
	bb.Uint32GaugeFn(name, func() uint32 { return *v }, opts...)
}

func (bb *batchBuilder) Uint64GaugeFrom(name string, v *uint64, opts ...metric.Int64ObservableGaugeOption) {
	bb.Uint64GaugeFn(name, func() uint64 { return *v }, opts...)
}

func (bb *batchBuilder) Int64GaugeFrom(name string, v *int64, opts ...metric.Int64ObservableGaugeOption) {
	bb.Int64GaugeFn(name, func() int64 { return *v }, opts...)
}

func (bb *batchBuilder) Float64GaugeFrom(name string, v *float64, opts ...metric.Float64ObservableGaugeOption) {
	bb.Float64GaugeFn(name, func() float64 { return *v }, opts...)
}
