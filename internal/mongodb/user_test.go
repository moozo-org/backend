package mongodb

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"moozo/moozo"
)

// documentValidationFailure is the server error code for a write the
// collection's $jsonSchema rejects.
const documentValidationFailure = 121

func newUser(email string, role moozo.Role) *moozo.User {
	now := time.Now().UTC().Truncate(time.Millisecond)
	return &moozo.User{
		ID:           uuid.Must(uuid.NewV7()),
		Email:        email,
		PasswordHash: "$2a$10$hash",
		Role:         role,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

func TestRegister(t *testing.T) {
	cases := map[string]struct {
		existing []*moozo.User // registered before the user under test
		user     *moozo.User
		wantErr  func(error) bool
	}{
		"stores user": {
			user:    newUser("jane@example.com", moozo.RolePlanner),
			wantErr: func(err error) bool { return err == nil },
		},
		"email taken": {
			existing: []*moozo.User{newUser("jane@example.com", moozo.RoleProvider)},
			user:     newUser("jane@example.com", moozo.RolePlanner),
			wantErr:  func(err error) bool { return errors.Is(err, moozo.ErrEmailTaken) },
		},
		// The index is case-insensitive, so uniqueness does not depend on
		// every caller normalizing emails first.
		"email taken with different case": {
			existing: []*moozo.User{newUser("jane@example.com", moozo.RoleProvider)},
			user:     newUser("Jane@Example.COM", moozo.RolePlanner),
			wantErr:  func(err error) bool { return errors.Is(err, moozo.ErrEmailTaken) },
		},
		// Only the email index maps to ErrEmailTaken; a validator rejection
		// is a bug upstream and must surface as-is.
		"schema violation": {
			user: newUser("jane@example.com", "root"),
			wantErr: func(err error) bool {
				var se mongo.ServerError
				return errors.As(err, &se) && se.HasErrorCode(documentValidationFailure)
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			repo := newRepository(t)
			for _, u := range tc.existing {
				if err := repo.Register(t.Context(), u); err != nil {
					t.Fatalf("register existing %s: %v", u.Email, err)
				}
			}

			err := repo.Register(t.Context(), tc.user)
			if !tc.wantErr(err) {
				t.Fatalf("Register() error = %v", err)
			}
			if err != nil {
				return
			}

			want := newUserDocument(tc.user)
			var got userDocument
			if err := repo.users.FindOne(t.Context(), bson.D{{Key: "_id", Value: want.ID}}).Decode(&got); err != nil {
				t.Fatalf("find registered user: %v", err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("stored %+v, want %+v", got, want)
			}
		})
	}
}
