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

const VerificationCodesCollectionName = "verification_codes"

var _ moozo.VerificationCodeRepository = (*Repository)(nil)

// verificationCodeDocument is moozo.VerificationCode as stored in the
// verification_codes collection; its fields must match
// schemas/verification_codes.json. The user ID is the _id, so a user has at
// most one code and a lookup is a primary-key read.
type verificationCodeDocument struct {
	UserID    bson.Binary `bson:"_id"`
	CodeHash  bson.Binary `bson:"code_hash"`
	Attempts  int32       `bson:"attempts"`
	CreatedAt time.Time   `bson:"created_at"`
	ExpiresAt time.Time   `bson:"expires_at"`
}

func newVerificationCodeDocument(c *moozo.VerificationCode) verificationCodeDocument {
	return verificationCodeDocument{
		UserID:    uuidID(c.UserID),
		CodeHash:  bson.Binary{Subtype: bson.TypeBinaryGeneric, Data: c.CodeHash[:]},
		Attempts:  int32(c.Attempts), //nolint:gosec // bounded by MaxVerificationAttempts
		CreatedAt: c.CreatedAt,
		ExpiresAt: c.ExpiresAt,
	}
}

func (d verificationCodeDocument) toVerificationCode() (*moozo.VerificationCode, error) {
	var hash moozo.VerificationCodeHash
	if len(d.CodeHash.Data) != len(hash) {
		return nil, fmt.Errorf("verification code_hash has %d bytes, want %d", len(d.CodeHash.Data), len(hash))
	}
	copy(hash[:], d.CodeHash.Data)

	userID, err := uuid.FromBytes(d.UserID.Data)
	if err != nil {
		return nil, fmt.Errorf("verification code _id: %w", err)
	}
	return &moozo.VerificationCode{
		UserID:    userID,
		CodeHash:  hash,
		Attempts:  int(d.Attempts),
		CreatedAt: d.CreatedAt,
		ExpiresAt: d.ExpiresAt,
	}, nil
}

func (r *Repository) SaveVerificationCode(ctx context.Context, c *moozo.VerificationCode) error {
	return codeStore{r.codes}.save(ctx, c)
}

func (r *Repository) FindVerificationCode(ctx context.Context, userID uuid.UUID) (*moozo.VerificationCode, error) {
	return codeStore{r.codes}.find(ctx, userID)
}

func (r *Repository) ConsumeVerificationAttempt(ctx context.Context, userID uuid.UUID) (*moozo.VerificationCode, error) {
	return codeStore{r.codes}.consume(ctx, userID)
}

func (r *Repository) DeleteVerificationCode(ctx context.Context, userID uuid.UUID) error {
	_, err := codeStore{r.codes}.delete(ctx, userID)
	return err
}

// codeStore implements the code operations over a collection of
// verificationCodeDocuments; verification and password-reset codes each have
// their own collection.
type codeStore struct {
	coll *mongo.Collection
}

func (s codeStore) save(ctx context.Context, c *moozo.VerificationCode) error {
	d := newVerificationCodeDocument(c)
	_, err := s.coll.ReplaceOne(ctx, bson.D{{Key: "_id", Value: d.UserID}}, d, options.Replace().SetUpsert(true))
	return err
}

func (s codeStore) find(ctx context.Context, userID uuid.UUID) (*moozo.VerificationCode, error) {
	return decodeVerificationCode(s.coll.FindOne(ctx, bson.D{{Key: "_id", Value: uuidID(userID)}}))
}

func (s codeStore) consume(ctx context.Context, userID uuid.UUID) (*moozo.VerificationCode, error) {
	return decodeVerificationCode(s.coll.FindOneAndUpdate(ctx,
		bson.D{{Key: "_id", Value: uuidID(userID)}},
		bson.D{{Key: "$inc", Value: bson.D{{Key: "attempts", Value: int32(1)}}}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	))
}

func (s codeStore) delete(ctx context.Context, userID uuid.UUID) (bool, error) {
	res, err := s.coll.DeleteOne(ctx, bson.D{{Key: "_id", Value: uuidID(userID)}})
	if err != nil {
		return false, err
	}
	return res.DeletedCount > 0, nil
}

func decodeVerificationCode(res *mongo.SingleResult) (*moozo.VerificationCode, error) {
	var d verificationCodeDocument
	err := res.Decode(&d)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, moozo.ErrVerificationCodeNotFound
	}
	if err != nil {
		return nil, err
	}
	return d.toVerificationCode()
}
