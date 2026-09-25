package api

import (
	"context"

	"github.com/ogen-go/ogen/ogenerrors"
	"go.uber.org/zap"

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

// NewError shapes handler errors into the spec's default response.
// LoggingMiddleware has already logged them.
func (h *Handler) NewError(ctx context.Context, err error) *generated.ErrorStatusCode {
	code := ogenerrors.ErrorCode(err)

	return &generated.ErrorStatusCode{
		StatusCode: code,
		Response:   errorBody(code, err, h.production),
	}
}
