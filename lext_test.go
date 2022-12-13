package lucene

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLex(t *testing.T) {
	type tc struct {
		in       string
		expected []token
	}
	tcs := map[string]tc{
		"empty_returns_eof": {
			in:       "",
			expected: []token{tok(tEOF, "EOF")},
		},
		"literals": {
			in:       "abc",
			expected: []token{tok(tLITERAL, "abc")},
		},
		"spaces_ignored": {
			in: "ab c",
			expected: []token{
				tok(tLITERAL, "ab"),
				tok(tLITERAL, "c"),
			},
		},
		"quotes_single_token": {
			in: `"abc"`,
			expected: []token{
				tok(tQUOTED, "\"abc\""),
			},
		},
		"single_quotes_single_token": {
			in: `'abc'`,
			expected: []token{
				tok(tQUOTED, "'abc'"),
			},
		},
		"quotes_single_token_with_spaces": {
			in: `"ab c"`,
			expected: []token{
				tok(tQUOTED, "\"ab c\""),
			},
		},
		"single_quotes_single_token_with_spaces": {
			in: `'ab c'`,
			expected: []token{
				tok(tQUOTED, "'ab c'"),
			},
		},
		"parens_tokenized": {
			in: `(ABC)`,
			expected: []token{
				tok(tLPAREN, "("),
				tok(tLITERAL, "ABC"),
				tok(tRPAREN, ")"),
			},
		},
		"equals_operator_tokenized_in_stream": {
			in: `a = b`,
			expected: []token{
				tok(tLITERAL, "a"),
				tok(tEQUAL, "="),
				tok(tLITERAL, "b"),
			},
		},
		"equals_operator_lucene_tokenized_in_stream": {
			in: `a:b`,
			expected: []token{
				tok(tLITERAL, "a"),
				tok(tCOLON, ":"),
				tok(tLITERAL, "b"),
			},
		},
		"and_boolean_tokenized": {
			in: `a AND b`,
			expected: []token{
				tok(tLITERAL, "a"),
				tok(tAND, "AND"),
				tok(tLITERAL, "b"),
			},
		},
		"or_boolean_tokenized": {
			in: `a OR b`,
			expected: []token{
				tok(tLITERAL, "a"),
				tok(tOR, "OR"),
				tok(tLITERAL, "b"),
			},
		},
		"not_boolean_tokenized": {
			in: `NOT a`,
			expected: []token{
				tok(tNOT, "NOT"),
				tok(tLITERAL, "a"),
			},
		},
		"to_tokenized": {
			in: `a TO b`,
			expected: []token{
				tok(tLITERAL, "a"),
				tok(tTO, "TO"),
				tok(tLITERAL, "b"),
			},
		},
		"regexp_tokenized": {
			in: `/a[b]*/`,
			expected: []token{
				tok(tREGEXP, "/a[b]*/"),
			},
		},
		"symbols_tokenized": {
			in: `()[]{}:+-=><`,
			expected: []token{
				tok(tLPAREN, "("),
				tok(tRPAREN, ")"),
				tok(tLSQUARE, "["),
				tok(tRSQUARE, "]"),
				tok(tLCURLY, "{"),
				tok(tRCURLY, "}"),
				tok(tCOLON, ":"),
				tok(tPLUS, "+"),
				tok(tMINUS, "-"),
				tok(tEQUAL, "="),
				tok(tGREATER, ">"),
				tok(tLESS, "<"),
			},
		},
		"token_boost": {
			in: "a:b^2 foo^4",
			expected: []token{
				tok(tLITERAL, "a"),
				tok(tCOLON, ":"),
				tok(tLITERAL, "b"),
				tok(tCARROT, "^"),
				tok(tLITERAL, "2"),
				tok(tLITERAL, "foo"),
				tok(tCARROT, "^"),
				tok(tLITERAL, "4"),
			},
		},
		"token_boost_floats": {
			in: "a:b^2.1 foo^4.40",
			expected: []token{
				tok(tLITERAL, "a"),
				tok(tCOLON, ":"),
				tok(tLITERAL, "b"),
				tok(tCARROT, "^"),
				tok(tLITERAL, "2.1"),
				tok(tLITERAL, "foo"),
				tok(tCARROT, "^"),
				tok(tLITERAL, "4.40"),
			},
		},
		"entire_stream_tokenized": {
			in: `(+k1:v1 AND -k2:v2) OR k3:"foo bar"^2 OR k4:a*~10`,
			expected: []token{
				tok(tLPAREN, "("),
				tok(tPLUS, "+"),
				tok(tLITERAL, "k1"),
				tok(tCOLON, ":"),
				tok(tLITERAL, "v1"),
				tok(tAND, "AND"),
				tok(tMINUS, "-"),
				tok(tLITERAL, "k2"),
				tok(tCOLON, ":"),
				tok(tLITERAL, "v2"),
				tok(tRPAREN, ")"),
				tok(tOR, "OR"),
				tok(tLITERAL, "k3"),
				tok(tCOLON, ":"),
				tok(tQUOTED, "\"foo bar\""),
				tok(tCARROT, "^"),
				tok(tLITERAL, "2"),
				tok(tOR, "OR"),
				tok(tLITERAL, "k4"),
				tok(tCOLON, ":"),
				tok(tLITERAL, "a*"),
				tok(tTILDE, "~"),
				tok(tLITERAL, "10"),
			},
		},
		"escape_sequence_tokenized": {
			in: `\(1\+1\)\:2`,
			expected: []token{
				tok(tLITERAL, `\(1\+1\)\:2`),
			},
		},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			tokens := consumeAll(tc.in)
			tc.expected = finalizeExpected(tc.in, tc.expected)
			assert.Equal(t, tc.expected, tokens)
		})
	}
}

func TestLexFailures(t *testing.T) {
	type tc struct {
		in       string
		expected []token
	}

	tcs := map[string]tc{
		"unterminated_quote": {
			in: `ab"abc`,
			expected: []token{
				tok(tLITERAL, "ab"),
				tok(tERR, "unterminated quote"),
			},
		},
		"paren_in_literal": {
			in: `ab(a)`,
			expected: []token{
				tok(tLITERAL, "ab"),
				tok(tLPAREN, "("),
				tok(tLITERAL, "a"),
				tok(tRPAREN, ")"),
			},
		},
		"unterminated_paren": {
			in: `((a AND b AND c      )`,
			expected: []token{
				tok(tLPAREN, "("),
				tok(tLPAREN, "("),
				tok(tLITERAL, "a"),
				tok(tAND, "AND"),
				tok(tLITERAL, "b"),
				tok(tAND, "AND"),
				tok(tLITERAL, "c"),
				tok(tRPAREN, ")"),
				tok(tERR, "unterminated ("),
			},
		},
		"unterminated_square": {
			in: `(((a)))[`,
			expected: []token{
				tok(tLPAREN, "("),
				tok(tLPAREN, "("),
				tok(tLPAREN, "("),
				tok(tLITERAL, "a"),
				tok(tRPAREN, ")"),
				tok(tRPAREN, ")"),
				tok(tRPAREN, ")"),
				tok(tLSQUARE, "["),
				tok(tERR, "unterminated ["),
			},
		},
		"unterminated_curly": {
			in: `{a OR (b AND c)`,
			expected: []token{
				tok(tLCURLY, "{"),
				tok(tLITERAL, "a"),
				tok(tOR, "OR"),
				tok(tLPAREN, "("),
				tok(tLITERAL, "b"),
				tok(tAND, "AND"),
				tok(tLITERAL, "c"),
				tok(tRPAREN, ")"),
				tok(tERR, "unterminated {"),
			},
		},
		"no_stack_cross_contamination": {
			in: `{()`,
			expected: []token{
				tok(tLCURLY, "{"),
				tok(tLPAREN, "("),
				tok(tRPAREN, ")"),
				tok(tERR, "unterminated {"),
			},
		},
		"no_starting_paren": {
			in: `a)`,
			expected: []token{
				tok(tLITERAL, "a"),
				tok(tRPAREN, ")"),
				tok(tERR, "unterminated ("),
			},
		},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			tokens := consumeAll(tc.in)
			tc.expected = finalizeExpected(tc.in, tc.expected)
			assert.Equal(t, tc.expected, tokens)
		})
	}
}

func finalizeExpected(in string, tokens []token) (out []token) {
	// if we are testing just the EOF return early and don't do anything
	if tokens[0].typ == tEOF {
		return tokens
	}

	offset := 0
	for idx, token := range tokens {
		sliced := in[offset:]

		// if its an error then we don't have any offset to calculate
		if token.typ == tERR {
			tokens[idx].pos = offset
			continue
		}

		// calculate the position of the new token in the string
		tokens[idx].pos = strings.Index(sliced, token.val) + offset

		// handle the whitespace that pops up so we keep the offset in sync
		whitespaceOffset := movePastWhitespace(sliced)
		offset += len(token.val) + whitespaceOffset
	}

	// if we didn't end in an error, add in an EOF token at the end
	if tokens[len(tokens)-1].typ != tERR {
		tokens = append(tokens, token{tEOF, len(in), "EOF"})
	}
	return tokens
}

func movePastWhitespace(in string) (count int) {
	for _, c := range in {
		if !isSpace(c) {
			return count
		}
		count++
	}
	return count
}

func consumeAll(in string) (toks []token) {
	l := lex(in)
	for {
		tok := l.nextToken()
		toks = append(toks, tok)
		if tok.typ == tEOF || tok.typ == tERR {
			return toks
		}
	}
}

func tok(typ tokType, val string) token {
	return token{
		typ: typ,
		// there is intentionally no pos set because we are doing it in generate
		val: val,
	}
}
