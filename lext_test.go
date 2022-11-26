package lucene

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

type tc struct {
	in       string
	expected []token
}

func TestLex(t *testing.T) {

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
		"entire_stream_tokenized": {
			in: `(+k1:v1 AND -k2:v2) OR k3:"foo bar" OR k4:a*`,
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
				tok(tOR, "OR"),
				tok(tLITERAL, "k4"),
				tok(tCOLON, ":"),
				tok(tLITERAL, "a*"),
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

func finalizeExpected(in string, tokens []token) (out []token) {
	// if we are testing just the EOF return early and don't do anything
	if tokens[0].typ == tEOF {
		return tokens
	}

	offset := 0
	for idx, token := range tokens {
		sliced := in[offset:]
		tokens[idx].pos = strings.Index(sliced, token.val) + offset

		offset += len(token.val)
	}
	tokens = append(tokens, token{tEOF, len(in), "EOF"})
	return tokens
}

func consumeAll(in string) (toks []token) {
	l := lex(in)
	for {
		tok := l.nextItem()
		toks = append(toks, tok)
		if tok.typ == tEOF {
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
