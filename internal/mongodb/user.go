package mongodb

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"moozo/moozo"
)

const UsersCollectionName = "users"

var _ moozo.UserRepository = (*Repository)(nil)

// userDocument is moozo.User as stored in the users collection; its fields
// must match schemas/users.json.
type userDocument struct {
	ID           bson.Binary `bson:"_id"`
	Email        string      `bson:"email"`
	PasswordHash string      `bson:"password_hash"`
	Role         string      `bson:"role"`
	CreatedAt    time.Time   `bson:"created_at"`
	UpdatedAt    time.Time   `bson:"updated_at"`
}

func newUserDocument(u *moozo.User) userDocument {
	return userDocument{
		// Subtype 4 is the standard BSON encoding for UUIDs.
		ID:           bson.Binary{Subtype: bson.TypeBinaryUUID, Data: u.ID[:]},
		Email:        u.Email,
		PasswordHash: u.PasswordHash,
		Role:         string(u.Role),
		CreatedAt:    u.CreatedAt,
		UpdatedAt:    u.UpdatedAt,
	}
}

func (r *Repository) Register(ctx context.Context, u *moozo.User) error {
	_, err := r.users.InsertOne(ctx, newUserDocument(u))
	if mongo.IsDuplicateKeyError(err) {
		return moozo.ErrEmailTaken
	}
	return err
}
