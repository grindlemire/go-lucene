package driver

import (
	"testing"

	"github.com/grindlemire/go-lucene/pkg/lucene/expr"
)

func TestSQLiteDriverBasic(t *testing.T) {
	got, err := NewSQLiteDriver().Render(expr.Eq("a", 5))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `"a" = 5`
	if got != want {
		t.Fatalf("wanted %s got %s", want, got)
	}
}
