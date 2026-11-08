package integration

import (
	"testing"

	lucene "github.com/grindlemire/go-lucene"
)

// TestSQLiteIntegrationNumbersAsWritten checks numbers a float64 can't hold
// in both render modes. SQLite reads an integer past int64 as a REAL, so
// those compare as the nearest double, as SQL written by hand would. A
// nonzero number too small for a float64 used to render as 0, which matched
// the text '0'.
func TestSQLiteIntegrationNumbersAsWritten(t *testing.T) {
	requireIntegration(t)
	db := openTestDB(t)

	const columns = `
        ALTER TABLE items ADD COLUMN big INTEGER;
        ALTER TABLE items ADD COLUMN code TEXT;
        UPDATE items SET big = 9223372036854775807, code = '1e-400' WHERE id = 1;
        UPDATE items SET big = -9223372036854775808, code = '0' WHERE id = 2;
    `
	if _, err := db.Exec(columns); err != nil {
		t.Fatalf("seed columns: %v", err)
	}

	cases := []struct {
		name    string
		lucene  string
		wantIDs []int
	}{
		{"past_int64", `big:9223372036854775808`, nil},
		{"less_than_past_int64", `big:<9223372036854775808`, []int{1, 2}},
		{"range", `big:[-9223372036854775809 TO 9223372036854775808]`, []int{1, 2}},
		{"in_list", `big:(9223372036854775808 OR 9223372036854775807)`, []int{1}},
		{"underflow_text", `code:1e-400`, []int{1}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			where, err := lucene.ToSQLite(tc.lucene)
			if err != nil {
				t.Fatalf("ToSQLite(%q): %v", tc.lucene, err)
			}
			if got := queryIDs(t, db, where); !equalInts(got, tc.wantIDs) {
				t.Fatalf("lucene=%q where=%q\n  want ids %v\n  got  ids %v", tc.lucene, where, tc.wantIDs, got)
			}
			where, params, err := lucene.ToParameterizedSQLite(tc.lucene)
			if err != nil {
				t.Fatalf("ToParameterizedSQLite(%q): %v", tc.lucene, err)
			}
			if got := queryIDs(t, db, where, params...); !equalInts(got, tc.wantIDs) {
				t.Fatalf("lucene=%q where=%q params=%v\n  want ids %v\n  got  ids %v", tc.lucene, where, params, tc.wantIDs, got)
			}
		})
	}
}
