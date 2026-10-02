package cron

import (
	"context"
	"fmt"
	"github.com/manhrev/mkit/config"
	"log/slog"
	"sync"
	"time"

	"github.com/go-redsync/redsync/v4"
	"github.com/robfig/cron/v3"
)

// Service is a manager to initialize cronjob at microservice startup time
// cron jobs and its configs must not be changed at runtime
type Service struct {
	cron          *cron.Cron
	logger        *slog.Logger
	redSync       *redsync.Redsync
	location      *time.Location
	ctx           context.Context
	ctxCancelFunc context.CancelFunc
	registry      *sync.Map
}

func New(
	rootCtx context.Context, location *time.Location, logger *slog.Logger, redSync *redsync.Redsync,
) *Service {
	ctx, cancelFunc := context.WithCancel(context.WithoutCancel(rootCtx))

	s := &Service{
		// cron.Recover stops a panicking job's handler from crashing the
		// whole process (and every other scheduled job with it) — without
		// this, robfig/cron does not recover panics on its own.
		cron:          cron.New(cron.WithLocation(location), cron.WithChain(cron.Recover(slogCronLogger{logger}))),
		logger:        logger,
		redSync:       redSync,
		location:      location,
		ctx:           ctx,
		ctxCancelFunc: cancelFunc,
		registry:      new(sync.Map),
	}

	s.cron.Start()

	return s
}

func (s *Service) Stop() error {
	s.ctxCancelFunc()
	ctx := s.cron.Stop()
	<-ctx.Done()

	return ctx.Err()
}

func (s *Service) ScheduleCron(cfg *config.Cronjob, handler Handler) error {
	if _, loaded := s.registry.LoadOrStore(cfg.ID, struct{}{}); loaded {
		return fmt.Errorf("cron [%s] is already scheduled", cfg.ID)
	}

	if cfg.Disabled {
		s.logger.Info("skip schedule cronjob, disabled", "cronID", cfg.ID)

		return nil
	}

	c := NewCronjob(cfg.ID, cfg.TaskTimeout, s.logger, s.redSync, handler)

	if _, err := s.cron.AddFunc(cfg.Spec, func() {
		c.Run(s.ctx)
	}); err != nil {
		return err
	}
	s.logger.Info("cron scheduled", "cronID", cfg.ID, "spec", cfg.Spec)

	return nil
}
