// Package mongotest provides throwaway MongoDB databases for integration tests.
package mongotest

import (
	"context"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// URIEnv names the variable holding the test server's URI. Tests that need a
// database are skipped when it is unset, except in CI, where they fail so a
// misconfigured pipeline cannot pass by skipping them.
const URIEnv = "MONGODB_TEST_URI"

const timeout = 10 * time.Second

// NewDatabase returns an empty database with a unique name, dropped when the
// test ends, so tests can run in parallel against one server.
func NewDatabase(t testing.TB) *mongo.Database {
	t.Helper()

	uri := os.Getenv(URIEnv)
	if uri == "" {
		if os.Getenv("CI") != "" {
			t.Fatalf("%s must be set in CI", URIEnv)
		}
		t.Skipf("%s not set; skipping MongoDB integration test", URIEnv)
	}

	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatalf("connect to %s: %v", uri, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := client.Ping(ctx, nil); err != nil {
		t.Fatalf("ping %s: %v", uri, err)
	}

	db := client.Database("test_" + bson.NewObjectID().Hex())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		_ = db.Drop(ctx)
		_ = client.Disconnect(ctx)
	})
	return db
}
