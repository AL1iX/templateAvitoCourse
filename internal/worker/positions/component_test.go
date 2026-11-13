package positions

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/AL1iX/templateAvitoCourse/internal/model"
)

const (
	tripA = "trip-a"
	tripB = "trip-b"
	tripC = "trip-c"
	tripD = "trip-d"
	tripE = "trip-e"

	eventTimeout = 10 * time.Second

	checkQueued    = "check:" + resultQueued
	checkDuplicate = "check:" + resultDuplicate
	checkDropped   = "check:" + resultDropped
	requestSuccess = "request:" + resultSuccess
	requestError   = "request:" + resultError
	requestDropped = "request:" + resultDropped
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func driverOf(tripID string) string {
	return "driver-" + tripID
}

func staleTrips(ids ...string) []model.StaleTrip {
	trips := make([]model.StaleTrip, 0, len(ids))
	for _, id := range ids {
		trips = append(trips, model.StaleTrip{TripID: id, DriverID: driverOf(id)})
	}
	return trips
}

type fakeTicker struct {
	c        chan time.Time
	stopCh   chan struct{}
	stopOnce sync.Once
}

func newFakeTicker() *fakeTicker {
	return &fakeTicker{c: make(chan time.Time), stopCh: make(chan struct{})}
}

func (f *fakeTicker) C() <-chan time.Time { return f.c }

func (f *fakeTicker) Stop() { f.stopOnce.Do(func() { close(f.stopCh) }) }

func (f *fakeTicker) isStopped() bool {
	select {
	case <-f.stopCh:
		return true
	default:
		return false
	}
}

func (f *fakeTicker) tick(t *testing.T) {
	t.Helper()

	select {
	case f.c <- time.Time{}:
	case <-time.After(eventTimeout):
		t.Fatal("the scanner did not take the tick")
	}
}

type fixture struct {
	c       *Component
	ticker  *fakeTicker
	finder  *MockstaleTripFinder
	gateway *MockpushGateway
	events  chan string
}

func newFixture(t *testing.T, workers, queueSize int) *fixture {
	t.Helper()

	ctrl := gomock.NewController(t)
	f := &fixture{
		ticker:  newFakeTicker(),
		finder:  NewMockstaleTripFinder(ctrl),
		gateway: NewMockpushGateway(ctrl),
		events:  make(chan string, 1000),
	}
	metrics := NewMockpositionMetrics(ctrl)
	metrics.EXPECT().StaleCheck(gomock.Any(), gomock.Any()).
		Do(func(_ context.Context, result string) { f.events <- "check:" + result }).AnyTimes()
	metrics.EXPECT().PositionRequest(gomock.Any(), gomock.Any()).
		Do(func(_ context.Context, result string) { f.events <- "request:" + result }).AnyTimes()
	metrics.EXPECT().PositionRequestDuration(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	f.c = New(
		Config{
			CheckInterval:  time.Second,
			StaleAfter:     time.Minute,
			Workers:        workers,
			QueueSize:      queueSize,
			RequestTimeout: time.Minute,
		},
		f.finder, f.gateway, metrics,
		noop.NewTracerProvider().Tracer("test"),
		slog.New(slog.NewJSONHandler(io.Discard, nil)),
	)
	f.c.newTicker = func(time.Duration) ticker { return f.ticker }
	return f
}

func (f *fixture) startWith(ctx context.Context, t *testing.T) {
	t.Helper()

	f.c.Start(ctx)
	t.Cleanup(func() {
		require.NoError(t, f.c.Stop(context.Background()))
	})
}

func (f *fixture) start(t *testing.T) {
	t.Helper()

	f.startWith(t.Context(), t)
}

func (f *fixture) stop(t *testing.T) {
	t.Helper()

	require.NoError(t, f.c.Stop(t.Context()))
}

func waitEvents(t *testing.T, events <-chan string, n int) []string {
	t.Helper()

	got := make([]string, 0, n)
	for range n {
		select {
		case e := <-events:
			got = append(got, e)
		case <-time.After(eventTimeout):
			t.Fatalf("got %d of %d events: %v", len(got), n, got)
		}
	}
	return got
}

func drain(events <-chan string) []string {
	var got []string
	for {
		select {
		case e := <-events:
			got = append(got, e)
		default:
			return got
		}
	}
}

func receive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()

	select {
	case v := <-ch:
		return v
	case <-time.After(eventTimeout):
		t.Fatal("timed out waiting for the event")
		panic("unreachable")
	}
}

func TestTickRequestsPositionOfStaleTrip(t *testing.T) {
	f := newFixture(t, 1, 10)
	f.finder.EXPECT().FindStale(gomock.Any(), time.Minute).Return(staleTrips(tripA), nil)
	f.gateway.EXPECT().RequestPosition(gomock.Any(), tripA, driverOf(tripA)).Return(nil)
	f.start(t)

	f.ticker.tick(t)

	require.ElementsMatch(t, []string{checkQueued, requestSuccess}, waitEvents(t, f.events, 2))
}

func TestTickDoesNotQueueTripThatIsAlreadyInTheQueue(t *testing.T) {
	f := newFixture(t, 1, 10)
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	f.finder.EXPECT().FindStale(gomock.Any(), time.Minute).Return(staleTrips(tripA), nil).Times(2)
	f.gateway.EXPECT().RequestPosition(gomock.Any(), tripA, driverOf(tripA)).
		DoAndReturn(func(context.Context, string, string) error {
			started <- struct{}{}
			<-release
			return nil
		}).Times(1)
	f.start(t)

	f.ticker.tick(t)
	require.Equal(t, []string{checkQueued}, waitEvents(t, f.events, 1))
	receive(t, started)
	f.ticker.tick(t)

	require.Equal(t, []string{checkDuplicate}, waitEvents(t, f.events, 1),
		"the trip is still held while its request is in flight")
	close(release)
	f.stop(t)
}

func TestTripIsQueuedAgainOnceItsRequestIsFinished(t *testing.T) {
	tests := []struct {
		name       string
		pushErr    error
		wantResult string
	}{
		{name: "after a successful request", wantResult: requestSuccess},
		{name: "after a failed request", pushErr: errors.New("push is down"), wantResult: requestError},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, 1, 10)
			f.finder.EXPECT().FindStale(gomock.Any(), time.Minute).Return(staleTrips(tripA), nil).Times(2)
			f.gateway.EXPECT().RequestPosition(gomock.Any(), tripA, driverOf(tripA)).Return(tc.pushErr).Times(2)
			f.start(t)

			f.ticker.tick(t)
			require.ElementsMatch(t, []string{checkQueued, tc.wantResult}, waitEvents(t, f.events, 2))
			f.ticker.tick(t)

			require.ElementsMatch(t, []string{checkQueued, tc.wantResult}, waitEvents(t, f.events, 2),
				"the next tick queues the trip again, the period of the ticker is the pause between requests")
		})
	}
}

func TestPositionCancelsRequestBetweenQueueingAndProcessing(t *testing.T) {
	f := newFixture(t, 1, 10)
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	f.finder.EXPECT().FindStale(gomock.Any(), time.Minute).Return(staleTrips(tripA, tripB), nil)
	f.gateway.EXPECT().RequestPosition(gomock.Any(), tripA, driverOf(tripA)).
		DoAndReturn(func(context.Context, string, string) error {
			started <- struct{}{}
			<-release
			return nil
		})
	f.start(t)
	f.ticker.tick(t)
	require.Equal(t, []string{checkQueued, checkQueued}, waitEvents(t, f.events, 2))
	receive(t, started)

	f.c.Cancel(tripB)
	close(release)
	f.stop(t)

	require.Equal(t, []string{requestSuccess}, drain(f.events), "only the trip that was not cancelled is requested")
}

func TestStaleCopyAfterCancelAndRequeueIsDiscarded(t *testing.T) {
	f := newFixture(t, 1, 10)
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	gomock.InOrder(
		f.finder.EXPECT().FindStale(gomock.Any(), time.Minute).Return(staleTrips(tripA, tripB), nil),
		f.finder.EXPECT().FindStale(gomock.Any(), time.Minute).Return(staleTrips(tripB), nil),
	)
	f.gateway.EXPECT().RequestPosition(gomock.Any(), tripA, driverOf(tripA)).
		DoAndReturn(func(context.Context, string, string) error {
			started <- struct{}{}
			<-release
			return nil
		})
	f.gateway.EXPECT().RequestPosition(gomock.Any(), tripB, driverOf(tripB)).Return(nil).Times(1)
	f.start(t)
	f.ticker.tick(t)
	require.Equal(t, []string{checkQueued, checkQueued}, waitEvents(t, f.events, 2))
	receive(t, started)

	f.c.Cancel(tripB)
	f.ticker.tick(t)
	require.Equal(t, []string{checkQueued}, waitEvents(t, f.events, 1), "the cancelled trip is queued again")
	close(release)
	f.stop(t)

	require.Equal(t, []string{requestSuccess, requestSuccess}, drain(f.events),
		"two copies of the second trip are in the channel, only the current one is requested")
}

func TestFullQueueDropsTripsWithoutBlockingTheScanner(t *testing.T) {
	f := newFixture(t, 1, 2)
	var logs bytes.Buffer
	f.c.logger = slog.New(slog.NewJSONHandler(&logs, nil))
	f.finder.EXPECT().FindStale(gomock.Any(), time.Minute).
		Return(staleTrips(tripA, tripB, tripC, tripD, tripE), nil).Times(2)

	f.c.tick(t.Context())

	require.ElementsMatch(t, []string{
		checkQueued, checkQueued,
		checkDropped, checkDropped, checkDropped,
		requestDropped, requestDropped, requestDropped,
	}, drain(f.events))
	require.Equal(t, 2, f.c.QueueSize())
	require.Equal(t, 3, strings.Count(logs.String(), `"level":"WARN"`))

	<-f.c.queue
	<-f.c.queue
	f.c.tick(t.Context())

	require.ElementsMatch(t, []string{
		checkDuplicate, checkDuplicate,
		checkQueued, checkQueued,
		checkDropped, requestDropped,
	}, drain(f.events), "the dropped trips left no trace in the queue and are retried")
}

func TestPoolRunsConfiguredNumberOfWorkers(t *testing.T) {
	const (
		workers = 3
		tasks   = 10
	)
	f := newFixture(t, workers, tasks)
	ids := make([]string, 0, tasks)
	for i := range tasks {
		ids = append(ids, "trip-"+string(rune('a'+i)))
	}
	var inFlight, maxInFlight, calls atomic.Int32
	started := make(chan struct{}, tasks)
	release := make(chan struct{})
	f.finder.EXPECT().FindStale(gomock.Any(), time.Minute).Return(staleTrips(ids...), nil)
	f.gateway.EXPECT().RequestPosition(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(context.Context, string, string) error {
			current := inFlight.Add(1)
			for {
				seen := maxInFlight.Load()
				if current <= seen || maxInFlight.CompareAndSwap(seen, current) {
					break
				}
			}
			calls.Add(1)
			started <- struct{}{}
			<-release
			inFlight.Add(-1)
			return nil
		}).Times(tasks)
	f.start(t)

	f.ticker.tick(t)
	require.Len(t, waitEvents(t, f.events, tasks), tasks)
	for range workers {
		receive(t, started)
	}

	require.Equal(t, workers, f.c.ActiveWorkers())
	require.Equal(t, tasks-workers, f.c.QueueSize())
	close(release)
	f.stop(t)

	require.EqualValues(t, tasks, calls.Load())
	require.EqualValues(t, workers, maxInFlight.Load(), "never more requests in flight than workers")
	require.Zero(t, f.c.ActiveWorkers())
}

func TestStopDrainsTheQueueAndStopsTheTicker(t *testing.T) {
	f := newFixture(t, 1, 10)
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	f.finder.EXPECT().FindStale(gomock.Any(), time.Minute).Return(staleTrips(tripA, tripB, tripC), nil)
	f.gateway.EXPECT().RequestPosition(gomock.Any(), tripA, driverOf(tripA)).
		DoAndReturn(func(context.Context, string, string) error {
			started <- struct{}{}
			<-release
			return nil
		})
	f.gateway.EXPECT().RequestPosition(gomock.Any(), tripB, driverOf(tripB)).Return(nil)
	f.gateway.EXPECT().RequestPosition(gomock.Any(), tripC, driverOf(tripC)).Return(nil)
	f.start(t)
	f.ticker.tick(t)
	waitEvents(t, f.events, 3)
	receive(t, started)

	stopped := make(chan error, 1)
	go func() { stopped <- f.c.Stop(t.Context()) }()
	select {
	case <-f.ticker.stopCh:
	case <-time.After(eventTimeout):
		t.Fatal("the scanner did not stop")
	}
	close(release)

	require.NoError(t, receive(t, stopped))
	_, open := <-f.c.queue
	require.False(t, open, "the scanner closes the queue, so the workers finish after draining it")
	require.True(t, f.ticker.isStopped())
	require.Equal(t, []string{requestSuccess, requestSuccess, requestSuccess}, drain(f.events),
		"the trips that were queued before the stop are still requested")
}

func TestStartIsNotStoppedByCancellationOfItsContext(t *testing.T) {
	f := newFixture(t, 1, 10)
	f.finder.EXPECT().FindStale(gomock.Any(), time.Minute).Return(staleTrips(tripA), nil)
	f.gateway.EXPECT().RequestPosition(gomock.Any(), tripA, driverOf(tripA)).Return(nil)
	ctx, cancel := context.WithCancel(t.Context())
	f.startWith(ctx, t)

	cancel()
	f.ticker.tick(t)

	require.ElementsMatch(t, []string{checkQueued, requestSuccess}, waitEvents(t, f.events, 2),
		"only Stop ends the scanner, so the signal context cannot stop it before the HTTP server")
}

func TestStopAbortsRequestsWhenTheBudgetIsSpent(t *testing.T) {
	f := newFixture(t, 1, 10)
	started := make(chan struct{}, 1)
	f.finder.EXPECT().FindStale(gomock.Any(), time.Minute).Return(staleTrips(tripA, tripB), nil)
	f.gateway.EXPECT().RequestPosition(gomock.Any(), tripA, driverOf(tripA)).
		DoAndReturn(func(ctx context.Context, _, _ string) error {
			started <- struct{}{}
			<-ctx.Done()
			return ctx.Err()
		})
	f.start(t)
	f.ticker.tick(t)
	waitEvents(t, f.events, 2)
	receive(t, started)
	budget, spend := context.WithCancel(t.Context())
	spend()

	err := f.c.Stop(budget)

	require.ErrorIs(t, err, context.Canceled)
	require.EqualValues(t, 1, f.c.abandoned.Load(), "the second trip was left in the queue")
	require.Zero(t, f.c.ActiveWorkers())
	require.Equal(t, []string{requestError}, drain(f.events))
}

func TestPushErrorDoesNotStopTheWorker(t *testing.T) {
	f := newFixture(t, 1, 10)
	f.finder.EXPECT().FindStale(gomock.Any(), time.Minute).Return(staleTrips(tripA, tripB), nil)
	gomock.InOrder(
		f.gateway.EXPECT().RequestPosition(gomock.Any(), tripA, driverOf(tripA)).Return(errors.New("push is down")),
		f.gateway.EXPECT().RequestPosition(gomock.Any(), tripB, driverOf(tripB)).Return(nil),
	)
	f.start(t)

	f.ticker.tick(t)

	events := waitEvents(t, f.events, 4)
	require.ElementsMatch(t, []string{checkQueued, checkQueued, requestError, requestSuccess}, events)
}

func TestTicksDoNotOverlap(t *testing.T) {
	f := newFixture(t, 1, 10)
	var inFlight, maxInFlight atomic.Int32
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	f.finder.EXPECT().FindStale(gomock.Any(), time.Minute).
		DoAndReturn(func(context.Context, time.Duration) ([]model.StaleTrip, error) {
			current := inFlight.Add(1)
			for {
				seen := maxInFlight.Load()
				if current <= seen || maxInFlight.CompareAndSwap(seen, current) {
					break
				}
			}
			started <- struct{}{}
			<-release
			inFlight.Add(-1)
			return nil, nil
		}).Times(2)
	f.start(t)

	f.ticker.tick(t)
	receive(t, started)
	secondTickTaken := make(chan struct{})
	go func() {
		f.ticker.c <- time.Time{}
		close(secondTickTaken)
	}()
	close(release)
	receive(t, started)
	receive(t, secondTickTaken)

	require.EqualValues(t, 1, maxInFlight.Load(), "the second pass starts only after the first one has finished")
}

func TestFailedScanDoesNotStopTheScanner(t *testing.T) {
	f := newFixture(t, 1, 10)
	gomock.InOrder(
		f.finder.EXPECT().FindStale(gomock.Any(), time.Minute).Return(nil, errors.New("database is down")),
		f.finder.EXPECT().FindStale(gomock.Any(), time.Minute).Return(staleTrips(tripA), nil),
	)
	f.gateway.EXPECT().RequestPosition(gomock.Any(), tripA, driverOf(tripA)).Return(nil)
	f.start(t)

	f.ticker.tick(t)
	f.ticker.tick(t)

	require.ElementsMatch(t, []string{checkQueued, requestSuccess}, waitEvents(t, f.events, 2))
}

func TestScanFailureIsLoggedUnlessTheServiceIsStopping(t *testing.T) {
	tests := []struct {
		name       string
		cancelCtx  bool
		wantErrors int
	}{
		{name: "failure of a live scan is logged once", wantErrors: 1},
		{name: "failure caused by the stop is not logged", cancelCtx: true, wantErrors: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, 1, 10)
			var logs bytes.Buffer
			f.c.logger = slog.New(slog.NewJSONHandler(&logs, nil))
			f.finder.EXPECT().FindStale(gomock.Any(), time.Minute).Return(nil, errors.New("database is down"))
			ctx, cancel := context.WithCancel(t.Context())
			if tc.cancelCtx {
				cancel()
			}
			defer cancel()

			f.c.tick(ctx)

			require.Equal(t, tc.wantErrors, strings.Count(logs.String(), `"level":"ERROR"`))
		})
	}
}

func TestProcessSpanIsLinkedToScanSpanAndNotItsChild(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	f := newFixture(t, 1, 10)
	f.c.tracer = provider.Tracer("test")
	f.finder.EXPECT().FindStale(gomock.Any(), time.Minute).Return(staleTrips(tripA), nil)
	f.gateway.EXPECT().RequestPosition(gomock.Any(), tripA, driverOf(tripA)).Return(nil)
	f.start(t)
	f.ticker.tick(t)
	waitEvents(t, f.events, 2)
	f.stop(t)

	spans := make(map[string]sdktrace.ReadOnlySpan)
	for _, s := range recorder.Ended() {
		spans[s.Name()] = s
	}
	scan, process, push := spans["positions.scan"], spans["positions.process"], spans["push.request_position"]
	require.NotNil(t, scan)
	require.NotNil(t, process)
	require.NotNil(t, push)
	require.Len(t, process.Links(), 1)
	require.Equal(t, scan.SpanContext().SpanID(), process.Links()[0].SpanContext.SpanID())
	require.Equal(t, scan.SpanContext().TraceID(), process.Links()[0].SpanContext.TraceID())
	require.False(t, process.Parent().IsValid(), "the worker span is linked to the scan, not parented by it")
	require.Equal(t, process.SpanContext().SpanID(), push.Parent().SpanID(), "the push call is a child of the worker span")
}
