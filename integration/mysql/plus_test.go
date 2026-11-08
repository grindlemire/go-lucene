package integration

import (
	"testing"

	lucene "github.com/grindlemire/go-lucene"
)

// TestMySQLIntegrationPlusInsideTerm checks that a + inside a term, with or
// without wildcards, matches a literal +, in both render modes. An unescaped
// + in a wildcard used to send the pattern to REGEXP, where it means "one or
// more".
func TestMySQLIntegrationPlusInsideTerm(t *testing.T) {
	requireIntegration(t)
	db := openTestDB(t)

	const column = `
		ALTER TABLE items ADD COLUMN lang VARCHAR(16);
		UPDATE items SET lang = 'c++' WHERE id = 1;
		UPDATE items SET lang = 'c' WHERE id = 2;
		UPDATE items SET lang = 'cc' WHERE id = 3;
		UPDATE items SET lang = 'c+x' WHERE id = 4;
	`
	if _, err := db.Exec(column); err != nil {
		t.Fatalf("seed column: %v", err)
	}

	cases := []integrationCase{
		{name: "term", lucene: `lang:c++`, wantIDs: []int{1}},
		{name: "star_after_plus", lucene: `lang:c++*`, wantIDs: []int{1}},
		{name: "star_after_one_plus", lucene: `lang:c+*`, wantIDs: []int{1, 4}},
		{name: "question_after_plus", lucene: `lang:c+?`, wantIDs: []int{1, 4}},
		{name: "plus_between_stars", lucene: `lang:*+*`, wantIDs: []int{1, 4}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			where, err := lucene.ToMySQL(tc.lucene)
			if err != nil {
				t.Fatalf("ToMySQL(%q): %v", tc.lucene, err)
			}
			if got := queryIDs(t, db, where); !equalInts(got, tc.wantIDs) {
				t.Fatalf("lucene=%q where=%q\n  want ids %v\n  got  ids %v", tc.lucene, where, tc.wantIDs, got)
			}
			where, params, err := lucene.ToParameterizedMySQL(tc.lucene)
			if err != nil {
				t.Fatalf("ToParameterizedMySQL(%q): %v", tc.lucene, err)
			}
			if got := queryIDs(t, db, where, params...); !equalInts(got, tc.wantIDs) {
				t.Fatalf("lucene=%q where=%q params=%v\n  want ids %v\n  got  ids %v", tc.lucene, where, params, tc.wantIDs, got)
			}
		})
	}
}
