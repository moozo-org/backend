package mongodb

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/mongo"

	"moozo/internal/mongodb/schemas"
)

// Repository is the MongoDB implementation of the domain repositories.
type Repository struct {
	users *mongo.Collection
}

// NewRepository applies every collection schema before returning, so a
// Repository never runs against a database without its validators and indexes.
func NewRepository(ctx context.Context, db *mongo.Database) (*Repository, error) {
	if err := schemas.Init(ctx, db); err != nil {
		return nil, err
	}
	return &Repository{
		users: db.Collection(UsersCollectionName),
	}, nil
}
