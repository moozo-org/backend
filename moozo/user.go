package moozo

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
)

type Role string

const (
	RolePlanner  Role = "planner"
	RoleProvider Role = "provider"
	RoleAdmin    Role = "admin"
)

// Roles lists every role a stored user may hold, in the order the collection
// validator declares them.
var Roles = []Role{RolePlanner, RoleProvider, RoleAdmin}

// SelfServiceRoles are the roles anyone can grant themselves by registering.
var SelfServiceRoles = []Role{RolePlanner, RoleProvider}

func (r Role) IsSelfService() bool {
	return slices.Contains(SelfServiceRoles, r)
}

// User is storage-agnostic: persistence formats live with their repository.
type User struct {
	ID           uuid.UUID
	Email        string
	PasswordHash string
	Role         Role
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

var ErrEmailTaken = errors.New("email already registered")

type UserRepository interface {
	// Register persists a new account.
	Register(ctx context.Context, u *User) error
}
