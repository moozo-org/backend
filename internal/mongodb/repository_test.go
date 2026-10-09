package mongodb

import (
	"testing"

	"moozo/internal/mongodb/mongotest"
)

// newRepository returns a Repository on a fresh, schema-initialized database
// dropped when the test ends.
func newRepository(t *testing.T) *Repository {
	t.Helper()
	repo, err := NewRepository(t.Context(), mongotest.NewDatabase(t))
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}
	return repo
}
