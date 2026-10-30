package command

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

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
	users moozo.UserRepository
}

func NewRegisterUserHandler(users moozo.UserRepository) RegisterUserHandler {
	return RegisterUserHandler{users: users}
}

// Execute returns the created user so the caller can respond without a
// follow-up read. It returns moozo.ErrEmailTaken when the email is in use.
func (h RegisterUserHandler) Execute(ctx context.Context, cmd RegisterUser) (*moozo.User, error) {
	if !cmd.Role.IsSelfService() {
		return nil, fmt.Errorf("%w: %q", ErrRoleNotSelfService, cmd.Role)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(cmd.Password), bcrypt.DefaultCost)
	if errors.Is(err, bcrypt.ErrPasswordTooLong) {
		return nil, ErrPasswordTooLong
	}
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	// v7 UUIDs are time-ordered, which keeps the _id index append-mostly.
	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("generate user id: %w", err)
	}

	// MongoDB stores dates at millisecond precision
	now := time.Now().UTC().Truncate(time.Millisecond)
	u := &moozo.User{
		ID:           id,
		Email:        strings.ToLower(strings.TrimSpace(cmd.Email)),
		PasswordHash: string(hash),
		Role:         cmd.Role,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := h.users.Register(ctx, u); err != nil {
		return nil, fmt.Errorf("register user: %w", err)
	}
	return u, nil
}
