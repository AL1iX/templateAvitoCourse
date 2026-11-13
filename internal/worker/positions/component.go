package positions

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/AL1iX/templateAvitoCourse/internal/model"
)

const (
	resultQueued    = "queued"
	resultDuplicate = "duplicate"
	resultDropped   = "dropped"
	resultSuccess   = "success"
	resultError     = "error"
)

type Config struct {
	CheckInterval  time.Duration
	StaleAfter     time.Duration
	Workers        int
	QueueSize      int
	RequestTimeout time.Duration
}

type task struct {
	trip    model.StaleTrip
	version uint64
	scan    trace.SpanContext
}

type Component struct {
	cfg       Config
	finder    staleTripFinder
	gateway   pushGateway
	metrics   positionMetrics
	tracer    trace.Tracer
	logger    *slog.Logger
	newTicker func(time.Duration) ticker

	inQueue   *inQueue
	queue     chan task
	active    atomic.Int64
	abandoned atomic.Int64
	wg        sync.WaitGroup
	forceCtx  context.Context
	abort     context.CancelFunc
	stopScan  context.CancelFunc
}

func New(
	cfg Config,
	finder staleTripFinder,
	gateway pushGateway,
	metrics positionMetrics,
	tracer trace.Tracer,
	logger *slog.Logger,
) *Component {
	return &Component{
		cfg:       cfg,
		finder:    finder,
		gateway:   gateway,
		metrics:   metrics,
		tracer:    tracer,
		logger:    logger,
		newTicker: newTimeTicker,
		inQueue:   newInQueue(),
		queue:     make(chan task, cfg.QueueSize),
	}
}

func (c *Component) Start(ctx context.Context) {
	base := context.WithoutCancel(ctx)
	scanCtx, stopScan := context.WithCancel(base)
	c.stopScan = stopScan
	c.forceCtx, c.abort = context.WithCancel(base)

	c.wg.Add(c.cfg.Workers + 1)
	for range c.cfg.Workers {
		go c.worker()
	}
	go c.scan(scanCtx)

	c.logger.InfoContext(ctx, "position polling started",
		"check_interval", c.cfg.CheckInterval, "stale_after", c.cfg.StaleAfter,
		"workers", c.cfg.Workers, "queue_size", c.cfg.QueueSize)
}

func (c *Component) Stop(ctx context.Context) error {
	c.stopScan()
	defer c.abort()

	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		c.abort()
		<-done
		c.logger.WarnContext(ctx, "position polling stopped before the queue was drained",
			"abandoned_tasks", c.abandoned.Load())
		return fmt.Errorf("drain position queue: %w", ctx.Err())
	}

	c.logger.InfoContext(ctx, "position polling stopped")
	return nil
}

func (c *Component) Cancel(tripID string) {
	c.inQueue.cancel(tripID)
}

func (c *Component) QueueSize() int {
	return len(c.queue)
}

func (c *Component) ActiveWorkers() int {
	return int(c.active.Load())
}

func (c *Component) scan(ctx context.Context) {
	defer c.wg.Done()
	defer close(c.queue)

	tk := c.newTicker(c.cfg.CheckInterval)
	defer tk.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-tk.C():
			c.tick(ctx)
		}
	}
}

func (c *Component) tick(ctx context.Context) {
	ctx, span := c.tracer.Start(ctx, "positions.scan")
	defer span.End()

	trips, err := c.finder.FindStale(ctx, c.cfg.StaleAfter)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "find stale trips failed")
		if ctx.Err() == nil {
			c.logger.ErrorContext(ctx, "find stale trips failed", "error", err)
		}
		return
	}

	for _, t := range trips {
		if ctx.Err() != nil {
			return
		}
		c.enqueue(ctx, t, span.SpanContext())
	}
}

func (c *Component) enqueue(ctx context.Context, t model.StaleTrip, scan trace.SpanContext) {
	version, added := c.inQueue.add(t.TripID)
	if !added {
		c.metrics.StaleCheck(ctx, resultDuplicate)
		return
	}

	select {
	case c.queue <- task{trip: t, version: version, scan: scan}:
		c.metrics.StaleCheck(ctx, resultQueued)
	default:
		c.inQueue.finish(t.TripID, version)
		c.metrics.StaleCheck(ctx, resultDropped)
		c.metrics.PositionRequest(ctx, resultDropped)
		c.logger.WarnContext(ctx, "position request queue is full, task dropped", "trip_id", t.TripID)
	}
}

func (c *Component) worker() {
	defer c.wg.Done()

	for t := range c.queue {
		c.handle(t)
	}
}

func (c *Component) handle(t task) {
	if c.forceCtx.Err() != nil {
		c.abandoned.Add(1)
		return
	}
	if !c.inQueue.current(t.trip.TripID, t.version) {
		c.logger.DebugContext(c.forceCtx, "position request cancelled before processing", "trip_id", t.trip.TripID)
		return
	}

	c.active.Add(1)
	defer c.active.Add(-1)

	ctx, span := c.tracer.Start(c.forceCtx, "positions.process",
		trace.WithNewRoot(), trace.WithLinks(trace.Link{SpanContext: t.scan}))
	defer span.End()
	ctx, cancel := context.WithTimeout(ctx, c.cfg.RequestTimeout)
	defer cancel()

	start := time.Now()
	err := c.requestPosition(ctx, t)
	c.inQueue.finish(t.trip.TripID, t.version)

	result := resultSuccess
	if err != nil {
		result = resultError
		span.RecordError(err)
		span.SetStatus(codes.Error, "position request failed")
		c.logger.WarnContext(ctx, "position request failed", "trip_id", t.trip.TripID, "error", err)
	}
	c.metrics.PositionRequest(ctx, result)
	c.metrics.PositionRequestDuration(ctx, result, time.Since(start))
}

func (c *Component) requestPosition(ctx context.Context, t task) error {
	ctx, span := c.tracer.Start(ctx, "push.request_position")
	defer span.End()

	if err := c.gateway.RequestPosition(ctx, t.trip.TripID, t.trip.DriverID); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "push request failed")
		return err
	}
	return nil
}
