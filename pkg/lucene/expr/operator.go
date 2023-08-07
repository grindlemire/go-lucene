package expr

// Operator is an enum over the different valid lucene operations
type Operator int

// operations that can be used
// To add a new operator, do the following:
// 1. Add it to the iota here
// 2. Add it to the string maps below
// 3. Add a render function for it at least in base, perhaps in all the drivers as well
// 4. Update the json parsing and tests to support the new operator
// 5. Add tests in parse_test and expression_test
const (
	Undefined Operator = iota
	And
	Or
	Equals
	Like
	Not
	Range
	Must
	MustNot
	Boost
	Fuzzy
	Literal
	Wild
	Regexp
	Greater
	Less
	GreaterEq
	LessEq
	In
	List
)

// String renders the operator as a string
func (o Operator) String() string {
	return toString[o]
}

var fromString = map[string]Operator{
	"AND":        And,
	"OR":         Or,
	"EQUALS":     Equals,
	"LIKE":       Like,
	"NOT":        Not,
	"RANGE":      Range,
	"MUST":       Must,
	"MUST_NOT":   MustNot,
	"BOOST":      Boost,
	"FUZZY":      Fuzzy,
	"LITERAL":    Literal,
	"WILD":       Wild,
	"REGEXP":     Regexp,
	"GREATER":    Greater,
	"LESS":       Less,
	"GREATER_EQ": GreaterEq,
	"LESS_EQ":    LessEq,
	"IN":         In,
	"LIST":       List,
}

var toString = map[Operator]string{
	And:       "AND",
	Or:        "OR",
	Equals:    "EQUALS",
	Like:      "LIKE",
	Not:       "NOT",
	Range:     "RANGE",
	Must:      "MUST",
	MustNot:   "MUST_NOT",
	Boost:     "BOOST",
	Fuzzy:     "FUZZY",
	Literal:   "LITERAL",
	Wild:      "WILD",
	Regexp:    "REGEXP",
	Greater:   "GREATER",
	Less:      "LESS",
	GreaterEq: "GREATER_EQ",
	LessEq:    "LESS_EQ",
	In:        "IN",
	List:      "LIST",
}
