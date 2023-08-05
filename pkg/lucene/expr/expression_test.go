package expr

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
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

func jsonEqual(got string, want string) bool {
	return stripWhitespace(got) == stripWhitespace(want)
}

func stripWhitespace(in string) string {
	return strings.Join(strings.Fields(in), "")
}
