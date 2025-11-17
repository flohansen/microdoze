package logging

import (
	"context"
	"log/slog"
	"os"
)

// ctxKey is used to store context values.
type ctxKey string

const (
	// ctxKeyLogger is used to store a logger as context value.
	ctxKeyLogger ctxKey = "logger"
)

// WithContext stores the given logger as context value.
func WithContext(parent context.Context, logger Logger) context.Context {
	return context.WithValue(parent, ctxKeyLogger, logger)
}

// FromContext loads a logger stored as context value from the given context.
// When there is no logger stored as context value, a new logger is being
// returned.
func FromContext(ctx context.Context) Logger {
	logger, ok := ctx.Value(ctxKeyLogger).(Logger)
	if !ok {
		return NewFromSlog(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	}

	return logger
}
