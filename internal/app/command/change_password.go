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

var (
	// ErrWrongPassword is returned when the current password does not match.
	ErrWrongPassword = errors.New("current password is incorrect")

	// ErrSamePassword is returned when the new password is the current one.
	ErrSamePassword = fmt.Errorf("%w: new password must differ from the current one", ErrInvalidInput)
)

type ChangePassword struct {
	UserID uuid.UUID
	// Session is the caller's session; it survives, every other session of
	// the user is revoked.
	Session         moozo.SessionTokenHash
	CurrentPassword string
	NewPassword     string
}

type ChangePasswordHandler struct {
	users    moozo.UserRepository
	sessions moozo.SessionRepository
}

func NewChangePasswordHandler(users moozo.UserRepository, sessions moozo.SessionRepository) ChangePasswordHandler {
	return ChangePasswordHandler{users: users, sessions: sessions}
}

// Execute asks for the current password so that a stolen session alone cannot
// take over the account, then revokes the user's other sessions, so a change
// made because the password leaked also logs the attacker out.
func (h ChangePasswordHandler) Execute(ctx context.Context, cmd ChangePassword) (struct{}, error) {
	u, err := h.users.FindUserByID(ctx, cmd.UserID)
	if err != nil {
		return struct{}{}, fmt.Errorf("find user: %w", err)
	}

	err = bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(cmd.CurrentPassword))
	if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) || errors.Is(err, bcrypt.ErrPasswordTooLong) {
		return struct{}{}, ErrWrongPassword
	}
	if err != nil {
		return struct{}{}, fmt.Errorf("compare password: %w", err)
	}
	if cmd.NewPassword == cmd.CurrentPassword {
		return struct{}{}, ErrSamePassword
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(cmd.NewPassword), bcrypt.DefaultCost)
	if errors.Is(err, bcrypt.ErrPasswordTooLong) {
		return struct{}{}, ErrPasswordTooLong
	}
	if err != nil {
		return struct{}{}, fmt.Errorf("hash password: %w", err)
	}

	// MongoDB stores dates at millisecond precision
	now := time.Now().UTC().Truncate(time.Millisecond)
	if err := h.users.UpdatePassword(ctx, u.ID, string(hash), now); err != nil {
		return struct{}{}, fmt.Errorf("update password: %w", err)
	}
	// If this fails, the password has still changed: the caller gets an
	// error and retries with the new password as the current one.
	if err := h.sessions.DeleteOtherSessions(ctx, u.ID, cmd.Session); err != nil {
		return struct{}{}, fmt.Errorf("revoke other sessions: %w", err)
	}
	return struct{}{}, nil
}
