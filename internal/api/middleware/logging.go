package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/ogen-go/ogen/middleware"
	"github.com/ogen-go/ogen/ogenerrors"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// LoggingMiddleware logs every operation it wraps. It is the only place with
// the operation name and duration, so it logs failures too; NewError stays
// silent to avoid a second line.
func LoggingMiddleware(logger *zap.Logger) middleware.Middleware {
	return func(
		req middleware.Request,
		next func(req middleware.Request) (middleware.Response, error),
	) (middleware.Response, error) {
		start := time.Now()

		resp, err := next(req)

		fields := []zap.Field{
			zap.String("operation", req.OperationName),
			zap.String("operation_id", req.OperationID),
			zap.Int64("took_ms", time.Since(start).Milliseconds()),
		}

		if err != nil {
			code := ogenerrors.ErrorCode(err)
			LogAt(WithTrace(req.Context, logger), code, "request failed",
				append(fields, zap.Int("status_code", code), zap.Error(err))...)
			return resp, err
		}

		WithTrace(req.Context, logger).Info("request served", fields...)

		return resp, err
	}
}

// WithTrace stamps trace and span IDs onto the logger; a no-op until a real
// TracerProvider is registered.
func WithTrace(ctx context.Context, logger *zap.Logger) *zap.Logger {
	sc := trace.SpanFromContext(ctx).SpanContext()
	if !sc.IsValid() {
		return logger
	}
	return logger.With(
		zap.String("trace_id", sc.TraceID().String()),
		zap.String("span_id", sc.SpanID().String()),
	)
}

// LogAt keeps client errors off the error level, where they would drown real
// faults. The entry's caller is LogAt's caller, not LogAt itself.
func LogAt(logger *zap.Logger, code int, msg string, fields ...zap.Field) {
	level := zapcore.ErrorLevel
	if code < http.StatusInternalServerError {
		level = zapcore.WarnLevel
	}
	if ce := logger.WithOptions(zap.AddCallerSkip(1)).Check(level, msg); ce != nil {
		ce.Write(fields...)
	}
}
