package orgstatus

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/metric"
)

// registerBacklogMetrics publishes the backlog as two gauges read at collection
// time. The count says how wide a divergence is and the age says how stuck it is.
func (w *Worker) registerBacklogMetrics(meter metric.Meter) error {
	unsynced, err := meter.Int64ObservableGauge("xata.projects.orgstatus.unsynced_organizations",
		metric.WithDescription("organizations whose branches do not match their desired status"))
	if err != nil {
		return fmt.Errorf("create unsynced organizations gauge: %w", err)
	}

	oldestAge, err := meter.Float64ObservableGauge("xata.projects.orgstatus.oldest_unsynced_age_seconds",
		metric.WithUnit("s"),
		metric.WithDescription("age of the oldest organization status awaiting a sync"))
	if err != nil {
		return fmt.Errorf("create oldest unsynced age gauge: %w", err)
	}

	_, err = meter.RegisterCallback(func(cbCtx context.Context, o metric.Observer) error {
		count, oldest, err := w.store.CountUnsyncedOrganizationStatuses(cbCtx)
		if err != nil {
			return fmt.Errorf("count unsynced organization statuses: %w", err)
		}
		o.ObserveInt64(unsynced, int64(count))
		if count > 0 {
			o.ObserveFloat64(oldestAge, w.now().Sub(oldest).Seconds())
			return nil
		}
		o.ObserveFloat64(oldestAge, 0)
		return nil
	}, unsynced, oldestAge)
	if err != nil {
		return fmt.Errorf("register backlog callback: %w", err)
	}
	return nil
}
