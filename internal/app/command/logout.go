package command

import (
	"context"
	"fmt"

	"moozo/moozo"
)

type Logout struct {
	TokenHash moozo.SessionTokenHash
}

type LogoutHandler struct {
	sessions moozo.SessionRepository
}

func NewLogoutHandler(sessions moozo.SessionRepository) LogoutHandler {
	return LogoutHandler{sessions: sessions}
}

// Execute revokes the session; it succeeds if the session is already gone.
func (h LogoutHandler) Execute(ctx context.Context, cmd Logout) (struct{}, error) {
	if err := h.sessions.DeleteSession(ctx, cmd.TokenHash); err != nil {
		return struct{}{}, fmt.Errorf("delete session: %w", err)
	}
	return struct{}{}, nil
}
