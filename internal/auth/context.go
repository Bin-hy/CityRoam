package auth

import (
	"context"
	"log/slog"
)

type loggerContextKey struct{}

// WithLogger attaches a request-scoped logger so auth events carry request_id.
func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	if logger == nil {
		return ctx
	}
	return context.WithValue(ctx, loggerContextKey{}, logger)
}

func (s *Service) loggerFor(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(loggerContextKey{}).(*slog.Logger); ok && logger != nil {
		return logger
	}
	return s.logger
}
