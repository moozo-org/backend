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

type usersJSONSchema struct {
	Required   []string `bson:"required"`
	Properties map[string]struct {
		Enum []string `bson:"enum"`
	} `bson:"properties"`
}

func loadUsersSchema(t *testing.T) (schemas.Collection, usersJSONSchema) {
	t.Helper()

	colls, err := schemas.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	i := slices.IndexFunc(colls, func(c schemas.Collection) bool { return c.Name == UsersCollectionName })
	if i < 0 {
		t.Fatalf("no schema file for collection %q", UsersCollectionName)
	}

	raw, err := bson.Marshal(colls[i].Validator)
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Schema usersJSONSchema `bson:"$jsonSchema"`
	}
	if err := bson.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	return colls[i], v.Schema
}

// schemas/users.json is hand-written; this keeps it in step with userDocument
// and the domain.
func TestUsersSchema(t *testing.T) {
	coll, schema := loadUsersSchema(t)

	var fields []string
	for f := range reflect.TypeFor[userDocument]().Fields() {
		fields = append(fields, strings.Split(f.Tag.Get("bson"), ",")[0])
	}
	var roles []string
	for _, r := range moozo.Roles {
		roles = append(roles, string(r))
	}

	cases := map[string]struct {
		got, want []string
	}{
		"properties match userDocument": {got: slices.Collect(maps.Keys(schema.Properties)), want: fields},
		"required match userDocument":   {got: schema.Required, want: fields},
		"role enum matches domain":      {got: schema.Properties["role"].Enum, want: roles},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := slices.Sorted(slices.Values(tc.got))
			want := slices.Sorted(slices.Values(tc.want))
			if !slices.Equal(got, want) {
				t.Errorf("schema = %v, want %v", got, want)
			}
		})
	}

	// Register relies on this index to report moozo.ErrEmailTaken, whatever
	// the email's case.
	t.Run("case-insensitive unique email index", func(t *testing.T) {
		if !slices.ContainsFunc(coll.Indexes, func(idx schemas.Index) bool {
			return idx.Unique && len(idx.Key) == 1 && idx.Key[0].Key == "email" &&
				idx.Collation != nil && idx.Collation.Strength <= 2
		}) {
			t.Errorf("indexes = %+v, want a unique email index with collation strength <= 2", coll.Indexes)
		}
	})
}
