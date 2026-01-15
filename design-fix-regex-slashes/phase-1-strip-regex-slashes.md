---
phase: 1
name: strip-regex-slashes
design: ./design.md
depends-on: []
verification:
  command: "go test ./..."
  expected: "PASS"
status: pending
---

# Phase 1: Strip Regex Delimiter Slashes in PostgreSQL Rendering

## Objective

Modify the PostgreSQL driver's regex rendering to strip Lucene's `/` delimiters from regex patterns, so `field:/pattern/` renders as `"field" ~ 'pattern'` instead of `"field" ~ '/pattern/'`.

## Tasks

### 1. Modify `like()` function in `pkg/driver/renderfn.go`

When detecting a regex pattern (string wrapped in `'/.../'`), strip the slash delimiters before outputting:
- Current: detects regex and outputs with slashes intact
- New: detect regex, strip the leading and trailing `/`, then output

### 2. Modify `likeParam()` function in `pkg/driver/renderfn.go`

When detecting a regex in the params slice, strip the slashes from the parameter value:
- The params slice is passed by reference, so modifying `params[0]` will affect the caller's slice
- Strip `/` from start and end of the regex string param

### 3. Update test expectations in `postgresql_test.go`

Update the following test cases in `TestPostgresSQLEndToEnd`:
- `regexp`: `"a" ~ '/b [c]/'` → `"a" ~ 'b [c]'`
- `regexp_with_keywords`: `"a" ~ '/b "[c]/'` → `"a" ~ 'b "[c]'`
- `regexp_with_escaped_chars`: `"url" ~ '/example.com\/foo\/bar\/.*/'` → `"url" ~ 'example.com\/foo\/bar\/.*'`
- `exclamation_mark_inside_regexp_is_literal`: `"field" ~ '/pattern with ! inside/'` → `"field" ~ 'pattern with ! inside'`

Update the following test cases in `TestPostgresParameterizedSQLEndToEnd`:
- `regexp`: params `["/b [c]/"]` → `["b [c]"]`
- `regexp_with_keywords`: params `["/b \"[c]/"]` → `["b \"[c]"]`
- `regexp_with_escaped_chars`: params `["/example.com\/foo\/bar\/.*/"]` → `["example.com\/foo\/bar\/.*"]`

## Constraints

- Do NOT modify the lexer or expression packages
- Preserve escape sequences within patterns (e.g., `\/` stays as `\/`)
- Ensure minimum pattern length checks account for the shorter stripped strings
