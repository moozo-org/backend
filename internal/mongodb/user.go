package mongodb

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

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
	Status       string      `bson:"status"`
	CreatedAt    time.Time   `bson:"created_at"`
	UpdatedAt    time.Time   `bson:"updated_at"`
}

// uuidID is the BSON form of a UUID key. Subtype 4 is the standard BSON
// encoding for UUIDs.
func uuidID(id uuid.UUID) bson.Binary {
	return bson.Binary{Subtype: bson.TypeBinaryUUID, Data: id[:]}
}

func newUserDocument(u *moozo.User) userDocument {
	return userDocument{
		ID:           uuidID(u.ID),
		Email:        u.Email,
		PasswordHash: u.PasswordHash,
		Role:         string(u.Role),
		Status:       string(u.Status),
		CreatedAt:    u.CreatedAt,
		UpdatedAt:    u.UpdatedAt,
	}
}

func (d userDocument) toUser() (*moozo.User, error) {
	id, err := uuid.FromBytes(d.ID.Data)
	if err != nil {
		return nil, fmt.Errorf("user _id: %w", err)
	}
	return &moozo.User{
		ID:           id,
		Email:        d.Email,
		PasswordHash: d.PasswordHash,
		Role:         moozo.Role(d.Role),
		Status:       moozo.Status(d.Status),
		CreatedAt:    d.CreatedAt,
		UpdatedAt:    d.UpdatedAt,
	}, nil
}

// emailCollation must match the email_unique index in schemas/users.json;
// a query uses that index only with the same collation.
var emailCollation = &options.Collation{Locale: "en", Strength: 2}

func (r *Repository) Register(ctx context.Context, u *moozo.User) error {
	_, err := r.users.InsertOne(ctx, newUserDocument(u))
	if mongo.IsDuplicateKeyError(err) {
		return moozo.ErrEmailTaken
	}
	return err
}

func (r *Repository) FindUserByEmail(ctx context.Context, email string) (*moozo.User, error) {
	var d userDocument
	err := r.users.FindOne(ctx, bson.D{{Key: "email", Value: email}},
		options.FindOne().SetCollation(emailCollation),
	).Decode(&d)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, moozo.ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return d.toUser()
}

func (r *Repository) ActivateUser(ctx context.Context, id uuid.UUID, at time.Time) error {
	res, err := r.users.UpdateOne(ctx, bson.D{{Key: "_id", Value: uuidID(id)}}, bson.D{{Key: "$set", Value: bson.D{
		{Key: "status", Value: string(moozo.StatusActive)},
		{Key: "updated_at", Value: at},
	}}})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return moozo.ErrUserNotFound
	}
	return nil
}
