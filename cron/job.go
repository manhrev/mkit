package cron

import (
	"context"
	redsyncutil "github.com/manhrev/mkit/cache/redsync"
	"log/slog"
	"time"

	"github.com/go-redsync/redsync/v4"
	"go.opentelemetry.io/otel"
)

const (
	defaultTaskTimeout = time.Minute * 2
	// lockExpiryBuffer pads the initial lock expiry past taskTimeout so the
	// lock can't lapse before the first MaintainLock renewal tick has a
	// chance to run.
	lockExpiryBuffer = 30 * time.Second
)

// Handler is the function a Cronjob runs on each scheduled tick.
type Handler func(ctx context.Context) error

// Cronjob is a job to run regularly
// ID is unique string represent it, eg: A:CRONJOB:MANAGE_COLLECTION_AUTOSTART, it's also used for redsync mutex lock name
type Cronjob struct {
	ID          string
	logger      *slog.Logger
	taskTimeout time.Duration
	handler     Handler
	redSync     *redsync.Redsync
}

func NewCronjob(
	id string, taskTimeout time.Duration, logger *slog.Logger, redSync *redsync.Redsync, handler Handler,
) *Cronjob {
	if taskTimeout == 0 {
		taskTimeout = defaultTaskTimeout
	}

	return &Cronjob{
		ID:          id,
		logger:      logger,
		taskTimeout: taskTimeout,
		handler:     handler,
		redSync:     redSync,
	}
}

// Run wraps Handler for tracing, single pod checking, logging
func (c *Cronjob) Run(sctx context.Context) {
	var (
		mutex = c.redSync.NewMutex(
			c.ID,
			redsync.WithExpiry(c.taskTimeout+lockExpiryBuffer), // lock expiration
			redsync.WithTries(5), // retry attempts
			redsync.WithRetryDelay(200*time.Millisecond),
		)
		tr           = otel.Tracer("humatest.pkg.cron")
		rctx, cancel = context.WithTimeout(sctx, c.taskTimeout)
		ctx, span    = tr.Start(rctx, "Cron."+c.ID)
		logger       = c.logger.With("cronID", c.ID)
	)

	defer cancel()
	defer span.End()

	ok, err := redsyncutil.TryLock(ctx, mutex)
	if err != nil {
		logger.Error("cannot acquire lock for cronjob", "error", err)

		return
	}
	if !ok {
		logger.Info("cronjob is already running, skipped")

		return
	}

	// Keep the lock alive for as long as the handler runs — without this,
	// a handler running longer than the lock's expiry would let another
	// pod's tick acquire the (by-then-expired) lock and run the same job
	// concurrently. maintainCtx is canceled (via the deferred stop) the
	// moment Run returns, which is MaintainLock's own signal to unlock.
	maintainCtx, stopMaintain := context.WithCancel(ctx)
	defer stopMaintain()
	go func() {
		if err := redsyncutil.MaintainLock(maintainCtx, mutex); err != nil {
			logger.Error("failed to maintain cronjob lock", "error", err)
		}
	}()

	if err := c.handler(ctx); err != nil {
		logger.Error("failed to execute cronjob task", "error", err)

		return
	}

	logger.Info("cronjob executed successfully")
}
