package integration

import (
	"testing"

	lucene "github.com/grindlemire/go-lucene"
)

// TestMySQLIntegrationNumbersAsWritten checks that a number a float64 can't
// hold compares as written in both render modes. Both used to send the
// nearest float64, so big:12345678901234567891 matched no row on MySQL and
// both rows on MariaDB. A nonzero number too small for a float64 used to
// render as 0, which MySQL compares equal to any text that doesn't start with
// a number.
func TestMySQLIntegrationNumbersAsWritten(t *testing.T) {
	requireIntegration(t)
	db := openTestDB(t)

	const columns = `
		ALTER TABLE items ADD COLUMN big DECIMAL(30,0), ADD COLUMN b BIGINT, ADD COLUMN code VARCHAR(16), ADD COLUMN nums JSON;
		UPDATE items SET big = 12345678901234567890, b = 9223372036854775807, code = '1e-400', nums = '{"n": 12345678901234567890}' WHERE id = 1;
		UPDATE items SET big = 12345678901234567891, b = 0, code = '0', nums = '{"n": 12345678901234567891}' WHERE id = 2;
	`
	if _, err := db.Exec(columns); err != nil {
		t.Fatalf("seed columns: %v", err)
	}

	cases := []integrationCase{
		{name: "equals", lucene: `big:12345678901234567891`, wantIDs: []int{2}},
		{name: "less", lucene: `big:<12345678901234567891`, wantIDs: []int{1}},
		{name: "range", lucene: `big:[12345678901234567891 TO *]`, wantIDs: []int{2}},
		{name: "range_with_param", lucene: `big:[1 TO 12345678901234567890]`, wantIDs: []int{1}},
		{name: "in_list", lucene: `big:(12345678901234567891 OR 7)`, wantIDs: []int{2}},
		{name: "past_bigint", lucene: `b:9223372036854775808`, wantIDs: nil},
		{name: "less_than_past_bigint", lucene: `b:<9223372036854775808`, wantIDs: []int{1, 2}},
		{name: "json_equals", lucene: `nums.n:12345678901234567891`, wantIDs: []int{2}},
		{name: "json_greater", lucene: `nums.n:>12345678901234567890`, wantIDs: []int{2}},
		{name: "underflow_text", lucene: `code:1e-400`, wantIDs: []int{1}},
		{name: "underflow_is_not_zero", lucene: `name:1e-400`, wantIDs: nil},
	}

	opt := lucene.WithJSONFields("nums")
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			where, err := lucene.ToMySQL(tc.lucene, opt)
			if err != nil {
				t.Fatalf("ToMySQL(%q): %v", tc.lucene, err)
			}
			if got := queryIDs(t, db, where); !equalInts(got, tc.wantIDs) {
				t.Fatalf("lucene=%q where=%q\n  want ids %v\n  got  ids %v", tc.lucene, where, tc.wantIDs, got)
			}
			where, params, err := lucene.ToParameterizedMySQL(tc.lucene, opt)
			if err != nil {
				t.Fatalf("ToParameterizedMySQL(%q): %v", tc.lucene, err)
			}
			if got := queryIDs(t, db, where, params...); !equalInts(got, tc.wantIDs) {
				t.Fatalf("lucene=%q where=%q params=%v\n  want ids %v\n  got  ids %v", tc.lucene, where, params, tc.wantIDs, got)
			}
		})
	}
}
