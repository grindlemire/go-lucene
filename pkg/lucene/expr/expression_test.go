package expr

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

const (
	errTemplate     = "%s:\n    wanted %#v\n    got    %#v"
	jsonErrTemplate = "%s:\n    wanted %s\n    got    %s"
)

func TestExprJSON(t *testing.T) {
	type tc struct {
		input string
		want  *Expression
	}

	tcs := map[string]tc{
		"flat_literal": {
			input: `"a"`,
			want:  Lit("a"),
		},
		"flat_wildcard": {
			input: `"a*"`,
			want:  WILD("a*"),
		},
		"flat_equals": {
			input: `{"left": "a", "operator": "EQUALS", "right": "b"}`,
			want:  Eq(Lit("a"), Lit("b")),
		},
		"flat_regexp": {
			input: `{
				"left": "a",
				"operator": "LIKE",
				"right": "/b [c]/"
			  }`,
			want: LIKE(Lit("a"), REGEXP("/b [c]/")),
		},
		"flat_inclusive_range": {
			input: `{
				"left": "a",
				"operator": "RANGE",
				"right": {
					"min": 1,
					"max": 2,
					"inclusive": true
				}
			  }`,
			want: Rang("a", 1, 2, true),
		},
		"flat_exclusive_range": {
			input: `{
				"left": "a",
				"operator": "RANGE",
				"right": {
					"min": 1,
					"max": 2,
					"inclusive": false
				}
			  }`,
			want: Rang("a", 1, 2, false),
		},
		"flat_range_with_float": {
			input: `{
				"left": "a",
				"operator": "RANGE",
				"right": {
					"min": 1.1,
					"max": 2.2,
					"inclusive": true
				}
			  }`,
			want: Rang("a", 1.1, 2.2, true),
		},
		"must_wrapping_range": {
			input: `{
				"left": {
					"left": "c",
					"operator": "RANGE",
					"right": {
						"min": "*",
						"max": "foo",
						"inclusive": false
					}
				},
				"operator": "MUST"
			}`,
			want: MUST(Rang("c", "*", "foo", false)),
		},
		"flat_must": {
			input: `{
				"left": "a",
				"operator": "MUST"
			}`,
			want: MUST(Lit("a")),
		},
		"flat_must_not": {
			input: `{
				"left": "a",
				"operator": "MUST_NOT"
			}`,
			want: MUSTNOT(Lit("a")),
		},
		"flat_not": {
			input: `{
				"left": "a",
				"operator": "NOT"
			}`,
			want: NOT(Lit("a")),
		},
		"flat_boost": {
			input: `{
				"left": "a",
				"operator": "BOOST"
			}`,
			want: BOOST(Lit("a")),
		},
		"flat_boost_explicit_power": {
			input: `{
				"left": "a",
				"operator": "BOOST",
				"power": 0.8
			}`,
			want: BOOST(Lit("a"), 0.8),
		},
		"flat_fuzzy": {
			input: `{
				"left": "a",
				"operator": "FUZZY"
			}`,
			want: FUZZY(Lit("a")),
		},
		"flat_fuzzy_explicit_power": {
			input: `{
				"left": "a",
				"operator": "FUZZY",
				"distance": 2
			}`,
			want: FUZZY("a", 2),
		},
		"flat_in_list": {
			input: `{
				"left": "a",
				"operator": "IN",
				"right": {
					"left": ["b", "c"],
					"operator": "LIST"
				}	
			}`,
			want: IN("a", LIST(Lit("b"), Lit("c"))),
		},
		"basic_and": {
			input: `{
				"left": {
					"left": "a",
					"operator": "EQUALS",
					"right": "b"
				},
				"operator": "AND",
				"right": {
					"left": "c",
					"operator": "EQUALS",
					"right": "d"
				}
			}`,
			want: AND(
				Eq("a", "b"),
				Eq("c", "d"),
			),
		},
		"basic_or": {
			input: `{
				"left": {
					"left": "a",
					"operator": "EQUALS",
					"right": "b"
				},
				"operator": "OR",
				"right": {
					"left": "c",
					"operator": "EQUALS",
					"right": "d"
				}
			}`,
			want: OR(
				Eq("a", "b"),
				Eq("c", "d"),
			),
		},
		"preserves_precedence": {
			input: `{
				"left": {
					"left": {
						"left": "a",
						"operator": "AND",
						"right": "b"
					},
					"operator": "OR",
					"right": {
						"left": "c",
						"operator": "AND",
						"right": "d"
					}
				},
				"operator": "OR",
				"right": "e"
			}`,
			want: OR(
				OR(
					AND("a", "b"),
					AND("c", "d"),
				),
				"e",
			),
		},
		"compound_using_range": {
			input: `{
				"left": {
					"left": {
						"left": "c",
						"operator": "RANGE",
						"right": {
							"min": "*",
							"max": "foo",
							"inclusive": false
						}
					},
					"operator": "MUST"
				},
				"operator": "OR",
				"right": {
					"left": {
						"left": {
							"left": "d",
							"operator": "EQUALS",
							"right": {
								"left": "bar",
								"operator": "FUZZY",
								"distance": 3
							}
						},
						"operator": "NOT"
					},
					"operator": "MUST_NOT"
				}
			}`,
			want: OR(
				MUST(Rang("c", "*", "foo", false)),
				MUSTNOT(NOT(Eq("d", FUZZY("bar", 3)))),
			),
		},
		"large_blob": {
			input: `{
				"left": "a",
				"operator": "OR",
				"right": {
					"left": {
						"left": "c",
						"operator": "RANGE",
						"right": {
							"min": "*",
							"max": "foo",
							"inclusive": false
						}
					},
					"operator": "OR",
					"right": "b"
				}
			}`,
			want: OR(
				"a",
				OR(
					Rang("c", "*", "foo", false),
					"b",
				),
			),
		},
		"every_operator_combined": {
			input: `{
				"left": {
					"left": {
						"left": "a",
						"operator": "RANGE",
						"right": {
							"min": 1,
							"max": "*",
							"inclusive": true
						}
					},
					"operator": "AND",
					"right": {
						"left": {
							"left": {
								"left": "b",
								"operator": "LIKE",
								"right": "/foo?ar.*/"
							},
							"operator": "NOT"
						},
						"operator": "BOOST"
					}
				},
				"operator": "OR",
				"right": {
					"left": {
						"left": {
							"left": "c",
							"operator": "RANGE",
							"right": {
								"min": "*",
								"max": "foo",
								"inclusive": false
							}
						},
						"operator": "MUST"
					},
					"operator": "OR",
					"right": {
						"left": {
							"left": {
								"left": "d",
								"operator": "EQUALS",
								"right": {
									"left": "bar",
									"operator": "FUZZY",
									"distance": 3
								}
							},
							"operator": "NOT"
						},
						"operator": "MUST_NOT"
					}
				}
			}`,
			want: OR(
				AND(
					Rang("a", 1, "*", true),
					BOOST(NOT(LIKE("b", REGEXP("/foo?ar.*/")))),
				),
				OR(
					MUST(Rang("c", "*", "foo", false)),
					MUSTNOT(NOT(Eq("d", FUZZY("bar", 3)))),
				),
			),
		},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			got := &Expression{}
			err := json.Unmarshal([]byte(tc.input), got)
			if err != nil {
				t.Fatalf("expected no error during unmarshal but got [%s]", err)
			}

			if !reflect.DeepEqual(tc.want, got) {
				t.Fatalf(errTemplate, "parsed expression doesn't match", tc.want, got)
			}

			gotSerialized, err := json.Marshal(got)
			if err != nil {
				t.Fatalf("expected no error during marshal but got [%s]", err)
			}

			if !jsonEqual(string(gotSerialized), tc.input) {
				t.Fatalf(
					jsonErrTemplate,
					"serialized expressions don't match",
					stripWhitespace(tc.input),
					stripWhitespace(string(gotSerialized)),
				)
			}
		})
	}
}

func TestNullOperatorPlumbing(t *testing.T) {
	n := NULL()
	if n.Op != Null {
		t.Fatalf("NULL() should have Op == Null, got %v", n.Op)
	}
	if n.Left != nil {
		t.Fatalf("NULL() should have nil Left, got %v", n.Left)
	}
	if n.Right != nil {
		t.Fatalf("NULL() should have nil Right, got %v", n.Right)
	}
	if got := n.String(); got != "null" {
		t.Fatalf("NULL().String() = %q, want %q", got, "null")
	}
	if toString[Null] != "NULL" {
		t.Fatalf("toString[Null] = %q, want %q", toString[Null], "NULL")
	}
	if fromString["NULL"] != Null {
		t.Fatalf("fromString[\"NULL\"] = %v, want Null", fromString["NULL"])
	}
}

// TestRenderLiteralQuoteEscaping pins that only a plain Literal escapes an
// embedded quote when rendered back to Lucene syntax (issue #59); Wild and
// Regexp keep their pre-existing, unescaped, space-only quoting since their
// raw text isn't phrase-delimiter-escaped.
func TestRenderLiteralQuoteEscaping(t *testing.T) {
	if got, want := Lit(`foo"bar`).String(), `"foo\"bar"`; got != want {
		t.Fatalf("Lit(%q).String() = %q, want %q", `foo"bar`, got, want)
	}
	if got, want := Lit(`it's`).String(), `"it's"`; got != want {
		t.Fatalf("Lit(%q).String() = %q, want %q", `it's`, got, want)
	}
	// a space alone triggers the same quote+escape branch as an embedded
	// quote, so a literal with both a space and a trailing backslash must
	// still have the backslash escaped - otherwise it renders as `"foo bar\"`,
	// which the lexer reads as an escaped closing quote and fails to re-parse.
	if got, want := Lit(`foo bar\`).String(), `"foo bar\\"`; got != want {
		t.Fatalf("Lit(%q).String() = %q, want %q", `foo bar\`, got, want)
	}
	// A wildcard is not quoted, since a phrase cannot hold a wildcard. The
	// quote is escaped instead, so the lexer keeps it in the word.
	if got, want := WILD(`fo"o*`).String(), `fo\"o*`; got != want {
		t.Fatalf("WILD(%q).String() = %q, want %q", `fo"o*`, got, want)
	}
	if got, want := REGEXP(`/fo"o/`).String(), `/fo"o/`; got != want {
		t.Fatalf("REGEXP(%q).String() = %q, want %q", `/fo"o/`, got, want)
	}
}

// TestRenderRangeWithoutField checks that a range the parser has not given a
// field yet prints as a bare range. It shows up in parse error messages.
func TestRenderRangeWithoutField(t *testing.T) {
	if got, want := Rang(nil, 1, 5, true).String(), `[1 TO 5]`; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
	if got, want := Rang(nil, 1, 5, false).String(), `{1 TO 5}`; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
	if got, want := Rang("a", 1, 5, true).String(), `a:[1 TO 5]`; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
	if got, want := fmt.Sprintf("%#v", Rang(nil, 1, 5, true)), `[LITERAL(1) TO LITERAL(5)]`; got != want {
		t.Fatalf("GoString() = %q, want %q", got, want)
	}
}

func TestNullValidation(t *testing.T) {
	if err := Validate(NULL()); err != nil {
		t.Fatalf("Validate(NULL()) returned err: %v", err)
	}

	if err := Validate(Eq("field", NULL())); err != nil {
		t.Fatalf("Validate(Eq(field, NULL())) returned err: %v", err)
	}

	list := LIST(Lit("a"), NULL())
	if err := Validate(list); err != nil {
		t.Fatalf("Validate(LIST containing NULL) returned err: %v", err)
	}

	bad := &Expression{Op: Null, Left: "x"}
	if err := Validate(bad); err == nil {
		t.Fatal("Validate(Null with Left) should have errored")
	}
}

func TestNullJSONRoundTrip(t *testing.T) {
	cases := []*Expression{
		NULL(),
		Eq("field", NULL()),
		AND(Eq("field", NULL()), Eq("other", 5)),
	}

	for _, original := range cases {
		raw, err := json.Marshal(original)
		if err != nil {
			t.Fatalf("marshal failed: %v", err)
		}

		var round Expression
		if err := json.Unmarshal(raw, &round); err != nil {
			t.Fatalf("unmarshal failed: %v (json was: %s)", err, raw)
		}

		if !reflect.DeepEqual(original, &round) {
			t.Fatalf("round-trip mismatch:\n  want %#v\n  got  %#v\n  json: %s",
				original, &round, raw)
		}
	}
}

// TestValidateRangeBounds checks that Validate only accepts a single term
// (a scalar, or a literal, wildcard, regexp, or null expression) on each side
// of a range. Compound bounds used to render partially into SQL (issue #68).
func TestValidateRangeBounds(t *testing.T) {
	rangeOf := func(min, max any) *Expression {
		return &Expression{
			Left:  Lit(Column("a")),
			Op:    Range,
			Right: &RangeBoundary{Min: min, Max: max, Inclusive: true},
		}
	}

	valid := map[string]*Expression{
		"helper_ints":    Rang("a", 1, 2, true),
		"helper_strings": Rang("a", "x", "y", false),
		"helper_wild":    Rang("a", "*", "y", true),
		"raw_scalars":    rangeOf(1, "z"),
		"raw_bool":       rangeOf(true, false),
		"regexp_bounds":  rangeOf(REGEXP("/x/"), REGEXP("/y/")),
		"null_bound":     rangeOf(NULL(), Lit(3)),
		"time_bounds":    Rang("a", time.Unix(0, 0), time.Unix(60, 0), true),
		"int8_bounds":    Rang("a", int8(1), int8(2), true),
	}
	for name, e := range valid {
		t.Run("valid/"+name, func(t *testing.T) {
			if err := Validate(e); err != nil {
				t.Fatalf("expected %#v to validate, got: %v", e, err)
			}
		})
	}

	invalid := map[string]*Expression{
		"and_min":        rangeOf(AND(Lit(1), Lit(2)), Lit(3)),
		"or_max":         rangeOf(Lit(1), OR(Lit(3), Lit(4))),
		"not_min":        rangeOf(NOT(Lit(1)), Lit(3)),
		"must_min":       rangeOf(MUST(Lit(1)), Lit(3)),
		"must_not_min":   rangeOf(MUSTNOT(Lit(1)), Lit(3)),
		"equals_min":     rangeOf(Eq("b", "c"), Lit(3)),
		"fuzzy_max":      rangeOf(Lit(1), FUZZY(Lit(3))),
		"boost_max":      rangeOf(Lit(1), BOOST(Lit(3), 2.0)),
		"range_max":      rangeOf(Lit(1), Rang("b", 1, 2, true)),
		"list_max":       rangeOf(Lit(1), LIST(Lit(3), Lit(4))),
		"nested_literal": rangeOf(Lit(Lit("x")), Lit(3)),
		"map_literal":    rangeOf(Lit(map[string]any{"x": "1"}), Lit(3)),
		"nil_literal":    rangeOf(&Expression{Op: Literal}, Lit(3)),
		"raw_map":        rangeOf(map[string]any{"x": "1"}, 3),
		"raw_slice":      rangeOf([]any{"x"}, 3),
		"ast_slice":      rangeOf(Lit([]*Expression{Lit("x")}), 5),
		"ast_value_list": rangeOf(Lit([]Expression{*Lit("x")}), 5),
		"typed_nil_min":  rangeOf((*Expression)(nil), Lit(3)),
		"typed_nil_max":  rangeOf(Lit(1), (*Expression)(nil)),
	}
	for name, e := range invalid {
		t.Run("invalid/"+name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("Validate panicked: %v", r)
				}
			}()
			err := Validate(e)
			if err == nil {
				t.Fatalf("expected %#v to fail validation", e)
			}
			if !strings.Contains(err.Error(), "must be a single term") {
				t.Fatalf("expected a single-term range bound error, got: %v", err)
			}
		})
	}

	// Bounds decoded from JSON skip the parser, so they are only validated if
	// the caller runs Validate. Objects and arrays decode to map/slice values.
	jsonInvalid := map[string]string{
		"object_min": `{"left":"a","operator":"RANGE","right":{"min":{"x":"1') OR 1=1 --"},"max":2,"inclusive":true}}`,
		"array_max":  `{"left":"a","operator":"RANGE","right":{"min":1,"max":["1') OR 1=1 --"],"inclusive":true}}`,
		"null_min":   `{"left":"a","operator":"RANGE","right":{"min":null,"max":2,"inclusive":true}}`,
	}
	for name, raw := range jsonInvalid {
		t.Run("json/"+name, func(t *testing.T) {
			var e Expression
			if err := json.Unmarshal([]byte(raw), &e); err != nil {
				t.Fatalf("unmarshal failed: %v", err)
			}
			err := Validate(&e)
			if err == nil {
				t.Fatalf("expected %#v to fail validation", &e)
			}
			if !strings.Contains(err.Error(), "must be a single term") {
				t.Fatalf("expected a single-term range bound error, got: %v", err)
			}
		})
	}
}

// TestRangeBoundErrorMessage pins the compound-bound error. The quoting hint
// only appears for AND, OR and EQUALS, the shapes an unquoted space or colon
// in a bound produces.
func TestRangeBoundErrorMessage(t *testing.T) {
	const prefix = "RANGE validation: range minimum must be a single term, not "
	const hint = "; quote a bound that contains spaces or ':'"

	tcs := map[string]struct {
		bound any
		want  string
	}{
		"and":      {AND(Lit(1), Lit(2)), prefix + "an AND expression" + hint},
		"or":       {OR(Lit(1), Lit(2)), prefix + "an OR expression" + hint},
		"equals":   {Eq("b", "c"), prefix + "an EQUALS expression" + hint},
		"not":      {NOT(Lit(1)), prefix + "a NOT expression"},
		"must":     {MUST(Lit(1)), prefix + "a MUST expression"},
		"must_not": {MUSTNOT(Lit(1)), prefix + "a MUST_NOT expression"},
		"boost":    {BOOST(Lit(1), 2.0), prefix + "a BOOST expression"},
		"fuzzy":    {FUZZY(Lit(1)), prefix + "a FUZZY expression"},
		"range":    {Rang("b", 1, 2, true), prefix + "a RANGE expression"},
		"in":       {IN("b", LIST(Lit(1))), prefix + "an IN expression"},
		"list":     {LIST(Lit(1)), prefix + "a LIST expression"},
	}
	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			e := &Expression{
				Left:  Lit(Column("a")),
				Op:    Range,
				Right: &RangeBoundary{Min: tc.bound, Max: Lit(3), Inclusive: true},
			}
			err := Validate(e)
			if err == nil {
				t.Fatalf("expected %#v to fail validation", e)
			}
			if err.Error() != tc.want {
				t.Fatalf("\nwant %s\ngot  %s", tc.want, err.Error())
			}
		})
	}
}

// TestLiteralJSONForm checks the JSON form of a string literal. A literal
// that a plain JSON string would decode as a wildcard or a regex is written
// as {"literal": ...}, so it decodes back as a literal.
func TestLiteralJSONForm(t *testing.T) {
	tcs := map[string]struct {
		input *Expression
		json  string
	}{
		"plain":       {Lit("foo"), `"foo"`},
		"star":        {Lit("fo*"), `{"literal":"fo*"}`},
		"question":    {Lit("f?o"), `{"literal":"f?o"}`},
		"slashes":     {Lit("/re/"), `{"literal":"/re/"}`},
		"wildcard":    {WILD("fo*"), `"fo*"`},
		"in_equals":   {Eq("a", Lit("fo*")), `{"left":"a","operator":"EQUALS","right":{"literal":"fo*"}}`},
		"in_list":     {IN("a", LIST(Lit("fo*"), Lit("b"))), `{"left":"a","operator":"IN","right":{"left":[{"literal":"fo*"},"b"],"operator":"LIST"}}`},
		"range_bound": {Rang("a", Lit("fo*"), Lit("z"), true), `{"left":"a","operator":"RANGE","right":{"min":{"literal":"fo*"},"max":"z","inclusive":true}}`},
	}
	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			raw, err := json.Marshal(tc.input)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if string(raw) != tc.json {
				t.Fatalf(jsonErrTemplate, "json does not match", tc.json, raw)
			}
			var got Expression
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatalf("unmarshal %s: %v", raw, err)
			}
			if !reflect.DeepEqual(tc.input, &got) {
				t.Fatalf(errTemplate, "round trip changed the expression", tc.input, &got)
			}
		})
	}

	invalid := map[string]string{
		"extra_key":  `{"literal":"fo*","left":"a"}`,
		"not_string": `{"literal":5}`,
		"in_list":    `{"left":"a","operator":"IN","right":{"left":[{"literal":5}],"operator":"LIST"}}`,
	}
	for name, raw := range invalid {
		t.Run("invalid/"+name, func(t *testing.T) {
			var got Expression
			if err := json.Unmarshal([]byte(raw), &got); err == nil {
				t.Fatalf("expected an error decoding %s, got %#v", raw, &got)
			}
		})
	}
}

func jsonEqual(got string, want string) bool {
	return stripWhitespace(got) == stripWhitespace(want)
}

func stripWhitespace(in string) string {
	return strings.Join(strings.Fields(in), "")
}
