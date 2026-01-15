# Design: Fix Regular Expression Slash Rendering for PostgreSQL (Issue #38)

## Overview

The current implementation includes the Lucene regex delimiter slashes (`/`) in the rendered PostgreSQL regex pattern. Lucene uses `/pattern/` syntax to denote regex, but PostgreSQL's `~` operator expects just the pattern without delimiters. This causes the slashes to become part of the pattern, resulting in failed matches. The fix involves stripping the leading and trailing slashes when rendering regex patterns for PostgreSQL.

## Problem Analysis

**Current behavior:**
- Input: `a:/b [c]/`
- Lexer stores: `/b [c]/` (includes slashes as part of the value)
- PostgreSQL output: `"a" ~ '/b [c]/'`
- The pattern `/b [c]/` requires the literal text to start with `/` and end with `/`

**Expected behavior:**
- Input: `a:/b [c]/`
- PostgreSQL output: `"a" ~ 'b [c]'`
- The pattern `b [c]` matches the intended text

**Root cause:** The slashes are preserved through the pipeline:
1. `lex.go:lexRegexp()` captures the content including the opening/closing `/` delimiters
2. `expression.go:REGEXP()` stores the value as-is
3. `renderfn.go:like()` and `likeParam()` check for slashes to detect regex but don't strip them
4. `base.go:serialize()` renders the string with quotes around the full value

## Architecture

### Data Flow (Current)
```
Input: "field:/pattern/"
   ↓
Lexer (TRegexp token with value "/pattern/")
   ↓
Parser creates expr.REGEXP("/pattern/")
   ↓
Driver serialize() → "'/pattern/'"
   ↓
like() detects slashes → "field" ~ '/pattern/'
```

### Data Flow (Proposed - Option A: Strip at render time)
```
Input: "field:/pattern/"
   ↓
Lexer (TRegexp token with value "/pattern/")
   ↓
Parser creates expr.REGEXP("/pattern/")
   ↓
Driver serialize() → strips slashes → "'pattern'"
   ↓
like() detects regex type → "field" ~ 'pattern'
```

### Alternative (Option B: Strip at parse time)
```
Input: "field:/pattern/"
   ↓
Lexer (TRegexp token with value "/pattern/")
   ↓
Parser creates expr.REGEXP("pattern") ← strips here
   ↓
Driver needs new way to detect regex type
```

## Trade-offs

### Option A: Strip slashes during SQL rendering (Recommended)

**Pros:**
- Minimal changes to existing code
- Preserves original Lucene semantics in the AST
- AST remains accurate representation of input
- JSON serialization roundtrips correctly
- Other drivers can handle regex differently if needed

**Cons:**
- Rendering code must know about Lucene's `/` delimiter convention

### Option B: Strip slashes during parsing

**Pros:**
- AST contains "clean" regex pattern

**Cons:**
- Loses information about original input
- Breaks JSON serialization roundtrip (can't reconstruct original query)
- Expression type (`Regexp`) already identifies it as regex, so slashes are redundant anyway
- Would require changes to validation and multiple test cases

**Decision:** Option A is preferred because it:
1. Maintains backward compatibility with JSON serialization
2. Keeps the AST as a faithful representation of the input
3. Contains changes to a single location (rendering)
4. Aligns with the principle that the AST captures syntax, drivers interpret semantics

## Interfaces

### Functions to Modify

**`pkg/driver/base.go:serialize()`** - For non-parameterized queries:
```go
func (b Base) serialize(in any) (s string, err error) {
    // ...existing code...
    case string:
        // NEW: Strip leading/trailing slashes from regex patterns
        // This is called after the expression tree is built, where
        // Regexp expressions store values like "/pattern/"
        // PostgreSQL's ~ operator expects just "pattern"
        return fmt.Sprintf("'%s'", strings.ReplaceAll(v, "'", "''")), nil
}
```

The actual stripping logic needs to happen when we know we're dealing with a regex. This requires passing context about the operator type.

**Better approach - modify `pkg/driver/renderfn.go:like()`:**
```go
func like(left, right string) (string, error) {
    // right comes in as "'/.../'" for regex
    if len(right) >= 4 && right[1] == '/' && right[len(right)-2] == '/' {
        // Strip the slashes: "'/.../'" → "'...'"
        stripped := "'" + right[2:len(right)-2] + "'"
        return fmt.Sprintf("%s ~ %s", left, stripped), nil
    }
    // ...existing wildcard handling...
}
```

**`pkg/driver/renderfn.go:likeParam()`:**
```go
func likeParam(left, right string, params []any) (string, error) {
    if len(params) == 1 {
        pright := params[0].(string)
        if len(pright) >= 2 && pright[0] == '/' && pright[len(pright)-1] == '/' {
            // Strip slashes from the param value
            params[0] = pright[1 : len(pright)-1]
            return fmt.Sprintf("%s ~ %s", left, right), nil
        }
    }
    return fmt.Sprintf("%s SIMILAR TO %s", left, right), nil
}
```

Note: `likeParam` receives `params` but doesn't modify the caller's slice. Need to check if this requires returning modified params or if the slice is shared.

### Affected Test Cases

Tests in `postgresql_test.go` that will need updated expected values:
- `regexp` - current: `"a" ~ '/b [c]/'`, expected: `"a" ~ 'b [c]'`
- `regexp_with_keywords` - current: `"a" ~ '/b "[c]/'`, expected: `"a" ~ 'b "[c]'`
- `regexp_with_escaped_chars` - current: `"url" ~ '/example.com\/foo\/bar\/.*/'`, expected: `"url" ~ 'example.com\/foo\/bar\/.*'`
- `exclamation_mark_inside_regexp_is_literal` - current: `"field" ~ '/pattern with ! inside/'`, expected: `"field" ~ 'pattern with ! inside'`

Parameterized tests:
- `regexp` - params should change from `["/b [c]/"]` to `["b [c]"]`
- `regexp_with_keywords` - params should change from `["/b \"[c]/"]` to `["b \"[c]"]`
- `regexp_with_escaped_chars` - params should change from `["/example.com\\/foo\\/bar\\/.*/"]` to `["example.com\\/foo\\/bar\\/.*"]`

## Constraints

1. **Do not modify the lexer** - The slash delimiters are part of Lucene syntax and should be preserved in the token
2. **Do not modify expression.go** - Keep AST as faithful representation of input
3. **JSON roundtrip must continue to work** - The existing JSON serialization tests must pass
4. **Escape handling must be preserved** - Escaped slashes within the pattern (e.g., `\/`) should remain intact
5. **Edge cases to handle:**
   - Empty regex: `/./` (single dot)
   - Regex with internal slashes: `/a/b/` should become `a/b`
   - Very short patterns

## Verification

All existing tests should pass after updating expected values. Additionally:
- Verify against real PostgreSQL that patterns match correctly
- Ensure escaped characters within patterns are preserved
