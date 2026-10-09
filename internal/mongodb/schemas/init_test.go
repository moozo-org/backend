package schemas_test

import (
	"slices"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"moozo/internal/mongodb/mongotest"
	"moozo/internal/mongodb/schemas"
)

func load(t *testing.T) []schemas.Collection {
	t.Helper()
	colls, err := schemas.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(colls) == 0 {
		t.Fatal("Load() found no schema files")
	}
	return colls
}

func TestLoadParsesEveryFile(t *testing.T) {
	for _, c := range load(t) {
		if c.Name == "" {
			t.Error("collection with empty name")
		}
		if len(c.Validator) == 0 || c.Validator[0].Key != "$jsonSchema" {
			t.Errorf("%s: validator = %v, want a $jsonSchema", c.Name, c.Validator)
		}
		if c.ValidationLevel == "" || c.ValidationAction == "" {
			t.Errorf("%s: validation level/action defaults not applied", c.Name)
		}
		for _, idx := range c.Indexes {
			if idx.Name == "" || len(idx.Key) == 0 {
				t.Errorf("%s: index %+v needs a name and a key", c.Name, idx)
			}
		}
	}
}

// assertApplied checks that every loaded collection exists on db with its
// validator, validation settings and indexes.
func assertApplied(t *testing.T, db *mongo.Database) {
	t.Helper()

	for _, c := range load(t) {
		specs, err := db.ListCollectionSpecifications(t.Context(), bson.D{{Key: "name", Value: c.Name}})
		if err != nil || len(specs) != 1 {
			t.Fatalf("%s: collection specs = %v, %v", c.Name, specs, err)
		}
		opts := specs[0].Options
		if _, err := opts.LookupErr("validator", "$jsonSchema"); err != nil {
			t.Errorf("%s: no $jsonSchema validator in %s", c.Name, opts)
		}
		if got := opts.Lookup("validationLevel").StringValue(); got != c.ValidationLevel {
			t.Errorf("%s: validationLevel = %q, want %q", c.Name, got, c.ValidationLevel)
		}
		if got := opts.Lookup("validationAction").StringValue(); got != c.ValidationAction {
			t.Errorf("%s: validationAction = %q, want %q", c.Name, got, c.ValidationAction)
		}

		assertIndexes(t, db.Collection(c.Name), c.Indexes)
	}
}

// assertIndexes reads the raw index documents: IndexSpecification omits
// collation.
func assertIndexes(t *testing.T, coll *mongo.Collection, want []schemas.Index) {
	t.Helper()

	cur, err := coll.Indexes().List(t.Context())
	if err != nil {
		t.Fatalf("%s: list indexes: %v", coll.Name(), err)
	}
	var got []schemas.Index
	if err := cur.All(t.Context(), &got); err != nil {
		t.Fatalf("%s: decode indexes: %v", coll.Name(), err)
	}

	for _, w := range want {
		i := slices.IndexFunc(got, func(g schemas.Index) bool { return g.Name == w.Name })
		if i < 0 {
			t.Errorf("%s: index %q missing", coll.Name(), w.Name)
			continue
		}
		g := got[i]
		if g.Unique != w.Unique {
			t.Errorf("%s: index %q unique = %v, want %v", coll.Name(), w.Name, g.Unique, w.Unique)
		}
		if (w.Collation == nil) != (g.Collation == nil) ||
			w.Collation != nil && (g.Collation.Locale != w.Collation.Locale || g.Collation.Strength != w.Collation.Strength) {
			t.Errorf("%s: index %q collation = %+v, want %+v", coll.Name(), w.Name, g.Collation, w.Collation)
		}
		if !equalPtr(g.ExpireAfterSeconds, w.ExpireAfterSeconds) {
			t.Errorf("%s: index %q expireAfterSeconds = %v, want %v", coll.Name(), w.Name, g.ExpireAfterSeconds, w.ExpireAfterSeconds)
		}
	}
}

func TestInit(t *testing.T) {
	cases := map[string]struct {
		setup func(t *testing.T, db *mongo.Database) // state before Init
	}{
		"empty database": {},
		// A collection created before its schema existed gets the validator
		// through collMod.
		"collections without validator": {
			setup: func(t *testing.T, db *mongo.Database) {
				for _, c := range load(t) {
					if err := db.CreateCollection(t.Context(), c.Name); err != nil {
						t.Fatalf("create %s: %v", c.Name, err)
					}
				}
			},
		},
		// Changing an index in a file rebuilds it on the next Init.
		"indexes with outdated options": {
			setup: func(t *testing.T, db *mongo.Database) {
				for _, c := range load(t) {
					for _, idx := range c.Indexes {
						_, err := db.Collection(c.Name).Indexes().CreateOne(t.Context(), mongo.IndexModel{
							Keys:    idx.Key,
							Options: options.Index().SetName(idx.Name).SetUnique(!idx.Unique),
						})
						if err != nil {
							t.Fatalf("create outdated %s.%s: %v", c.Name, idx.Name, err)
						}
					}
				}
			},
		},
		"already initialized": {
			setup: func(t *testing.T, db *mongo.Database) {
				if err := schemas.Init(t.Context(), db); err != nil {
					t.Fatalf("first Init() error = %v", err)
				}
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			db := mongotest.NewDatabase(t)
			if tc.setup != nil {
				tc.setup(t, db)
			}

			if err := schemas.Init(t.Context(), db); err != nil {
				t.Fatalf("Init() error = %v", err)
			}
			assertApplied(t, db)
		})
	}
}

func equalPtr[T comparable](a, b *T) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}
