// Package schemas holds one JSON file per MongoDB collection, named after the
// collection, and applies them to a database.
//
// A file holds the collection's $jsonSchema validator and its indexes:
//
//	{
//	  "validator": { "$jsonSchema": { ... } },
//	  "validationLevel": "strict",   // optional, default strict
//	  "validationAction": "error",   // optional, default error
//	  "indexes": [
//	    { "key": { "email": 1 }, "name": "email_unique", "unique": true,
//	      "collation": { "locale": "en", "strength": 2 } },  // optional
//	    { "key": { "expires_at": 1 }, "name": "expires_at_ttl",
//	      "expireAfterSeconds": 0 }                          // optional, TTL
//	  ]
//	}
//
// Files are parsed as MongoDB Extended JSON (relaxed).
package schemas

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

//go:embed *.json
var files embed.FS

// Server error codes.
const (
	// namespaceExists: creating a collection that is already there.
	namespaceExists = 48
	// indexOptionsConflict and indexKeySpecsConflict: creating an index whose
	// name already exists with different options or key.
	indexOptionsConflict  = 85
	indexKeySpecsConflict = 86
)

type Collection struct {
	Name             string  `bson:"-"`
	Validator        bson.D  `bson:"validator"`
	ValidationLevel  string  `bson:"validationLevel"`
	ValidationAction string  `bson:"validationAction"`
	Indexes          []Index `bson:"indexes"`
}

type Index struct {
	Key       bson.D     `bson:"key"`
	Name      string     `bson:"name"`
	Unique    bool       `bson:"unique"`
	Collation *Collation `bson:"collation"`
	// ExpireAfterSeconds makes a TTL index: MongoDB deletes a document once
	// the indexed date is this many seconds in the past.
	ExpireAfterSeconds *int32 `bson:"expireAfterSeconds"`
}

// Collation makes string comparisons locale-aware; strength 2 ignores case,
// so a unique index with it rejects emails differing only by case.
type Collation struct {
	Locale   string `bson:"locale"`
	Strength int    `bson:"strength"`
}

func (idx Index) model() mongo.IndexModel {
	opts := options.Index().SetName(idx.Name).SetUnique(idx.Unique)
	if idx.Collation != nil {
		opts.SetCollation(&options.Collation{Locale: idx.Collation.Locale, Strength: idx.Collation.Strength})
	}
	if idx.ExpireAfterSeconds != nil {
		opts.SetExpireAfterSeconds(*idx.ExpireAfterSeconds)
	}
	return mongo.IndexModel{Keys: idx.Key, Options: opts}
}

// Load parses every embedded schema file.
func Load() ([]Collection, error) {
	names, err := fs.Glob(files, "*.json")
	if err != nil {
		return nil, err
	}

	colls := make([]Collection, 0, len(names))
	for _, name := range names {
		data, err := files.ReadFile(name)
		if err != nil {
			return nil, err
		}

		var c Collection
		if err := bson.UnmarshalExtJSON(data, false, &c); err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}
		c.Name = strings.TrimSuffix(path.Base(name), ".json")
		if c.ValidationLevel == "" {
			c.ValidationLevel = "strict"
		}
		if c.ValidationAction == "" {
			c.ValidationAction = "error"
		}
		colls = append(colls, c)
	}
	return colls, nil
}

// Init creates each collection with its validator and indexes, or brings an
// existing one up to date. It is idempotent. An index whose options changed
// is dropped and rebuilt; indexes removed from a file are not dropped.
func Init(ctx context.Context, db *mongo.Database) error {
	colls, err := Load()
	if err != nil {
		return err
	}
	for _, c := range colls {
		if err := apply(ctx, db, c); err != nil {
			return err
		}
	}
	return nil
}

func apply(ctx context.Context, db *mongo.Database, c Collection) error {
	err := db.CreateCollection(ctx, c.Name, options.CreateCollection().
		SetValidator(c.Validator).
		SetValidationLevel(c.ValidationLevel).
		SetValidationAction(c.ValidationAction),
	)
	var cmdErr mongo.CommandError
	switch {
	case err == nil:
	case errors.As(err, &cmdErr) && cmdErr.Code == namespaceExists:
		// Already there: replace the validator so schema changes roll out.
		err = db.RunCommand(ctx, bson.D{
			{Key: "collMod", Value: c.Name},
			{Key: "validator", Value: c.Validator},
			{Key: "validationLevel", Value: c.ValidationLevel},
			{Key: "validationAction", Value: c.ValidationAction},
		}).Err()
		if err != nil {
			return fmt.Errorf("update %s validator: %w", c.Name, err)
		}
	default:
		return fmt.Errorf("create %s collection: %w", c.Name, err)
	}

	for _, idx := range c.Indexes {
		if err := ensureIndex(ctx, db.Collection(c.Name), idx); err != nil {
			return fmt.Errorf("create %s index %s: %w", c.Name, idx.Name, err)
		}
	}
	return nil
}

// ensureIndex creates idx, rebuilding it when an index of the same name exists
// with other options. Until the rebuild finishes a unique index is not
// enforced, which is acceptable at startup.
func ensureIndex(ctx context.Context, coll *mongo.Collection, idx Index) error {
	indexes := coll.Indexes()
	_, err := indexes.CreateOne(ctx, idx.model())
	if !isIndexConflict(err) {
		return err
	}

	if err := indexes.DropOne(ctx, idx.Name); err != nil {
		return fmt.Errorf("drop outdated index: %w", err)
	}
	_, err = indexes.CreateOne(ctx, idx.model())
	return err
}

func isIndexConflict(err error) bool {
	var se mongo.ServerError
	return errors.As(err, &se) && (se.HasErrorCode(indexOptionsConflict) || se.HasErrorCode(indexKeySpecsConflict))
}
