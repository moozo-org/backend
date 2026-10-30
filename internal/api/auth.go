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

	res := userResponse(u)
	return &res, nil
}

func (h *Handler) Login(ctx context.Context, req *generated.LoginRequest) (generated.LoginRes, error) {
	res, err := h.app.Commands.Login.Execute(ctx, command.Login{
		Email:    req.Email,
		Password: req.Password,
	})
	if errors.Is(err, command.ErrInvalidCredentials) {
		return &generated.Error{ErrorMessage: err.Error()}, nil
	}
	if err != nil {
		return nil, err
	}

	return &generated.Session{
		Token:     res.Token,
		ExpiresAt: res.Session.ExpiresAt,
		User:      userResponse(res.User),
	}, nil
}

func (h *Handler) Logout(ctx context.Context) (generated.LogoutRes, error) {
	s, ok := sessionFrom(ctx)
	if !ok {
		return nil, errors.New("logout: no session in context; is bearerAuth declared on the operation?")
	}
	if _, err := h.app.Commands.Logout.Execute(ctx, command.Logout{TokenHash: s.TokenHash}); err != nil {
		return nil, err
	}
	return &generated.LogoutNoContent{}, nil
}

func userResponse(u *moozo.User) generated.User {
	return generated.User{
		ID:        u.ID,
		Email:     u.Email,
		Role:      generated.Role(u.Role),
		CreatedAt: u.CreatedAt,
	}
}
