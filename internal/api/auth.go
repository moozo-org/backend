package api

import (
	"context"
	"errors"
	"net/http"

	"moozo/internal/api/openapi/generated"
	"moozo/internal/app/command"
	"moozo/moozo"
)

func (h *Handler) Register(ctx context.Context, req *generated.RegisterRequest) (generated.RegisterRes, error) {
	u, err := h.app.Commands.RegisterUser.Execute(ctx, command.RegisterUser{
		Email:    req.Email,
		Password: req.Password,
		Role:     moozo.Role(req.Role),
	})
	switch {
	case errors.Is(err, command.ErrInvalidInput):
		return &generated.RegisterBadRequest{ErrorMessage: err.Error()}, nil
	case errors.Is(err, moozo.ErrEmailTaken):
		return &generated.RegisterConflict{ErrorMessage: http.StatusText(http.StatusConflict)}, nil
	case err != nil:
		return nil, err
	}

	return &generated.User{
		ID:        u.ID,
		Email:     u.Email,
		Role:      generated.Role(u.Role),
		CreatedAt: u.CreatedAt,
	}, nil
}
