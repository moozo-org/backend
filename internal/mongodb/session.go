package mongodb

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"moozo/moozo"
)

const SessionsCollectionName = "sessions"

var _ moozo.SessionRepository = (*Repository)(nil)

// sessionDocument is moozo.Session as stored in the sessions collection; its
// fields must match schemas/sessions.json. The token hash is the _id, so a
// lookup is a primary-key read.
type sessionDocument struct {
	TokenHash bson.Binary `bson:"_id"`
	UserID    bson.Binary `bson:"user_id"`
	Role      string      `bson:"role"`
	CreatedAt time.Time   `bson:"created_at"`
	ExpiresAt time.Time   `bson:"expires_at"`
}

func tokenHashID(h moozo.SessionTokenHash) bson.Binary {
	return bson.Binary{Subtype: bson.TypeBinaryGeneric, Data: h[:]}
}

func newSessionDocument(s *moozo.Session) sessionDocument {
	return sessionDocument{
		TokenHash: tokenHashID(s.TokenHash),
		UserID:    bson.Binary{Subtype: bson.TypeBinaryUUID, Data: s.UserID[:]},
		Role:      string(s.Role),
		CreatedAt: s.CreatedAt,
		ExpiresAt: s.ExpiresAt,
	}
}

func (d sessionDocument) toSession() (*moozo.Session, error) {
	var hash moozo.SessionTokenHash
	if len(d.TokenHash.Data) != len(hash) {
		return nil, fmt.Errorf("session _id has %d bytes, want %d", len(d.TokenHash.Data), len(hash))
	}
	copy(hash[:], d.TokenHash.Data)

	userID, err := uuid.FromBytes(d.UserID.Data)
	if err != nil {
		return nil, fmt.Errorf("session user_id: %w", err)
	}
	return &moozo.Session{
		TokenHash: hash,
		UserID:    userID,
		Role:      moozo.Role(d.Role),
		CreatedAt: d.CreatedAt,
		ExpiresAt: d.ExpiresAt,
	}, nil
}

func (r *Repository) CreateSession(ctx context.Context, s *moozo.Session) error {
	_, err := r.sessions.InsertOne(ctx, newSessionDocument(s))
	return err
}

func (r *Repository) FindSession(ctx context.Context, hash moozo.SessionTokenHash) (*moozo.Session, error) {
	var d sessionDocument
	err := r.sessions.FindOne(ctx, bson.D{{Key: "_id", Value: tokenHashID(hash)}}).Decode(&d)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, moozo.ErrSessionNotFound
	}
	if err != nil {
		return nil, err
	}
	return d.toSession()
}

func (r *Repository) DeleteSession(ctx context.Context, hash moozo.SessionTokenHash) error {
	_, err := r.sessions.DeleteOne(ctx, bson.D{{Key: "_id", Value: tokenHashID(hash)}})
	return err
}
