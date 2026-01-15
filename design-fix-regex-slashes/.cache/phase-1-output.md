# Phase 1 Output: Strip Regex Delimiter Slashes in PostgreSQL Rendering

## Status: COMPLETED

## Summary

Successfully modified the PostgreSQL driver to strip Lucene's `/` delimiters from regex patterns, so `field:/pattern/` now renders as `"field" ~ 'pattern'` instead of `"field" ~ '/pattern/'`.

## Changes Made

### 1. `pkg/driver/renderfn.go`

**Added `regexpLiteral` function** (lines 26-40):
- New render function specifically for `expr.Regexp` expressions
- Strips leading and trailing slashes from regex patterns formatted as `'/.../`
- Used instead of `literal` for standalone regex literals

**Added `likeRender` function** (lines 57-68):
- New helper function for the Like operator with explicit `isRegex` flag
- Called from `Render()` which determines if the right side is a Regexp expression
- Outputs `~ pattern` for regex patterns, `SIMILAR TO pattern` for wildcards

**Modified `likeParam` function** (lines 70-76):
- Updated signature to accept `isRegex bool` parameter
- Simplified logic to use the passed flag instead of detecting regex by slash presence

### 2. `pkg/driver/base.go`

**Updated `Shared` map** (line 24):
- Changed `expr.Regexp: literal` to `expr.Regexp: regexpLiteral`

**Modified `RenderParam` function** (lines 63-77):
- Added `isRegex` flag tracking
- Strip slashes from regex patterns before appending to params
- Pass `isRegex` flag to `likeParam`

**Modified `Render` function** (lines 150-158):
- Added special handling for `Like` operator
- Detect if right side is a `Regexp` expression
- Call `likeRender` with appropriate `isRegex` flag

### 3. `postgresql_test.go` (root)

**Updated test expectations in `TestPostgresSQLEndToEnd`**:
- `regexp`: `"a" ~ '/b [c]/'` -> `"a" ~ 'b [c]'`
- `regexp_with_keywords`: `"a" ~ '/b "[c]/'` -> `"a" ~ 'b "[c]'`
- `regexp_with_escaped_chars`: `"url" ~ '/example.com\/foo\/bar\/.*/'` -> `"url" ~ 'example.com\/foo\/bar\/.*'`
- `exclamation_mark_inside_regexp_is_literal`: `"field" ~ '/pattern with ! inside/'` -> `"field" ~ 'pattern with ! inside'`

**Updated test expectations in `TestPostgresParameterizedSQLEndToEnd`**:
- `regexp`: params `["/b [c]/"]` -> `["b [c]"]`
- `regexp_with_keywords`: params `["/b \"[c]/"]` -> `["b \"[c]"]`
- `regexp_with_escaped_chars`: params `["/example.com\/foo\/bar\/.*/"]` -> `["example.com\/foo\/bar\/.*"]`

### 4. `pkg/driver/postgresql_test.go`

**Updated test expectations in `TestSQLDriver`**:
- `regexp`: `'/b*ar/'` -> `'b*ar'`
- `nested_filter`: regex portion `~ '/b*ar/'` -> `~ 'b*ar'`

## Verification

```
$ go test ./...
ok  	github.com/grindlemire/go-lucene	0.205s
?   	github.com/grindlemire/go-lucene/cmd	[no test files]
ok  	github.com/grindlemire/go-lucene/internal/lex	(cached)
ok  	github.com/grindlemire/go-lucene/pkg/driver	0.360s
ok  	github.com/grindlemire/go-lucene/pkg/lucene/expr	(cached)
?   	github.com/grindlemire/go-lucene/pkg/lucene/reduce	[no test files]
```

All tests pass.

## Technical Notes

1. **Two code paths**: The fix had to handle both the parameterized (`RenderParam`) and non-parameterized (`Render`) code paths.

2. **Regex detection**: For parameterized queries, regex is detected by checking if the param string starts and ends with `/`. For non-parameterized queries, regex is detected by checking if the right-side expression has `Op == expr.Regexp`.

3. **Escape sequences preserved**: Escaped characters within patterns (e.g., `\/`) are preserved correctly.

4. **Backward compatibility**: The original `like` function with string-based slash detection is preserved for any edge cases, but the primary detection now uses expression type checking.
