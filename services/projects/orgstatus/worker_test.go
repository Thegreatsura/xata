package orgstatus

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"xata/internal/apitest"
	"xata/services/projects/cells/cellsmock"
	"xata/services/projects/store"
	"xata/services/projects/store/mocks"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// fixedNow keeps retry scheduling assertable. Run is never started in these
// tests: ProcessOnce blocks until the pass is done and returns what it did, so
// nothing here waits on a goroutine or sleeps.
var fixedNow = time.Date(2026, 8, 26, 13, 50, 0, 0, time.UTC)

func newTestWorker(t *testing.T, cfg Config) (*Worker, *mocks.ProjectsStore, *cellsmock.Cells) {
	t.Helper()
	mockStore := mocks.NewProjectsStore(t)
	mockCells := cellsmock.NewCells(t)
	w := NewWorker(mockStore, mockCells, cfg)
	w.setClock(func() time.Time { return fixedNow })
	return w, mockStore, mockCells
}

func TestWorkerProcessOnce(t *testing.T) {
	pending := store.OrganizationStatus{
		OrganizationID: apitest.TestOrganization,
		Disabled:       true,
		Version:        7,
		Attempts:       1,
	}

	tests := map[string]struct {
		setupMock     func(*mocks.ProjectsStore)
		wantAttempted int
		wantErr       bool
	}{
		"nothing due": {
			setupMock: func(s *mocks.ProjectsStore) {
				s.EXPECT().ClaimOrganizationStatusForSync(mock.Anything, fixedNow, mock.Anything).
					Return(nil, nil).Once()
			},
			wantAttempted: 0,
		},
		"syncs and marks synced at the claimed version": {
			setupMock: func(s *mocks.ProjectsStore) {
				s.EXPECT().ClaimOrganizationStatusForSync(mock.Anything, fixedNow, mock.Anything).
					Return(&pending, nil).Once()
				// An organization with no projects still exercises the full
				// claim, sync and bookkeeping path.
				s.EXPECT().ListProjects(mock.Anything, apitest.TestOrganization).Return(nil, nil).Once()
				s.EXPECT().MarkOrganizationStatusSynced(mock.Anything, apitest.TestOrganization, int64(7), fixedNow).
					Return(true, nil).Once()
				// Second claim drains the queue and ends the pass.
				s.EXPECT().ClaimOrganizationStatusForSync(mock.Anything, fixedNow, mock.Anything).
					Return(nil, nil).Once()
			},
			wantAttempted: 1,
		},
		"failed sync is recorded with a backoff, not returned": {
			setupMock: func(s *mocks.ProjectsStore) {
				s.EXPECT().ClaimOrganizationStatusForSync(mock.Anything, fixedNow, mock.Anything).
					Return(&pending, nil).Once()
				s.EXPECT().ListProjects(mock.Anything, apitest.TestOrganization).
					Return(nil, errors.New("boom")).Once()
				// Attempts was 1 at claim time, so the next try is 1<<0 seconds out.
				s.EXPECT().MarkOrganizationStatusFailed(mock.Anything, apitest.TestOrganization, int64(7), mock.Anything, fixedNow.Add(time.Second)).
					Return(nil).Once()
				s.EXPECT().ClaimOrganizationStatusForSync(mock.Anything, fixedNow, mock.Anything).
					Return(nil, nil).Once()
			},
			wantAttempted: 1,
		},
		"status changed during sync leaves the row owed": {
			setupMock: func(s *mocks.ProjectsStore) {
				s.EXPECT().ClaimOrganizationStatusForSync(mock.Anything, fixedNow, mock.Anything).
					Return(&pending, nil).Once()
				s.EXPECT().ListProjects(mock.Anything, apitest.TestOrganization).Return(nil, nil).Once()
				// The version moved, so the row is not marked synced and the
				// next pass picks it up again. That is not an error.
				s.EXPECT().MarkOrganizationStatusSynced(mock.Anything, apitest.TestOrganization, int64(7), fixedNow).
					Return(false, nil).Once()
				s.EXPECT().ClaimOrganizationStatusForSync(mock.Anything, fixedNow, mock.Anything).
					Return(nil, nil).Once()
			},
			wantAttempted: 1,
		},
		"claim failure ends the pass": {
			setupMock: func(s *mocks.ProjectsStore) {
				s.EXPECT().ClaimOrganizationStatusForSync(mock.Anything, fixedNow, mock.Anything).
					Return(nil, errors.New("db down")).Once()
			},
			wantAttempted: 0,
			wantErr:       true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			w, mockStore, _ := newTestWorker(t, Config{})
			tt.setupMock(mockStore)

			got, err := w.ProcessOnce(context.Background())

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tt.wantAttempted, got)
		})
	}
}

func TestWorkerProcessOnceShutdown(t *testing.T) {
	pending := store.OrganizationStatus{
		OrganizationID: apitest.TestOrganization,
		Disabled:       true,
		Version:        7,
		Attempts:       3,
	}

	w, mockStore, _ := newTestWorker(t, Config{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockStore.EXPECT().ClaimOrganizationStatusForSync(mock.Anything, fixedNow, mock.Anything).
		Return(&pending, nil).Once()
	mockStore.EXPECT().ListProjects(mock.Anything, apitest.TestOrganization).
		Run(func(context.Context, string) { cancel() }).
		Return(nil, context.Canceled).Once()
	// retry_at is fixedNow rather than fixedNow plus the backoff for attempt 3:
	// the shutdown did not earn one. The write itself must still run, which it
	// only can on a context that outlives the cancelled one.
	mockStore.EXPECT().MarkOrganizationStatusFailed(mock.Anything, apitest.TestOrganization, int64(7), mock.Anything, fixedNow).
		Run(func(bookkeepingCtx context.Context, _ string, _ int64, _ string, _ time.Time) {
			require.NoError(t, bookkeepingCtx.Err())
		}).
		Return(nil).Once()

	attempted, err := w.ProcessOnce(ctx)

	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, attempted)
}

func TestFailureLevel(t *testing.T) {
	tests := map[string]struct {
		unsyncedFor time.Duration
		want        zerolog.Level
	}{
		"just written":           {unsyncedFor: 0, want: zerolog.WarnLevel},
		"below the threshold":    {unsyncedFor: escalateAfter - time.Minute, want: zerolog.WarnLevel},
		"at the threshold":       {unsyncedFor: escalateAfter, want: zerolog.ErrorLevel},
		"far past the threshold": {unsyncedFor: 24 * time.Hour, want: zerolog.ErrorLevel},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tt.want, failureLevel(tt.unsyncedFor))
		})
	}
}

func TestWorkerBackoff(t *testing.T) {
	w, _, _ := newTestWorker(t, Config{MaxBackoff: 30 * time.Second})

	tests := map[string]struct {
		attempts int
		want     time.Duration
	}{
		"first attempt":   {attempts: 1, want: time.Second},
		"third attempt":   {attempts: 3, want: 4 * time.Second},
		"capped":          {attempts: 20, want: 30 * time.Second},
		"never below one": {attempts: 0, want: time.Second},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tt.want, w.backoff(tt.attempts))
		})
	}
}

func TestConfigWithDefaults(t *testing.T) {
	tests := map[string]struct {
		cfg       Config
		wantLease time.Duration
	}{
		"zero value takes the defaults": {
			cfg:       Config{},
			wantLease: DefaultLease,
		},
		"lease is raised to cover the sync timeout": {
			// A sync may run for the whole timeout. A shorter lease would let
			// another worker claim the row while this one is still syncing it.
			cfg:       Config{SyncTimeout: 20 * time.Minute, Lease: 15 * time.Minute},
			wantLease: 20 * time.Minute,
		},
		"a lease that already covers the sync timeout is kept": {
			cfg:       Config{SyncTimeout: time.Minute, Lease: time.Hour},
			wantLease: time.Hour,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := tt.cfg.withDefaults()
			require.Equal(t, tt.wantLease, got.Lease)
		})
	}
}

func TestWorkerNudgeDoesNotBlock(t *testing.T) {
	w, _, _ := newTestWorker(t, Config{})

	// The channel holds one wake-up. Further nudges are dropped rather than
	// blocking the gRPC handler that sends them.
	for range 5 {
		w.Nudge()
	}
	require.Len(t, w.nudge, 1)
}

// TestWorkerRun covers the loop around ProcessOnce: the poll ticker, the nudge
// channel, and shutdown. These are the parts that are timing dependent, so the
// test runs inside a synctest bubble. Time there is fake and advances only when
// every goroutine is blocked.
func TestWorkerRun(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var passes atomic.Int64

		mockStore := mocks.NewProjectsStore(t)
		mockCells := cellsmock.NewCells(t)
		mockStore.EXPECT().
			ClaimOrganizationStatusForSync(mock.Anything, mock.Anything, mock.Anything).
			Run(func(context.Context, time.Time, time.Duration) { passes.Add(1) }).
			Return(nil, nil)
		mockStore.EXPECT().CountUnsyncedOrganizationStatuses(mock.Anything).Return(0, time.Time{}, nil)

		w := NewWorker(mockStore, mockCells, Config{PollInterval: 30 * time.Second})

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- w.Run(ctx, nil) }()

		// Wait returns once every goroutine in the bubble is blocked, which
		// here means the first pass finished and Run is parked on its select.
		synctest.Wait()
		require.Equal(t, int64(1), passes.Load(), "the worker runs a pass on start")

		// Advancing the fake clock past the interval fires the ticker. No real
		// 30 seconds elapse.
		time.Sleep(30 * time.Second)
		synctest.Wait()
		require.Equal(t, int64(2), passes.Load(), "the poll ticker drives a pass")

		// A nudge runs a pass without any time passing, which is what makes a
		// status change visible in seconds rather than at the next tick.
		w.Nudge()
		synctest.Wait()
		require.Equal(t, int64(3), passes.Load(), "a nudge drives a pass immediately")

		cancel()
		synctest.Wait()
		select {
		case err := <-done:
			require.NoError(t, err, "cancellation is a clean shutdown, not an error")
		default:
			t.Fatal("Run did not return after its context was cancelled")
		}
	})
}

// TestWorkerSyncSpan checks that each organization sync is one span, that a
// failed fan-out is recorded as an error on it even though syncOne returns
// nil, and that a clean sync is not. The incident this package answers was
// invisible in traces because nothing set a status on the failing span.
func TestReportBacklogLevel(t *testing.T) {
	cfg := Config{}.withDefaults()

	tests := map[string]struct {
		oldest time.Time
		want   zerolog.Level
	}{
		"a row younger than one lease is work in flight": {
			oldest: fixedNow.Add(-time.Second),
			want:   zerolog.InfoLevel,
		},
		"a row older than one lease is a backlog": {
			oldest: fixedNow.Add(-cfg.Lease),
			want:   zerolog.WarnLevel,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var got zerolog.Level
			logger := zerolog.New(zerolog.Nop()).Hook(zerolog.HookFunc(
				func(_ *zerolog.Event, level zerolog.Level, _ string) { got = level },
			))

			w, mockStore, _ := newTestWorker(t, Config{})
			mockStore.EXPECT().CountUnsyncedOrganizationStatuses(mock.Anything).
				Return(1, tt.oldest, nil).Once()

			w.reportBacklog(logger.WithContext(context.Background()))

			require.Equal(t, tt.want, got)
		})
	}
}

func TestBacklogMetrics(t *testing.T) {
	tests := map[string]struct {
		count       int
		oldest      time.Time
		wantCount   int64
		wantAgeSecs float64
	}{
		"empty backlog reports zero for both": {
			count: 0, oldest: time.Time{},
			wantCount: 0, wantAgeSecs: 0,
		},
		"a waiting organization reports its age": {
			count: 3, oldest: fixedNow.Add(-90 * time.Second),
			wantCount: 3, wantAgeSecs: 90,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			reader := sdkmetric.NewManualReader()
			provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
			t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

			w, mockStore, _ := newTestWorker(t, Config{})
			mockStore.EXPECT().CountUnsyncedOrganizationStatuses(mock.Anything).
				Return(tt.count, tt.oldest, nil).Once()
			require.NoError(t, w.registerBacklogMetrics(provider.Meter(instrumentationName)))

			var rm metricdata.ResourceMetrics
			require.NoError(t, reader.Collect(context.Background(), &rm))

			got := map[string]float64{}
			for _, sm := range rm.ScopeMetrics {
				for _, m := range sm.Metrics {
					switch d := m.Data.(type) {
					case metricdata.Gauge[int64]:
						got[m.Name] = float64(d.DataPoints[0].Value)
					case metricdata.Gauge[float64]:
						got[m.Name] = d.DataPoints[0].Value
					}
				}
			}

			require.Equal(t, float64(tt.wantCount), got["xata.projects.orgstatus.unsynced_organizations"])
			require.Equal(t, tt.wantAgeSecs, got["xata.projects.orgstatus.oldest_unsynced_age_seconds"])
		})
	}
}

func TestWorkerSyncSpan(t *testing.T) {
	pending := store.OrganizationStatus{
		OrganizationID: apitest.TestOrganization,
		Disabled:       true,
		Version:        7,
		Attempts:       1,
	}

	tests := map[string]struct {
		setupMock    func(*mocks.ProjectsStore)
		cancelParent bool
		wantStatus   codes.Code
	}{
		"clean sync leaves the span unset": {
			setupMock: func(s *mocks.ProjectsStore) {
				s.EXPECT().ListProjects(mock.Anything, apitest.TestOrganization).Return(nil, nil).Once()
				s.EXPECT().MarkOrganizationStatusSynced(mock.Anything, apitest.TestOrganization, int64(7), fixedNow).
					Return(true, nil).Once()
			},
			wantStatus: codes.Unset,
		},
		"a shutdown is not an error on the span": {
			setupMock: func(s *mocks.ProjectsStore) {
				s.EXPECT().ListProjects(mock.Anything, apitest.TestOrganization).
					Return(nil, context.Canceled).Once()
				s.EXPECT().MarkOrganizationStatusFailed(mock.Anything, apitest.TestOrganization, int64(7), mock.Anything, mock.Anything).
					Return(nil).Once()
			},
			cancelParent: true,
			wantStatus:   codes.Unset,
		},
		"failed fan-out marks the span as an error": {
			setupMock: func(s *mocks.ProjectsStore) {
				s.EXPECT().ListProjects(mock.Anything, apitest.TestOrganization).
					Return(nil, errors.New("cell unreachable")).Once()
				s.EXPECT().MarkOrganizationStatusFailed(mock.Anything, apitest.TestOrganization, int64(7), mock.Anything, mock.Anything).
					Return(nil).Once()
			},
			wantStatus: codes.Error,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			exporter := tracetest.NewInMemoryExporter()
			provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
			t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

			w, mockStore, _ := newTestWorker(t, Config{})
			w.setTracer(provider.Tracer(instrumentationName))
			tt.setupMock(mockStore)

			ctx := context.Background()
			if tt.cancelParent {
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = cancelled
			}

			require.NoError(t, w.syncOne(ctx, pending))

			spans := exporter.GetSpans()
			require.Len(t, spans, 1, "one span per organization sync")
			got := spans[0]
			require.Equal(t, "orgstatus.sync", got.Name)
			require.Equal(t, tt.wantStatus, got.Status.Code)
			require.Contains(t, got.Attributes, attribute.String("xata.organization.id", apitest.TestOrganization))
			if tt.wantStatus == codes.Error {
				require.NotEmpty(t, got.Events, "the error is recorded as a span event")
			}
		})
	}
}
