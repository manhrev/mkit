package redsync

import (
	"context"
	"errors"
	"fmt"
	"github.com/manhrev/mkit/tracing"
	"time"

	"github.com/go-redsync/redsync/v4"
)

func TryLock(ctx context.Context, mutex *redsync.Mutex) (bool, error) {
	var errTaken *redsync.ErrTaken
	if err := mutex.LockContext(tracing.WithSkipTrace(ctx)); err != nil {
		if errors.As(err, &errTaken) {
			return false, nil
		} else {
			return false, err
		}
	}

	return true, nil
}

// MaintainLock is a blocked action
func MaintainLock(ctx context.Context, mutex *redsync.Mutex) error {
	var (
		ticker = time.NewTicker(10 * time.Second)
	)
	defer ticker.Stop()

	// Maintain locked mutex
	for {
		select {
		case <-ctx.Done():
			if _, err := mutex.Unlock(); err != nil {
				return fmt.Errorf("failed to unlock mutex: %w", err)
			}

			return nil
		case <-ticker.C:
			if _, err := mutex.ExtendContext(tracing.WithSkipTrace(ctx)); err != nil {
				return fmt.Errorf("failed to extend mutex: %w", err)
			}
		}
	}
}

func IsLockValid(mutex *redsync.Mutex) bool {
	expiry := mutex.Until()
	buffer := 500 * time.Millisecond

	return time.Now().Add(buffer).Before(expiry)
}
