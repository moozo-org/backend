package api

import (
	"context"
	"moozo/internal/api/generated"
)

type Handler struct{}

func (h *Handler) Hello(ctx context.Context) (*generated.HelloOK, error) {
	return &generated.HelloOK{
		Message: "Hello, World!",
	}, nil
}

var _ generated.Handler = (*Handler)(nil)

func NewHandler() *Handler {
	return &Handler{}
}
