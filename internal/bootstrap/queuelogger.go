package bootstrap

import (
	"fmt"
	"log/slog"
)

type queueLogger struct {
	logger *slog.Logger
}

func newQueueLogger(logger *slog.Logger) queueLogger {
	return queueLogger{logger: logger.With("component", "queue")}
}

func (l queueLogger) Debug(args ...any) {
	l.logger.Debug(fmt.Sprint(args...))
}

func (l queueLogger) Info(args ...any) {
	l.logger.Info(fmt.Sprint(args...))
}

func (l queueLogger) Warn(args ...any) {
	l.logger.Warn(fmt.Sprint(args...))
}

func (l queueLogger) Error(args ...any) {
	l.logger.Error(fmt.Sprint(args...))
}

func (l queueLogger) Fatal(args ...any) {
	l.logger.Error(fmt.Sprint(args...))
}
