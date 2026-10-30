package command

import (
	"errors"
	"testing"
	"testing/synctest"

	"moozo/moozo"
	"moozo/moozo/moozotest"
)

func TestCreateUser(t *testing.T) {
	// Validation and normalization are covered through RegisterUser, which
	// delegates here; this only checks the role restriction is not applied.
	for _, role := range moozo.Roles {
		t.Run(string(role), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				repo := &moozotest.Repository{}
				cmd := CreateUser{Email: "jane@example.com", Password: "correct horse", Role: role}

				u, err := NewCreateUserHandler(repo).Execute(t.Context(), cmd)
				if err != nil {
					t.Fatalf("Execute() error = %v", err)
				}
				assertRegistered(t, RegisterUser(cmd), u, repo)
			})
		})
	}
}

func TestCreateUserEmailTaken(t *testing.T) {
	repo := &moozotest.Repository{}
	if err := repo.Register(t.Context(), &moozo.User{Email: "jane@example.com"}); err != nil {
		t.Fatal(err)
	}

	_, err := NewCreateUserHandler(repo).Execute(t.Context(),
		CreateUser{Email: "Jane@example.com", Password: "password123", Role: moozo.RoleAdmin})
	if !errors.Is(err, moozo.ErrEmailTaken) {
		t.Fatalf("Execute() error = %v, want %v", err, moozo.ErrEmailTaken)
	}
}
