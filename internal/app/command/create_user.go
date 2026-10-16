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

// CreateUser creates an account with any role, admin included. It is for
// trusted callers such as the seed command; sign-up goes through RegisterUser.
type CreateUser struct {
	Email    string
	Password string
	Role     moozo.Role
}

type CreateUserHandler struct {
	users moozo.UserRepository
}

func NewCreateUserHandler(users moozo.UserRepository) CreateUserHandler {
	return CreateUserHandler{users: users}
}

// Execute returns the created user. It returns moozo.ErrEmailTaken when the
// email is in use.
func (h CreateUserHandler) Execute(ctx context.Context, cmd CreateUser) (*moozo.User, error) {
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
