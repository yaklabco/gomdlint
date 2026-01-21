package logging

import (
	"context"

	"github.com/charmbracelet/log"
)

// contextKey is the type for context keys used by this package.
type contextKey struct{}

// loggerKey is the key used to store the logger in context.
//
//nolint:gochecknoglobals // Package-level context key is idiomatic
var loggerKey = contextKey{}

// FromContext retrieves a Logger from context, or returns the default logger.
func FromContext(ctx context.Context) *log.Logger {
	if ctx == nil {
		return Default()
	}
	if logger, ok := ctx.Value(loggerKey).(*log.Logger); ok && logger != nil {
		return logger
	}
	return Default()
}

// WithLogger returns a context with the given logger attached.
func WithLogger(ctx context.Context, logger *log.Logger) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, loggerKey, logger)
}
