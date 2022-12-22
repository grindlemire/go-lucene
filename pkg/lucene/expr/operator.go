package expr

type Expr int

const (
	And Expr = iota
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

var fromJSON = map[string]Expr{
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

var toJSON = map[Expr]string{
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
