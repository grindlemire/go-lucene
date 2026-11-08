package lucene

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/grindlemire/go-lucene/pkg/driver"
	"github.com/grindlemire/go-lucene/pkg/lucene/expr"
)

// TestHandBuiltFieldGroupRendersLikeParsed checks that an EQUALS on a group,
// built in code or read from JSON, renders the same SQL as the query the
// parser reads into the field-spread form. expr.Eq("a", expr.AND("x", "y"))
// used to render as "a" = ('x' AND 'y').
func TestHandBuiltFieldGroupRendersLikeParsed(t *testing.T) {
	labels := expr.Lit(expr.JSONPath{Column: "labels", Path: []string{"k"}})
	fieldless := &expr.Expression{Op: expr.Range, Right: &expr.RangeBoundary{Min: expr.Lit(1), Max: expr.Lit(5), Inclusive: true}}

	tcs := map[string]struct {
		query string
		built *expr.Expression
		json  string
	}{
		"and": {
			query: `a:(x AND y)`,
			built: expr.Eq("a", expr.AND("x", "y")),
			json:  `{"left":"a","operator":"EQUALS","right":{"left":"x","operator":"AND","right":"y"}}`,
		},
		"or": {
			query: `a:(x OR y)`,
			built: expr.Eq("a", expr.OR("x", "y")),
			json:  `{"left":"a","operator":"EQUALS","right":{"left":"x","operator":"OR","right":"y"}}`,
		},
		"or_null": {
			query: `a:(x OR null)`,
			built: expr.Eq("a", expr.OR("x", expr.NULL())),
		},
		"must_not": {
			query: `a:(x -y)`,
			built: expr.Eq("a", expr.AND("x", expr.MUSTNOT(expr.Lit("y")))),
			json:  `{"left":"a","operator":"EQUALS","right":{"left":"x","operator":"AND","right":{"left":"y","operator":"MUST_NOT"}}}`,
		},
		"wildcard": {
			query: `a:(x OR foo*)`,
			built: expr.Eq("a", expr.OR("x", expr.WILD("foo*"))),
		},
		"range": {
			query: `a:(x OR [1 TO 5])`,
			built: expr.Eq("a", expr.OR("x", fieldless)),
		},
		"own_field": {
			query: `a:(b:c OR d)`,
			built: expr.Eq("a", expr.OR(expr.Eq("b", "c"), "d")),
		},
		"own_field_value": {
			query: `a:(b:1)`,
			built: expr.Eq("a", expr.Eq("b", 1)),
			json:  `{"left":"a","operator":"EQUALS","right":{"left":"b","operator":"EQUALS","right":1}}`,
		},
		"own_field_range": {
			query: `a:(b:[1 TO 5])`,
			built: expr.Eq("a", expr.Rang("b", 1, 5, true)),
		},
		"own_field_compare": {
			query: `a:(b:>1)`,
			built: expr.Eq("a", expr.GREATER("b", 1)),
		},
		"json_path": {
			query: `labels.k:(x AND 5)`,
			built: &expr.Expression{Op: expr.Equals, Left: labels, Right: expr.AND("x", 5)},
			json:  `{"left":{"json_path":{"column":"labels","path":["k"]}},"operator":"EQUALS","right":{"left":"x","operator":"AND","right":5}}`,
		},
	}

	drivers := map[string]interface {
		Render(*expr.Expression) (string, error)
		RenderParam(*expr.Expression) (string, []any, error)
	}{
		"postgres": driver.NewPostgresDriver(),
		"mysql":    driver.NewMySQLDriver(),
		"sqlite":   driver.NewSQLiteDriver(),
	}

	for name, tc := range tcs {
		parsed, err := Parse(tc.query, WithJSONFields("labels"))
		if err != nil {
			t.Fatalf("%s: parse %q: %v", name, tc.query, err)
		}
		inputs := map[string]*expr.Expression{"built": tc.built}
		if tc.json != "" {
			var decoded expr.Expression
			if err := json.Unmarshal([]byte(tc.json), &decoded); err != nil {
				t.Fatalf("%s: unmarshal: %v", name, err)
			}
			inputs["json"] = &decoded
		}
		for dname, d := range drivers {
			wantSQL, err := d.Render(parsed)
			if err != nil {
				t.Fatalf("%s/%s: render parsed: %v", name, dname, err)
			}
			wantParam, wantParams, err := d.RenderParam(parsed)
			if err != nil {
				t.Fatalf("%s/%s: render parsed with params: %v", name, dname, err)
			}
			for iname, in := range inputs {
				t.Run(name+"/"+dname+"/"+iname, func(t *testing.T) {
					got, err := d.Render(in)
					if err != nil {
						t.Fatalf("Render: unexpected error: %v", err)
					}
					if got != wantSQL {
						t.Fatalf("Render\nwant %s\ngot  %s", wantSQL, got)
					}
					got, params, err := d.RenderParam(in)
					if err != nil {
						t.Fatalf("RenderParam: unexpected error: %v", err)
					}
					if got != wantParam || !reflect.DeepEqual(params, wantParams) {
						t.Fatalf("RenderParam\nwant %s %#v\ngot  %s %#v", wantParam, wantParams, got, params)
					}
				})
			}
		}
	}
}
