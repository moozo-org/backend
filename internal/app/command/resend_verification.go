package command

import (
	"context"
	"errors"
	"fmt"
	"time"

	"moozo/moozo"
)

// ResendCooldown is the minimum time between two codes sent to one user, so
// the endpoint cannot be used to flood an inbox.
const ResendCooldown = time.Minute

type ResendVerification struct {
	Email string
}

type ResendVerificationHandler struct {
	users moozo.UserRepository
	codes moozo.VerificationCodeRepository
	send  sendVerificationCode
}

func NewResendVerificationHandler(users moozo.UserRepository, codes moozo.VerificationCodeRepository, mailer moozo.Mailer) ResendVerificationHandler {
	return ResendVerificationHandler{
		users: users,
		codes: codes,
		send:  sendVerificationCode{codes: codes, mailer: mailer},
	}
}

// Execute emails a new code to a pending user, replacing the previous one. It
// also succeeds, sending nothing, for an unknown email, an active account or
// within ResendCooldown of the last code, so the response reveals nothing.
func (h ResendVerificationHandler) Execute(ctx context.Context, cmd ResendVerification) (struct{}, error) {
	u, err := h.users.FindUserByEmail(ctx, normalizeEmail(cmd.Email))
	if errors.Is(err, moozo.ErrUserNotFound) {
		return struct{}{}, nil
	}
	if err != nil {
		return struct{}{}, fmt.Errorf("find user: %w", err)
	}
	if u.Status != moozo.StatusPending {
		return struct{}{}, nil
	}

	c, err := h.codes.FindVerificationCode(ctx, u.ID)
	switch {
	case err == nil && time.Since(c.CreatedAt) < ResendCooldown:
		return struct{}{}, nil
	case err != nil && !errors.Is(err, moozo.ErrVerificationCodeNotFound):
		return struct{}{}, fmt.Errorf("find verification code: %w", err)
	}

	return struct{}{}, h.send.send(ctx, u)
}
