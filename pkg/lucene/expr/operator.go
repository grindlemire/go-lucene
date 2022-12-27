package expr

// Operator is an enum over the different valid lucene operations
type Operator int

// operations that can be used
const (
	Undefined Operator = iota
	And
	Or
	Equals
	Not
	Range
	Must
	MustNot
	Boost
	Fuzzy
	Literal
	Wild
	Regexp
)

// String renders the operator as a string
func (o Operator) String() string {
	return toString[o]
}

var fromString = map[string]Operator{
	"AND":      And,
	"OR":       Or,
	"EQUALS":   Equals,
	"NOT":      Not,
	"RANGE":    Range,
	"MUST":     Must,
	"MUST_NOT": MustNot,
	"BOOST":    Boost,
	"FUZZY":    Fuzzy,
	"LITERAL":  Literal,
	"WILD":     Wild,
	"REGEXP":   Regexp,
}

var toString = map[Operator]string{
	And:     "AND",
	Or:      "OR",
	Equals:  "EQUALS",
	Not:     "NOT",
	Range:   "RANGE",
	Must:    "MUST",
	MustNot: "MUST_NOT",
	Boost:   "BOOST",
	Fuzzy:   "FUZZY",
	Literal: "LITERAL",
	Wild:    "WILD",
	Regexp:  "REGEXP",
}
