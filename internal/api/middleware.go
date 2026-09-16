package api

import (
	"time"

	"github.com/ogen-go/ogen/middleware"
	"github.com/ogen-go/ogen/ogenerrors"
	"go.uber.org/zap"
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
			logAt(withTrace(req.Context, logger), code, "request failed",
				append(fields, zap.Int("status_code", code), zap.Error(err))...)
			return resp, err
		}

		withTrace(req.Context, logger).Info("request served", fields...)

		return resp, err
	}
}
