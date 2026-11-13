package command

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"moozo/moozo"
)

// ErrInvalidResetCode covers an unknown email, a wrong, expired, exhausted or
// already used code, and no code at all, so callers cannot tell them apart.
var ErrInvalidResetCode = errors.New("invalid or expired reset code")

type ResetPassword struct {
	Email       string
	Code        string
	NewPassword string
}

type ResetPasswordHandler struct {
	users    moozo.UserRepository
	sessions moozo.SessionRepository
	codes    moozo.PasswordResetCodeRepository
}

func NewResetPasswordHandler(users moozo.UserRepository, sessions moozo.SessionRepository, codes moozo.PasswordResetCodeRepository) ResetPasswordHandler {
	return ResetPasswordHandler{users: users, sessions: sessions, codes: codes}
}

// Execute sets a new password when the code matches, then logs the user out
// everywhere: whoever had the old password must not keep a session. The code
// works once. A pending user is activated too, since the code proves they own
// the email.
func (h ResetPasswordHandler) Execute(ctx context.Context, cmd ResetPassword) (struct{}, error) {
	// Hashing first rejects an invalid password without spending an attempt.
	hash, err := bcrypt.GenerateFromPassword([]byte(cmd.NewPassword), bcrypt.DefaultCost)
	if errors.Is(err, bcrypt.ErrPasswordTooLong) {
		return struct{}{}, ErrPasswordTooLong
	}
	if err != nil {
		return struct{}{}, fmt.Errorf("hash password: %w", err)
	}

	u, err := h.users.FindUserByEmail(ctx, normalizeEmail(cmd.Email))
	if errors.Is(err, moozo.ErrUserNotFound) {
		return struct{}{}, ErrInvalidResetCode
	}
	if err != nil {
		return struct{}{}, fmt.Errorf("find user: %w", err)
	}

	ok, err := codeAttempts{consume: h.codes.ConsumePasswordResetAttempt, remove: h.deleteCode}.check(ctx, u.ID, cmd.Code)
	if err != nil {
		return struct{}{}, err
	}
	if !ok {
		return struct{}{}, ErrInvalidResetCode
	}
	// Deleting before using the code makes it single-use: of two concurrent
	// resets with it, only the one that deletes it goes on.
	deleted, err := h.codes.DeletePasswordResetCode(ctx, u.ID)
	if err != nil {
		return struct{}{}, fmt.Errorf("delete reset code: %w", err)
	}
	if !deleted {
		return struct{}{}, ErrInvalidResetCode
	}

	// MongoDB stores dates at millisecond precision
	now := time.Now().UTC().Truncate(time.Millisecond)
	if err := h.users.UpdatePassword(ctx, u.ID, string(hash), now); err != nil {
		return struct{}{}, fmt.Errorf("update password: %w", err)
	}
	if u.Status == moozo.StatusPending {
		if err := h.users.ActivateUser(ctx, u.ID, now); err != nil {
			return struct{}{}, fmt.Errorf("activate user: %w", err)
		}
	}
	if err := h.sessions.DeleteUserSessions(ctx, u.ID); err != nil {
		return struct{}{}, fmt.Errorf("revoke sessions: %w", err)
	}
	return struct{}{}, nil
}

func (h ResetPasswordHandler) deleteCode(ctx context.Context, userID uuid.UUID) error {
	_, err := h.codes.DeletePasswordResetCode(ctx, userID)
	return err
}
