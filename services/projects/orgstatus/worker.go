package orgstatus

import (
	"context"
	"fmt"
	"time"

	"xata/internal/o11y"
	"xata/services/projects/cells"
	"xata/services/projects/store"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// Defaults chosen so a status is applied within seconds of the webhook in the
// normal case (the nudge), while a worker that dies mid-sync releases its claim
// in minutes.
const (
	DefaultPollInterval = 30 * time.Second
	DefaultSyncTimeout  = 10 * time.Minute
	DefaultLease        = 15 * time.Minute
	DefaultMaxBackoff   = 10 * time.Minute
)

// escalateAfter is how long an organization may be owed a sync before a failure
// logs at Error rather than Warn. It is measured from the desired-state write.
const escalateAfter = time.Hour

// bookkeepingTimeout bounds the write that records a sync outcome. The write
// runs on a context detached from the caller's, so it needs a limit of its own.
const bookkeepingTimeout = 5 * time.Second

// Config tunes the worker. The zero value is filled in with the defaults above.
type Config struct {
	PollInterval time.Duration
	// SyncTimeout bounds one organization's fan-out.
	SyncTimeout time.Duration
	// Lease is how long a claim holds a row. It is raised to at least
	// SyncTimeout so that another worker cannot claim the row while this one
	// is still syncing it.
	Lease      time.Duration
	MaxBackoff time.Duration
}

func (c Config) withDefaults() Config {
	if c.PollInterval <= 0 {
		c.PollInterval = DefaultPollInterval
	}
	if c.SyncTimeout <= 0 {
		c.SyncTimeout = DefaultSyncTimeout
	}
	if c.Lease <= 0 {
		c.Lease = DefaultLease
	}
	if c.MaxBackoff <= 0 {
		c.MaxBackoff = DefaultMaxBackoff
	}
	if c.Lease < c.SyncTimeout {
		c.Lease = c.SyncTimeout
	}
	return c
}

// Worker applies pending organization statuses to the fleet.
type Worker struct {
	store store.ProjectsStore
	cells cells.Cells
	cfg   Config

	// now is injected so retry scheduling can be asserted against fixed times.
	now func() time.Time

	// nudge lets the gRPC handler wake the worker without waiting for the next
	// tick. It is buffered with capacity one: a pending wake-up already covers
	// any number of writes, so sends never block.
	nudge chan struct{}

	// tracer starts one span per organization sync. The worker's context has
	// no request span, so without this every outbound call to a cell would be
	// its own root trace with nothing to tie them together.
	tracer trace.Tracer
}

// NewWorker builds a worker. Pass the zero value Config for the defaults.
func NewWorker(projectsStore store.ProjectsStore, cellConns cells.Cells, cfg Config) *Worker {
	return &Worker{
		store:  projectsStore,
		cells:  cellConns,
		cfg:    cfg.withDefaults(),
		now:    func() time.Time { return time.Now().UTC() },
		nudge:  make(chan struct{}, 1),
		tracer: noop.NewTracerProvider().Tracer(instrumentationName),
	}
}

const instrumentationName = "xata/services/projects/orgstatus"

// setClock replaces the worker's clock. For tests.
func (w *Worker) setClock(now func() time.Time) { w.now = now }

// setTracer replaces the worker's tracer. For tests.
func (w *Worker) setTracer(tracer trace.Tracer) { w.tracer = tracer }

// Nudge asks the worker to run a pass now rather than at the next tick. It never
// blocks, so a caller on a request path is never held up by a busy worker.
func (w *Worker) Nudge() {
	select {
	case w.nudge <- struct{}{}:
	default:
	}
}

// ProcessOnce claims organizations due for a sync one at a time, applies each,
// and records the outcome. It returns the number of organizations it attempted.
//
// It keeps claiming until a claim comes back empty, so a caller that gets 0 back
// knows nothing is outstanding. Claiming one row at a time keeps the lease
// aligned with the sync it covers. The error is the first claim or bookkeeping
// failure; a single organization failing to sync is recorded on its row and
// does not stop the pass.
func (w *Worker) ProcessOnce(ctx context.Context) (int, error) {
	attempted := 0
	for {
		if err := ctx.Err(); err != nil {
			return attempted, err
		}

		pending, err := w.store.ClaimOrganizationStatusForSync(ctx, w.now(), w.cfg.Lease)
		if err != nil {
			return attempted, fmt.Errorf("claim organization status: %w", err)
		}
		if pending == nil {
			return attempted, nil
		}

		attempted++
		if err := w.syncOne(ctx, *pending); err != nil {
			return attempted, err
		}
	}
}

// syncOne applies one organization and records the outcome. It returns an error
// only when the bookkeeping write fails; a failed fan-out is recorded on the row
// so the next pass retries it.
func (w *Worker) syncOne(ctx context.Context, pending store.OrganizationStatus) (err error) {
	ctx, span := w.tracer.Start(ctx, "orgstatus.sync", trace.WithAttributes(
		attribute.String("xata.organization.id", pending.OrganizationID),
		attribute.Bool("xata.organization.disabled", pending.Disabled),
		attribute.Int64("xata.orgstatus.version", pending.Version),
		attribute.Int("xata.orgstatus.attempts", pending.Attempts),
	))
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}()

	syncCtx, cancel := context.WithTimeout(ctx, w.cfg.SyncTimeout)
	defer cancel()

	syncErr := Sync(syncCtx, w.store, w.cells, pending.OrganizationID, pending.Disabled)

	// Detached from ctx so a shutdown still releases the lease it claimed.
	bookkeepingCtx, cancelBookkeeping := context.WithTimeout(context.WithoutCancel(ctx), bookkeepingTimeout)
	defer cancelBookkeeping()

	if syncErr != nil {
		retryAt := w.now().Add(w.backoff(pending.Attempts))
		if ctx.Err() != nil {
			// The service is stopping so the next replica takes the row immediately.
			retryAt = w.now()
			span.AddEvent("sync interrupted by shutdown")
			log.Ctx(ctx).Info().
				Str("organization", pending.OrganizationID).
				Msg("Organization status sync interrupted, releasing the lease")
		} else {
			// A failed fan-out is the outcome of this span even though it is not
			// returned: the row records it and the next pass retries.
			span.RecordError(syncErr)
			span.SetStatus(codes.Error, syncErr.Error())

			unsyncedFor := w.now().Sub(pending.UpdatedAt)
			log.Ctx(ctx).WithLevel(failureLevel(unsyncedFor)).Err(syncErr).
				Str("organization", pending.OrganizationID).
				Int("attempts", pending.Attempts).
				Dur("unsynced_for", unsyncedFor).
				Time("retry_at", retryAt).
				Msg("Organization status sync failed, will retry")
		}
		if err := w.store.MarkOrganizationStatusFailed(bookkeepingCtx, pending.OrganizationID, pending.Version, syncErr.Error(), retryAt); err != nil {
			return fmt.Errorf("record sync failure for %s: %w", pending.OrganizationID, err)
		}
		return nil
	}

	marked, err := w.store.MarkOrganizationStatusSynced(bookkeepingCtx, pending.OrganizationID, pending.Version, w.now())
	if err != nil {
		return fmt.Errorf("record sync success for %s: %w", pending.OrganizationID, err)
	}
	span.SetAttributes(attribute.Bool("xata.orgstatus.version_moved", !marked))
	if !marked {
		// The desired state changed while this sync ran, so the row is still
		// owed a pass. Leaving synced_at NULL is what makes the next claim pick
		// it up; there is nothing else to do here.
		log.Ctx(ctx).Info().
			Str("organization", pending.OrganizationID).
			Int64("synced_version", pending.Version).
			Msg("Organization status changed during sync, another pass is owed")
	}
	return nil
}

// failureLevel raises an organization the fleet has not matched for a long time
// above the noise of one that is merely between retries.
func failureLevel(unsyncedFor time.Duration) zerolog.Level {
	if unsyncedFor >= escalateAfter {
		return zerolog.ErrorLevel
	}
	return zerolog.WarnLevel
}

// backoff grows with the attempt count and is capped, so a cell that is down for
// an hour is retried steadily rather than hammered.
func (w *Worker) backoff(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	backoff := time.Duration(1<<min(attempts-1, 20)) * time.Second
	return min(backoff, w.cfg.MaxBackoff)
}

// Run implements service.RunnerService.
func (w *Worker) Run(ctx context.Context, o *o11y.O) error {
	w.tracer = o.Tracer(instrumentationName)
	if err := w.registerBacklogMetrics(o.Meter(instrumentationName)); err != nil {
		return fmt.Errorf("register organization status metrics: %w", err)
	}

	ticker := time.NewTicker(w.cfg.PollInterval)
	defer ticker.Stop()

	log.Ctx(ctx).Info().
		Dur("poll_interval", w.cfg.PollInterval).
		Dur("sync_timeout", w.cfg.SyncTimeout).
		Msg("Organization status worker started")

	for {
		if _, err := w.ProcessOnce(ctx); err != nil && ctx.Err() == nil {
			log.Ctx(ctx).Err(err).Msg("Organization status pass failed")
		}
		w.reportBacklog(ctx)

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		case <-w.nudge:
		}
	}
}

// reportBacklog logs how many organizations are waiting and how stale the oldest
// is.
func (w *Worker) reportBacklog(ctx context.Context) {
	count, oldest, err := w.store.CountUnsyncedOrganizationStatuses(ctx)
	if err != nil {
		if ctx.Err() == nil {
			log.Ctx(ctx).Warn().Err(err).Msg("Count unsynced organization statuses")
		}
		return
	}
	if count == 0 {
		return
	}
	// A row younger than one lease is still explainable as work in flight, and
	// a nudge makes every status change produce one such pass.
	oldestAge := w.now().Sub(oldest)
	level := zerolog.InfoLevel
	if oldestAge >= w.cfg.Lease {
		level = zerolog.WarnLevel
	}
	log.Ctx(ctx).WithLevel(level).
		Int("unsynced_organizations", count).
		Dur("oldest_age", oldestAge).
		Msg("Organizations awaiting status sync")
}
