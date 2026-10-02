package cron

import "log/slog"

// slogCronLogger adapts *slog.Logger to robfig/cron's Logger interface, so
// cron.WithChain(cron.Recover(...)) below can log through the app's normal
// logger instead of a separate one.
type slogCronLogger struct {
	logger *slog.Logger
}

func (l slogCronLogger) Info(msg string, keysAndValues ...any) {
	l.logger.Info(msg, keysAndValues...)
}

func (l slogCronLogger) Error(err error, msg string, keysAndValues ...any) {
	l.logger.Error(msg, append(keysAndValues, "error", err)...)
}
