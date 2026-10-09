package mongodb

import (
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"moozo/internal/mongodb/schemas"
	"moozo/moozo"
)

// The schema files are hand-written; these tests keep them in step with the
// document types and the domain.

type jsonSchema struct {
	Required   []string `bson:"required"`
	Properties map[string]struct {
		Enum []string `bson:"enum"`
	} `bson:"properties"`
}

func loadSchema(t *testing.T, collection string) (schemas.Collection, jsonSchema) {
	t.Helper()

	colls, err := schemas.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	i := slices.IndexFunc(colls, func(c schemas.Collection) bool { return c.Name == collection })
	if i < 0 {
		t.Fatalf("no schema file for collection %q", collection)
	}

	raw, err := bson.Marshal(colls[i].Validator)
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Schema jsonSchema `bson:"$jsonSchema"`
	}
	if err := bson.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	return colls[i], v.Schema
}

func bsonFields(typ reflect.Type) []string {
	var fields []string
	for f := range typ.Fields() {
		fields = append(fields, strings.Split(f.Tag.Get("bson"), ",")[0])
	}
	return fields
}

func TestSchemasMatchDocuments(t *testing.T) {
	cases := map[string]reflect.Type{
		UsersCollectionName:    reflect.TypeFor[userDocument](),
		SessionsCollectionName: reflect.TypeFor[sessionDocument](),
	}

	for collection, doc := range cases {
		t.Run(collection, func(t *testing.T) {
			_, schema := loadSchema(t, collection)
			fields := slices.Sorted(slices.Values(bsonFields(doc)))

			if props := slices.Sorted(maps.Keys(schema.Properties)); !slices.Equal(props, fields) {
				t.Errorf("properties = %v, %s fields = %v", props, doc.Name(), fields)
			}
			if required := slices.Sorted(slices.Values(schema.Required)); !slices.Equal(required, fields) {
				t.Errorf("required = %v, %s fields = %v", required, doc.Name(), fields)
			}
		})
	}
}

// Users and sessions both store a role; each schema's enum is a copy of
// moozo.Roles.
func TestSchemaRolesMatchDomain(t *testing.T) {
	var roles []string
	for _, r := range moozo.Roles {
		roles = append(roles, string(r))
	}
	slices.Sort(roles)

	for _, collection := range []string{UsersCollectionName, SessionsCollectionName} {
		t.Run(collection, func(t *testing.T) {
			_, schema := loadSchema(t, collection)
			if got := slices.Sorted(slices.Values(schema.Properties["role"].Enum)); !slices.Equal(got, roles) {
				t.Errorf("role enum = %v, moozo.Roles = %v", got, roles)
			}
		})
	}
}

func TestUsersSchema(t *testing.T) {
	coll, _ := loadSchema(t, UsersCollectionName)

	// Register relies on this index to report moozo.ErrEmailTaken whatever
	// the email's case, and FindUserByEmail queries with emailCollation.
	if !slices.ContainsFunc(coll.Indexes, func(idx schemas.Index) bool {
		return idx.Unique && len(idx.Key) == 1 && idx.Key[0].Key == "email" && idx.Collation != nil &&
			idx.Collation.Locale == emailCollation.Locale && idx.Collation.Strength == emailCollation.Strength
	}) {
		t.Errorf("indexes = %+v, want a unique email index with collation %+v", coll.Indexes, *emailCollation)
	}
}

// Expired sessions are deleted by MongoDB; FindSession's callers still check
// ExpiresAt because the TTL monitor runs only about once a minute.
func TestSessionsSchema(t *testing.T) {
	coll, _ := loadSchema(t, SessionsCollectionName)

	if !slices.ContainsFunc(coll.Indexes, func(idx schemas.Index) bool {
		return len(idx.Key) == 1 && idx.Key[0].Key == "expires_at" &&
			idx.ExpireAfterSeconds != nil && *idx.ExpireAfterSeconds == 0
	}) {
		t.Errorf("indexes = %+v, want a TTL index on expires_at with expireAfterSeconds 0", coll.Indexes)
	}
}
