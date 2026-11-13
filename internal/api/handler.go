package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/ogen-go/ogen/ogenerrors"
	"go.uber.org/zap"

	"moozo/internal/api/middleware"
	"moozo/internal/api/openapi/generated"
	"moozo/internal/app"
)

type Handler struct {
	logger     *zap.Logger
	production bool
	app        app.Application
}

var _ generated.Handler = (*Handler)(nil)

func NewHandler(logger *zap.Logger, production bool, application app.Application) *Handler {
	return &Handler{logger: logger, production: production, app: application}
}

// NewError shapes handler and security errors into the spec's default
// response. LoggingMiddleware has already logged handler errors; security
// errors happen before middleware runs, so they are logged here.
func (h *Handler) NewError(ctx context.Context, err error) *generated.ErrorStatusCode {
	code := statusCode(err)

	var se *ogenerrors.SecurityError
	if errors.As(err, &se) {
		msg := "authentication failed"
		if code == http.StatusForbidden {
			msg = "access denied"
		}
		middleware.LogAt(middleware.WithTrace(ctx, h.logger), code, msg,
			zap.String("operation", se.OperationName()),
			zap.String("operation_id", se.OperationID()),
			zap.Int("status_code", code),
			zap.Error(err),
		)
	}

	return &generated.ErrorStatusCode{
		StatusCode: code,
		Response:   errorBody(code, err, h.production),
	}
}
