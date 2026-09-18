package api

import (
	"context"
	"net/http"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// withTrace stamps trace and span IDs onto the logger; a no-op until a real
// TracerProvider is registered.
func withTrace(ctx context.Context, logger *zap.Logger) *zap.Logger {
	sc := trace.SpanFromContext(ctx).SpanContext()
	if !sc.IsValid() {
		return logger
	}
	return logger.With(
		zap.String("trace_id", sc.TraceID().String()),
		zap.String("span_id", sc.SpanID().String()),
	)
}

// logAt keeps client errors off the error level, where they would drown real faults.
func logAt(logger *zap.Logger, code int, msg string, fields ...zap.Field) {
	level := zapcore.ErrorLevel
	if code < http.StatusInternalServerError {
		level = zapcore.WarnLevel
	}
	if ce := logger.Check(level, msg); ce != nil {
		ce.Write(fields...)
	}
}
