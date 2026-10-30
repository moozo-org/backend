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

type RegisterUserHandler struct {
	create CreateUserHandler
}

func NewRegisterUserHandler(users moozo.UserRepository) RegisterUserHandler {
	return RegisterUserHandler{create: NewCreateUserHandler(users)}
}

// Execute returns the created user so the caller can respond without a
// follow-up read. It returns moozo.ErrEmailTaken when the email is in use.
func (h RegisterUserHandler) Execute(ctx context.Context, cmd RegisterUser) (*moozo.User, error) {
	if !cmd.Role.IsSelfService() {
		return nil, fmt.Errorf("%w: %q", ErrRoleNotSelfService, cmd.Role)
	}

	return h.create.Execute(ctx, CreateUser(cmd))
}
