package mongodb

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const connectTimeout = 10 * time.Second

// Connect connects to uri and pings the server, so callers fail fast when
// MongoDB is unreachable. The returned func disconnects the client.
func Connect(uri, database string) (*mongo.Database, func(), error) {
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return nil, nil, fmt.Errorf("connect to mongo: %w", err)
	}
	disconnect := func() {
		ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
		defer cancel()
		_ = client.Disconnect(ctx)
	}

	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()

	if err := client.Ping(ctx, nil); err != nil {
		disconnect()
		return nil, nil, fmt.Errorf("ping mongo: %w", err)
	}

	return client.Database(database), disconnect, nil
}
