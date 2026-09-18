package api

import (
	"context"

	"github.com/ogen-go/ogen/ogenerrors"
	"go.uber.org/zap"

	"moozo/internal/api/generated"
)

type Handler struct {
	logger     *zap.Logger
	production bool
}

var _ generated.Handler = (*Handler)(nil)

func NewHandler(logger *zap.Logger, production bool) *Handler {
	return &Handler{logger: logger, production: production}
}

func (h *Handler) Hello(ctx context.Context) (*generated.HelloOK, error) {
	return &generated.HelloOK{
		Message: "Hello, World!",
	}, nil
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
