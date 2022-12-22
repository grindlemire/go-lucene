package expr

// Operation is an enum over the different valid lucene operations
type Operation int

// operations that can be used
const (
	Undefined Operation = iota
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

var fromJSON = map[string]Operation{
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

var toJSON = map[Operation]string{
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
