package lex

import (
	"reflect"
	"strings"
	"testing"
)

const errTemplate = "%s:\n    wanted %v\n    got    %v"

func TestLex(t *testing.T) {
	type tc struct {
		in       string
		expected []Token
	}
	tcs := map[string]tc{
		"empty_returns_eof": {
			in:       "",
			expected: []Token{tok(TEOF, "EOF")},
		},
		"negatives": {
			in:       "-1",
			expected: []Token{tok(TLiteral, "-1")},
		},
		"negatives_mixed_with_minus": {
			in: "a:-1 AND -b:c",
			expected: []Token{
				tok(TLiteral, "a"),
				tok(TColon, ":"),
				tok(TLiteral, "-1"),
				tok(TAnd, "AND"),
				tok(TMinus, "-"),
				tok(TLiteral, "b"),
				tok(TColon, ":"),
				tok(TLiteral, "c"),
			},
		},
		"literals": {
			in:       "abc",
			expected: []Token{tok(TLiteral, "abc")},
		},
		"spaces_ignored": {
			in: "ab c",
			expected: []Token{
				tok(TLiteral, "ab"),
				tok(TLiteral, "c"),
			},
		},
		"quotes_single_token": {
			in: `"abc"`,
			expected: []Token{
				tok(TQuoted, "\"abc\""),
			},
		},
		"single_quotes_single_token": {
			in: `'abc'`,
			expected: []Token{
				tok(TQuoted, "'abc'"),
			},
		},
		"quotes_single_token_with_spaces": {
			in: `"ab c"`,
			expected: []Token{
				tok(TQuoted, "\"ab c\""),
			},
		},
		"single_quotes_single_token_with_spaces": {
			in: `'ab c'`,
			expected: []Token{
				tok(TQuoted, "'ab c'"),
			},
		},
		"parens_tokenized": {
			in: `(ABC)`,
			expected: []Token{
				tok(TLParen, "("),
				tok(TLiteral, "ABC"),
				tok(TRParen, ")"),
			},
		},
		"equals_operator_tokenized_in_stream": {
			in: `a = b`,
			expected: []Token{
				tok(TLiteral, "a"),
				tok(TEqual, "="),
				tok(TLiteral, "b"),
			},
		},
		"equals_operator_lucene_tokenized_in_stream": {
			in: `a:b`,
			expected: []Token{
				tok(TLiteral, "a"),
				tok(TColon, ":"),
				tok(TLiteral, "b"),
			},
		},
		"and_boolean_tokenized": {
			in: `a AND b`,
			expected: []Token{
				tok(TLiteral, "a"),
				tok(TAnd, "AND"),
				tok(TLiteral, "b"),
			},
		},
		"or_boolean_tokenized": {
			in: `a OR b`,
			expected: []Token{
				tok(TLiteral, "a"),
				tok(TOr, "OR"),
				tok(TLiteral, "b"),
			},
		},
		"not_boolean_tokenized": {
			in: `NOT a`,
			expected: []Token{
				tok(TNot, "NOT"),
				tok(TLiteral, "a"),
			},
		},
		"to_tokenized": {
			in: `a TO b`,
			expected: []Token{
				tok(TLiteral, "a"),
				tok(TTO, "TO"),
				tok(TLiteral, "b"),
			},
		},
		"regexp_tokenized": {
			in: `/a[b]*/`,
			expected: []Token{
				tok(TRegexp, "/a[b]*/"),
			},
		},
		"symbols_tokenized": {
			in: `()[]{}:+-=><`,
			expected: []Token{
				tok(TLParen, "("),
				tok(TRParen, ")"),
				tok(TLSquare, "["),
				tok(TRSquare, "]"),
				tok(TLCurly, "{"),
				tok(TRCurly, "}"),
				tok(TColon, ":"),
				tok(TPlus, "+"),
				tok(TMinus, "-"),
				tok(TEqual, "="),
				tok(TGreater, ">"),
				tok(TLess, "<"),
			},
		},
		"token_boost": {
			in: "a:b^2 foo^4",
			expected: []Token{
				tok(TLiteral, "a"),
				tok(TColon, ":"),
				tok(TLiteral, "b"),
				tok(TCarrot, "^"),
				tok(TLiteral, "2"),
				tok(TLiteral, "foo"),
				tok(TCarrot, "^"),
				tok(TLiteral, "4"),
			},
		},
		"token_boost_floats": {
			in: "a:b^2.1 foo^4.40",
			expected: []Token{
				tok(TLiteral, "a"),
				tok(TColon, ":"),
				tok(TLiteral, "b"),
				tok(TCarrot, "^"),
				tok(TLiteral, "2.1"),
				tok(TLiteral, "foo"),
				tok(TCarrot, "^"),
				tok(TLiteral, "4.40"),
			},
		},
		"entire_stream_tokenized": {
			in: `(+k1:v1 AND -k2:v2) OR k3:"foo bar"^2 OR k4:a*~10`,
			expected: []Token{
				tok(TLParen, "("),
				tok(TPlus, "+"),
				tok(TLiteral, "k1"),
				tok(TColon, ":"),
				tok(TLiteral, "v1"),
				tok(TAnd, "AND"),
				tok(TMinus, "-"),
				tok(TLiteral, "k2"),
				tok(TColon, ":"),
				tok(TLiteral, "v2"),
				tok(TRParen, ")"),
				tok(TOr, "OR"),
				tok(TLiteral, "k3"),
				tok(TColon, ":"),
				tok(TQuoted, "\"foo bar\""),
				tok(TCarrot, "^"),
				tok(TLiteral, "2"),
				tok(TOr, "OR"),
				tok(TLiteral, "k4"),
				tok(TColon, ":"),
				tok(TLiteral, "a*"),
				tok(TTilde, "~"),
				tok(TLiteral, "10"),
			},
		},
		"escape_sequence_tokenized": {
			in: `\(1\+1\)\:2`,
			expected: []Token{
				tok(TLiteral, `\(1\+1\)\:2`),
			},
		},
		"quoted_sequence_tokensized": {
			in: `"foo bar":"works well"`,
			expected: []Token{
				tok(TQuoted, "\"foo bar\""),
				tok(TColon, ":"),
				tok(TQuoted, "\"works well\""),
			},
		},
	}

	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			tokens := consumeAll(tc.in)
			tc.expected = finalizeExpected(tc.in, tc.expected)
			if !reflect.DeepEqual(tc.expected, tokens) {
				t.Fatalf(errTemplate, "token streams don't match", tc.expected, tokens)
			}
		})
	}
}

func finalizeExpected(in string, tokens []Token) (out []Token) {
	// if we are testing just the EOF return early and don't do anything
	if tokens[0].Typ == TEOF {
		return tokens
	}

	offset := 0
	for idx, token := range tokens {
		sliced := in[offset:]

		// if its an error then we don't have any offset to calculate
		if token.Typ == TErr {
			tokens[idx].pos = offset
			continue
		}

		// calculate the position of the new token in the string
		tokens[idx].pos = strings.Index(sliced, token.Val) + offset

		// handle the whitespace that pops up so we keep the offset in sync
		whitespaceOffset := movePastWhitespace(sliced)
		offset += len(token.Val) + whitespaceOffset
	}

	// if we didn't end in an error, add in an EOF token at the end
	if tokens[len(tokens)-1].Typ != TErr {
		tokens = append(tokens, Token{TEOF, len(in), "EOF"})
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

func consumeAll(in string) (toks []Token) {
	l := Lex(in)
	for {
		tok := l.Next()
		toks = append(toks, tok)
		if tok.Typ == TEOF || tok.Typ == TErr {
			return toks
		}
	}
}

func tok(typ TokType, val string) Token {
	return Token{
		Typ: typ,
		// there is intentionally no pos set because we are doing it in generate
		Val: val,
	}
}
