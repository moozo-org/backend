package command

import (
	"context"
	"errors"
	"fmt"

	"moozo/moozo"
)

// ErrInvalidInput is wrapped by every error caused by the caller's input,
// so transports can map them all to one response.
var ErrInvalidInput = errors.New("invalid input")

var (
	// ErrRoleNotSelfService is returned when the requested role, such as
	// admin, cannot be granted through sign-up.
	ErrRoleNotSelfService = fmt.Errorf("%w: role cannot be self-assigned", ErrInvalidInput)

	// ErrPasswordTooLong is returned when the password exceeds bcrypt's
	// 72-byte limit, which a character-count check can miss.
	ErrPasswordTooLong = fmt.Errorf("%w: password must be at most 72 bytes", ErrInvalidInput)
)

type RegisterUser struct {
	Email    string
	Password string
	Role     moozo.Role
}

// ErrVerificationNotSent is returned, together with the created user, when
// the account exists but its verification email could not be sent. The user
// can ask for another one through ResendVerification.
var ErrVerificationNotSent = errors.New("verification email not sent")

type RegisterUserHandler struct {
	create CreateUserHandler
	send   sendVerificationCode
}

func NewRegisterUserHandler(users moozo.UserRepository, codes moozo.VerificationCodeRepository, mailer moozo.Mailer) RegisterUserHandler {
	return RegisterUserHandler{
		create: NewCreateUserHandler(users),
		send:   sendVerificationCode{codes: codes, mailer: mailer},
	}
}

// Execute creates a pending user and emails them a verification code. It
// returns the created user so the caller can respond without a follow-up
// read, and moozo.ErrEmailTaken when the email is in use.
func (h RegisterUserHandler) Execute(ctx context.Context, cmd RegisterUser) (*moozo.User, error) {
	if !cmd.Role.IsSelfService() {
		return nil, fmt.Errorf("%w: %q", ErrRoleNotSelfService, cmd.Role)
	}

	u, err := h.create.Execute(ctx, CreateUser{
		Email:    cmd.Email,
		Password: cmd.Password,
		Role:     cmd.Role,
		Status:   moozo.StatusPending,
	})
	if err != nil {
		return nil, err
	}
	if err := h.send.send(ctx, u); err != nil {
		return u, fmt.Errorf("%w: %w", ErrVerificationNotSent, err)
	}
	return u, nil
}
