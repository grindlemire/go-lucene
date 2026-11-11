# Changelog

## [0.3.1](https://github.com/grindlemire/go-lucene/compare/v0.3.0...v0.3.1) (2026-11-11)


### Bug Fixes

* parse each term without splitting it into a list of characters, which brings parsing back to the speed and memory of 0.2.1. Since 0.3.0 the parser built that list for every term to look for wildcards and escapes, so a field group of 100 escaped ids took about twice the time and four and a half times the memory of 0.2.1 ([#94](https://github.com/grindlemire/go-lucene/pull/94), [34274af](https://github.com/grindlemire/go-lucene/commit/34274afa82381edd356d06c38eefed6dd540a108)), closes [#93](https://github.com/grindlemire/go-lucene/issues/93)

## [0.3.0](https://github.com/grindlemire/go-lucene/compare/v0.2.1...v0.3.0) (2026-11-09)


### Highlights

- **JSON fields.** `lucene.WithJSONFields("labels")` makes `labels.category:fruit` query a key inside a JSON column, on Postgres, MySQL/MariaDB and SQLite. Every operator works on a JSON key: comparisons, ranges, IN lists, wildcards, regex and null. ([#66](https://github.com/grindlemire/go-lucene/pull/66))
- **Case-insensitive fields.** `lucene.WithCaseInsensitiveFields("name")` makes matches on those fields ignore case. ([#73](https://github.com/grindlemire/go-lucene/pull/73))
- **Parsing matches Lucene more closely.** This covers field groups, `+` inside terms, numbers like `1E+3` and `.5`, and escapes.
- **Safer SQL.** Values that could write raw text into the SQL are rejected. Numbers too large or too precise for a float64 reach the database with every digit.
- **Queries built in code don't panic.** A missing operand, such as `expr.Eq("a", nil)` or an empty `expr.LIST()`, returns an error instead of panicking or producing broken SQL.
- **`String()` prints a query that parses back to the same query.**

### Upgrade notes

#### ⚠ BREAKING CHANGES: backslashes in patterns built in code

A backslash in a pattern built with `expr.LIKE` or `expr.WILD`, or read from JSON, now means "the next character is literal" in every dialect. MySQL and SQLite used to read it as a plain backslash. Queries parsed from Lucene text aren't affected. ([#82](https://github.com/grindlemire/go-lucene/pull/82))

```go
expr.LIKE("path", `C:\Users\*`)
```

| | 0.2.1 | 0.3.0 |
|---|---|---|
| MySQL | `LIKE 'C:\\Users\\%'`: starts with `C:\Users\` | `LIKE 'C:Users*'`: exactly `C:Users*` |
| SQLite | `GLOB 'C:\Users\*'`: starts with `C:\Users\` | `GLOB 'C:Users[*]'`: exactly `C:Users*` |
| Postgres | `SIMILAR TO 'C:\Users\%'`: exactly `C:Users%` | `SIMILAR TO 'C:Users\*'`: exactly `C:Users*` |

To match a literal backslash, double it (`` `C:\\Users\\*` ``) or escape the text with `expr.EscapeWildcard`:

```go
expr.LIKE("path", expr.EscapeWildcard(`C:\Users\`)+"*")
```

#### JSON written by 0.3.0 can't be read by 0.2.1

The JSON form of a query has new object forms: `{"literal":…}`, `{"wildcard":…}`, `{"regexp":…}`, `{"number":…}` and `{"json_path":…}`. 0.2.1 can't decode them. If services share this JSON, upgrade the ones that read it before the ones that write it.

#### Hand-written JSON with bare numbers

A bare JSON number now decodes the way the same number parses in a query:
- `9223372036854775808` keeps every digit.
- `1.0`, `1E2` and `-0` keep the text they were written as.
- A range bound written `5.0` is now a float. It used to be the integer 5.
- `1e400` and `1e-400` are errors. `1e-400` used to decode as 0.

#### Queries that now parse differently

These examples use the default field `df` on Postgres:

| Query | 0.2.1 | 0.3.0 |
|---|---|---|
| `a:(x AND -y)` | `"a" = (("df" = 'x') AND ...)` | `("a" = 'x') AND (NOT("a" = 'y'))` |
| `a:(b:c)` | `"a" = ("b" = 'c')` | `"b" = 'c'` |
| `c++` | parse error | `"df" = 'c++'` |
| `x (y)` | parse error | `("df" = 'x') AND ("df" = 'y')` |
| `0:1` | `0 = 1` | `"0" = 1` (column `0`) |
| `1E+3` | `("df" = '1E') AND ("df" = 3)` | `"df" = 1000` |
| `a:.5` | parse error | `"a" = 0.5` |
| `a:fo\*` | `SIMILAR TO 'fo\%'` | `"a" = 'fo*'` |
| `a:b\\c` | `'bc'` | `'b\c'` |
| `a:[a* TO b]` | `"a" <= 'b'` | `"a" BETWEEN 'a*' AND 'b'` |

### Features

* case-insensitive matching for listed fields ([#73](https://github.com/grindlemire/go-lucene/pull/73), [6766bbe](https://github.com/grindlemire/go-lucene/commit/6766bbe5e3beac7c83c9d02d49dd00669561906c))
* render JSON sub-field paths in SQL dialects ([#66](https://github.com/grindlemire/go-lucene/pull/66), [2b1d354](https://github.com/grindlemire/go-lucene/commit/2b1d354a6d40fe75143542000f21be1499727f73))
* add `expr.EscapeWildcard` for text in a pattern built in code ([#82](https://github.com/grindlemire/go-lucene/pull/82), [3bae41e](https://github.com/grindlemire/go-lucene/commit/3bae41e64b87c432da233020908cdf6b57484b40))

### Bug Fixes

#### Parsing

* apply a field group's field to every term inside it, and keep the default field out of field groups ([#74](https://github.com/grindlemire/go-lucene/pull/74), [#78](https://github.com/grindlemire/go-lucene/pull/78), [027b0d0](https://github.com/grindlemire/go-lucene/commit/027b0d0b98e493a5f503e6628f2f3f0ef1ab18cf), [360a378](https://github.com/grindlemire/go-lucene/commit/360a378a7a5cb9dc2cbc9a158e3d6fc03b16e6fe)), closes [#67](https://github.com/grindlemire/go-lucene/issues/67)
* give the default field to wildcards, regexes, nulls and ranges ([#79](https://github.com/grindlemire/go-lucene/pull/79), [b23ff55](https://github.com/grindlemire/go-lucene/commit/b23ff558af116b527a39d2f290f1d2f0cb077c57))
* insert an implicit AND before a parenthesised group ([#63](https://github.com/grindlemire/go-lucene/pull/63), [2297401](https://github.com/grindlemire/go-lucene/commit/2297401282d89db82785a329f2a5c33100aa821a))
* keep a `+` inside a term in the term, and keep a `+` at the end of the query out of the number before it ([#81](https://github.com/grindlemire/go-lucene/pull/81), [#82](https://github.com/grindlemire/go-lucene/pull/82), [#84](https://github.com/grindlemire/go-lucene/pull/84), [59940ac](https://github.com/grindlemire/go-lucene/commit/59940aca93e7a326711641433e2ab33492003487), [a47116a](https://github.com/grindlemire/go-lucene/commit/a47116ab55b20a105fe9eaf33094705de567ea94), [aca5477](https://github.com/grindlemire/go-lucene/commit/aca5477a67fc6585f00eb9e31a04f64201edbda6))
* lex numbers written as `1E+3` or `.5`, and treat only plain decimal numbers as numbers ([#75](https://github.com/grindlemire/go-lucene/pull/75), [#81](https://github.com/grindlemire/go-lucene/pull/81), [adc9eff](https://github.com/grindlemire/go-lucene/commit/adc9eff527825fb771bec48606184ebeaae0755c), [11e60a8](https://github.com/grindlemire/go-lucene/commit/11e60a88039ca24ab55aa6829fcc799a72491f75), [a971d24](https://github.com/grindlemire/go-lucene/commit/a971d2434162c0d3dd4cb3956e3d4a805c8ec7b3))
* keep a nonzero number too small for a float64 as text ([#85](https://github.com/grindlemire/go-lucene/pull/85), [a1aea78](https://github.com/grindlemire/go-lucene/commit/a1aea78755e1a0c74be19395f488a722eed63252))
* treat a number used as a field as a column, also with `=` and in parentheses ([#80](https://github.com/grindlemire/go-lucene/pull/80), [172d8f2](https://github.com/grindlemire/go-lucene/commit/172d8f217a01848fd087aa43b51c9e1f99d00917))
* treat an escaped `*` or `?` as a literal character, keep an escaped backslash, and reject a backslash at the end of the query ([#81](https://github.com/grindlemire/go-lucene/pull/81), [#82](https://github.com/grindlemire/go-lucene/pull/82), [315512f](https://github.com/grindlemire/go-lucene/commit/315512f1aa0541036e191b67c58946df1530dd38), [685a33a](https://github.com/grindlemire/go-lucene/commit/685a33a0ca3d62538db5ecc2ee87b7d83dce7e94), [3fdd1a5](https://github.com/grindlemire/go-lucene/commit/3fdd1a535007753c8d20ca767db48effef2946d4))
* drop the escapes from a wildcard used as a field ([#91](https://github.com/grindlemire/go-lucene/pull/91), [4ad235c](https://github.com/grindlemire/go-lucene/commit/4ad235c357df333d39c7f280993f1c539e8bbf88))
* read a fuzzy distance only from an unquoted number ([#75](https://github.com/grindlemire/go-lucene/pull/75), [03b7f98](https://github.com/grindlemire/go-lucene/commit/03b7f987cca298085feabc11a79bf3f38ee69257))
* report the lexer's error when a query fails to lex ([#81](https://github.com/grindlemire/go-lucene/pull/81), [f9b6cb9](https://github.com/grindlemire/go-lucene/commit/f9b6cb91d3298fd70b481477d763fde980b5ebfe))

#### SQL output

* reject compound range bounds and unescaped values in generated SQL ([#72](https://github.com/grindlemire/go-lucene/pull/72), [f4a97e0](https://github.com/grindlemire/go-lucene/commit/f4a97e0a3cb6fa3778b8b220f234dcc4adfd0d0b))
* reject a comparison against a group or a range, and a field operator with no field ([#78](https://github.com/grindlemire/go-lucene/pull/78), [#86](https://github.com/grindlemire/go-lucene/pull/86), [b3c1160](https://github.com/grindlemire/go-lucene/commit/b3c11600ad684f99521193ec1e0783feb1a8f545), [9dd67e1](https://github.com/grindlemire/go-lucene/commit/9dd67e1fbf76b168134b535cbc38487716b72115))
* require a single term in a comparison, an IN list item and a field ([#86](https://github.com/grindlemire/go-lucene/pull/86), [213e323](https://github.com/grindlemire/go-lucene/commit/213e323b1325c0f3519099f62a266e9082fb2d15), [a7e08df](https://github.com/grindlemire/go-lucene/commit/a7e08df9e895c59b9410b418b96e54ee5ed61a09))
* treat only a bare `*` as an open range bound ([#72](https://github.com/grindlemire/go-lucene/pull/72), [f7660ba](https://github.com/grindlemire/go-lucene/commit/f7660ba9b5144d1c2f5c697a7325c2f3e584dc9d))
* compare against column range bounds in parameterized mode ([#72](https://github.com/grindlemire/go-lucene/pull/72), [945e126](https://github.com/grindlemire/go-lucene/commit/945e12694fe25904a21c9fc23fececb54303dbd7))
* write a number that a float64 rounds into the SQL with all its digits, and check that text before writing it ([#85](https://github.com/grindlemire/go-lucene/pull/85), [ad87b18](https://github.com/grindlemire/go-lucene/commit/ad87b183c81d8b8b6a562e53818b05c52e2ec602), [495ecfa](https://github.com/grindlemire/go-lucene/commit/495ecfab9fabca7401cec88e936edabf38502502), [543894d](https://github.com/grindlemire/go-lucene/commit/543894ddfe2aea9205d53ea94d1e3d9f2f96dbbb), [7b38388](https://github.com/grindlemire/go-lucene/commit/7b383889871ab96392c20b2f219c62fc755be1e2))
* keep escapes inside a bracket expression of a pattern built in code ([#82](https://github.com/grindlemire/go-lucene/pull/82), [acff425](https://github.com/grindlemire/go-lucene/commit/acff425da10c084f23c35c7591297e923d4c05bf))

#### JSON fields

* make JSON path comparisons type-safe, match JSON numbers stored as text, and check paths when the query is parsed ([#66](https://github.com/grindlemire/go-lucene/pull/66), [9e25fc6](https://github.com/grindlemire/go-lucene/commit/9e25fc67003fc619c1237e40987b6c81a1016764), [4884543](https://github.com/grindlemire/go-lucene/commit/488454393ee6bf9f3de9e39326f3b691f95b7528))
* reject an escaped dot in a JSON field, also before `=` and inside parentheses ([#91](https://github.com/grindlemire/go-lucene/pull/91), [fb0cbb0](https://github.com/grindlemire/go-lucene/commit/fb0cbb029f55d9bd4ae6834cec23ceebd7857b40))
* keep a number's written text, and let plain-text matches use an index ([#66](https://github.com/grindlemire/go-lucene/pull/66), [eb181c6](https://github.com/grindlemire/go-lucene/commit/eb181c6053690e4083e948b9b01692bd10ab3d9e), [4dc07b7](https://github.com/grindlemire/go-lucene/commit/4dc07b7d1e51e9439a476c166af25591e679cd9d))
* compare a bool as text ([#66](https://github.com/grindlemire/go-lucene/pull/66), [5d64fa5](https://github.com/grindlemire/go-lucene/commit/5d64fa59772421e329598416b850ba3dcf2ff8af))
* match a case-insensitive JSON path by its plain dotted name ([#87](https://github.com/grindlemire/go-lucene/pull/87), [6a0d2cf](https://github.com/grindlemire/go-lucene/commit/6a0d2cf5880c5eb324feaf7a71a307700449f90c), [af714b9](https://github.com/grindlemire/go-lucene/commit/af714b9099d89e2b60ae24f5e8951769e5190c7a))

#### Queries built in code or read from JSON

* report missing operands instead of panicking, including inside groups and LISTs, and let `expr.LIST` take plain values ([#89](https://github.com/grindlemire/go-lucene/pull/89), [afba164](https://github.com/grindlemire/go-lucene/commit/afba1647e930ab3e781805f5fd1a7eff8405b4ad), [88676ef](https://github.com/grindlemire/go-lucene/commit/88676ef5f40e4d57a05bd29a00bb5e4b52ea05bb))
* spread a hand-built field group over its terms when rendering ([#86](https://github.com/grindlemire/go-lucene/pull/86), [22f77fd](https://github.com/grindlemire/go-lucene/commit/22f77fd0d822c002c41eb579197ef8c6f714d39f), [cbf25c1](https://github.com/grindlemire/go-lucene/commit/cbf25c18de2dfc314b5c810c8d8abf5b22c1b0e5))
* return nil from `WrapDefaultField` for a nil expression ([#79](https://github.com/grindlemire/go-lucene/pull/79), [6942ecb](https://github.com/grindlemire/go-lucene/commit/6942ecb6f1e3339f128c4432d9decd2a94e00bba))
* keep literals, wildcards, regexes and number spellings the same through a JSON round trip ([#82](https://github.com/grindlemire/go-lucene/pull/82), [#83](https://github.com/grindlemire/go-lucene/pull/83), [#90](https://github.com/grindlemire/go-lucene/pull/90), [2ba90e6](https://github.com/grindlemire/go-lucene/commit/2ba90e6a8fe357b27a07310208d200abcc2b1f2f), [df84f77](https://github.com/grindlemire/go-lucene/commit/df84f777ce387bcf4141b58f6737d3016475f62f), [bf73d35](https://github.com/grindlemire/go-lucene/commit/bf73d357627f65bece4bf59716c58cf02cac5f1d))
* keep the digits of a bare JSON number, and read a null field as no field ([#86](https://github.com/grindlemire/go-lucene/pull/86), [#90](https://github.com/grindlemire/go-lucene/pull/90), [35b98d2](https://github.com/grindlemire/go-lucene/commit/35b98d2c114d546589b867b4a68bf72397378b22), [b25a1f7](https://github.com/grindlemire/go-lucene/commit/b25a1f7accda4049bda0723a370cc0b384bdc394))

#### String()

* print Lucene syntax that parses back to the same query ([#76](https://github.com/grindlemire/go-lucene/pull/76), [0510b6b](https://github.com/grindlemire/go-lucene/commit/0510b6b1013bc7120564bacefe7c8ed4f0deae78))
* write only the parentheses that are needed, and keep the ones around field groups, negated numbers and nested prefix, boost and fuzzy operands ([#65](https://github.com/grindlemire/go-lucene/pull/65), [#76](https://github.com/grindlemire/go-lucene/pull/76), [782e916](https://github.com/grindlemire/go-lucene/commit/782e9167b0e43e16d9f8c443020610217c15b893), [bf8466f](https://github.com/grindlemire/go-lucene/commit/bf8466f03f45dbcd25013785e3e022e281108a13), [21e28f2](https://github.com/grindlemire/go-lucene/commit/21e28f284312514be04e04f2e5b457025bc9dfe6), [3ebb04f](https://github.com/grindlemire/go-lucene/commit/3ebb04fdece12495c3513a0a2f71db8b5901319b))
* quote terms that wouldn't parse back as the same term, including a `NULL` field name and a JSON-path field that isn't one word ([#76](https://github.com/grindlemire/go-lucene/pull/76), [#84](https://github.com/grindlemire/go-lucene/pull/84), [#87](https://github.com/grindlemire/go-lucene/pull/87), [bccb403](https://github.com/grindlemire/go-lucene/commit/bccb4033fd88f621ca3b61b7f961fe747eb95fa8), [134b8ec](https://github.com/grindlemire/go-lucene/commit/134b8eca362fdfc9c4e08078256f6d1ecaeb6fb3), [9cc4eb1](https://github.com/grindlemire/go-lucene/commit/9cc4eb16c37294fc95084444f56a4d1e80d2153e), [a5aa91b](https://github.com/grindlemire/go-lucene/commit/a5aa91b9c7582949507e24158b9a10fc69d1dbc9))
* print comparisons, wildcards, regexes and IN lists as Lucene syntax, and print wildcards and regexes built in code as terms that parse back ([#76](https://github.com/grindlemire/go-lucene/pull/76), [#83](https://github.com/grindlemire/go-lucene/pull/83), [81f854d](https://github.com/grindlemire/go-lucene/commit/81f854db3e8bf7006766b0a7475a1f053d4e5f34), [7547ce3](https://github.com/grindlemire/go-lucene/commit/7547ce3b68915c3c680adfea38aa42c50790dce9), [273e95c](https://github.com/grindlemire/go-lucene/commit/273e95cf212b51fd32014dd2bdbff52bef4ad5fc), [97b0d29](https://github.com/grindlemire/go-lucene/commit/97b0d2981def9cd31d7f07cd43c605ff8e435eb1), [96649cf](https://github.com/grindlemire/go-lucene/commit/96649cfa9853f36e47465fc5c84d2675aaf153e8))
* print a number as written, a float as a float, and the exact boost power and fuzzy distance ([#66](https://github.com/grindlemire/go-lucene/pull/66), [#76](https://github.com/grindlemire/go-lucene/pull/76), [737eebd](https://github.com/grindlemire/go-lucene/commit/737eebd312872aea79de4c3265d41a4c456041ac), [3132e5e](https://github.com/grindlemire/go-lucene/commit/3132e5e75585d3380cfa460409809100c0723793), [444bc5c](https://github.com/grindlemire/go-lucene/commit/444bc5cfceef81bcf8d290dabbaa3b955edde5f6))
* print a range with no field without a `<nil>` prefix in `%#v` ([#78](https://github.com/grindlemire/go-lucene/pull/78), [c3ca8da](https://github.com/grindlemire/go-lucene/commit/c3ca8da0aed3c3e0946419d6658770edaef6dce2))

### Documentation

* JSON fields, case-insensitive fields, and folding case on a JSON key ([#73](https://github.com/grindlemire/go-lucene/pull/73), [#87](https://github.com/grindlemire/go-lucene/pull/87), [d6b8061](https://github.com/grindlemire/go-lucene/commit/d6b8061deca5acd1de78f6d33b7a86e22103de5c), [e7241b4](https://github.com/grindlemire/go-lucene/commit/e7241b4cecfc030063479e5471194566dde6e349), [68bdbb1](https://github.com/grindlemire/go-lucene/commit/68bdbb1109735dbcd0b7b2815095097d9490eab3))
* backslashes, escaped wildcard characters and `expr.EscapeWildcard` in patterns ([#82](https://github.com/grindlemire/go-lucene/pull/82), [#83](https://github.com/grindlemire/go-lucene/pull/83), [c1cb8cf](https://github.com/grindlemire/go-lucene/commit/c1cb8cfc72e15f851c766dd0c5c7c6b18ccecb8d), [3600b7a](https://github.com/grindlemire/go-lucene/commit/3600b7a568734bd65d626bc3ea8885abc089bb68), [f1eabf5](https://github.com/grindlemire/go-lucene/commit/f1eabf5f01f86fb8720ea60f75b7db5c4cd73b1c))
* which terms parse as numbers, and how numbers a float64 can't hold render ([#75](https://github.com/grindlemire/go-lucene/pull/75), [#85](https://github.com/grindlemire/go-lucene/pull/85), [0380e4f](https://github.com/grindlemire/go-lucene/commit/0380e4ff86fa1710f07a1cb33d1d6d3ab6db60ff), [a324b59](https://github.com/grindlemire/go-lucene/commit/a324b59cd7efeef410c4f69e16068199c98b9843))
* how the default field and field groups apply, including groups built in code ([#78](https://github.com/grindlemire/go-lucene/pull/78), [#79](https://github.com/grindlemire/go-lucene/pull/79), [#86](https://github.com/grindlemire/go-lucene/pull/86), [6eeffe4](https://github.com/grindlemire/go-lucene/commit/6eeffe4f15e6e56aa6ebde890fa1b5ddf7d3668f), [aeda48a](https://github.com/grindlemire/go-lucene/commit/aeda48ac8f946d6af7a330c4c9790fb75cee1b34), [5e4f721](https://github.com/grindlemire/go-lucene/commit/5e4f72110e01d60f3c538618d5be22f181c13a26), [9c1dac9](https://github.com/grindlemire/go-lucene/commit/9c1dac9c491da2a53a16d65fe4ae18b994d09a5b))
* how a dotted column prints, and how `String()` handles a dot in a JSON-path name ([#87](https://github.com/grindlemire/go-lucene/pull/87), [#92](https://github.com/grindlemire/go-lucene/pull/92), [016180e](https://github.com/grindlemire/go-lucene/commit/016180e90a9a7b42b6c4c394484be8e2a70030d0), [9e1aad6](https://github.com/grindlemire/go-lucene/commit/9e1aad6539d3357fa128a8128ada32888bff7808))
* `+` and `-` inside a term, and a `+` in a wildcard ([#84](https://github.com/grindlemire/go-lucene/pull/84), [f36fb85](https://github.com/grindlemire/go-lucene/commit/f36fb855bd9e4545204962a6f5eaa01872e60c66), [b111fd8](https://github.com/grindlemire/go-lucene/commit/b111fd8d26527c5a21a575c791cfb7102d222d2c))
* MySQL output without parameters assumes utf8mb4 ([#72](https://github.com/grindlemire/go-lucene/pull/72), [659687e](https://github.com/grindlemire/go-lucene/commit/659687e708427c5085f36b3f0837b9576a6a5602))
* `PrepareLikePattern` now gets the raw pattern ([#72](https://github.com/grindlemire/go-lucene/pull/72), [8f63ef9](https://github.com/grindlemire/go-lucene/commit/8f63ef9fd6468590aeb6b741f681cadbef9b18fe))

## [0.2.1](https://github.com/grindlemire/go-lucene/compare/v0.2.0...v0.2.1) (2026-07-15)


### Bug Fixes

* escape embedded quotes when rendering literals back to Lucene syntax ([cc5f8a6](https://github.com/grindlemire/go-lucene/commit/cc5f8a690dc5eb6322b886d6d5de3555b9695139))
* honor backslash-escaped quotes inside phrases ([56786e1](https://github.com/grindlemire/go-lucene/commit/56786e126899d49553e75ec500f143f13553c193))
* honor backslash-escaped quotes inside phrases ([d124016](https://github.com/grindlemire/go-lucene/commit/d1240164484566373014b6a2d4c02af394e5fca0)), closes [#59](https://github.com/grindlemire/go-lucene/issues/59)
* scope quote-escaping in renderLiteral to plain literals only ([bc62f13](https://github.com/grindlemire/go-lucene/commit/bc62f13c019c68b9c8165f7383521d97e03837fc))
* strip delimiters and unescape single-quoted phrases too ([c2d6ee3](https://github.com/grindlemire/go-lucene/commit/c2d6ee309f15af928a4e221456be73f94d7d80fa))

## [0.2.0](https://github.com/grindlemire/go-lucene/compare/v0.1.0...v0.2.0) (2026-05-13)


### Features

* add Null operator constant and NULL() constructor ([514d410](https://github.com/grindlemire/go-lucene/commit/514d4101b259fd0d49037bd653a3e61c4fadeda6))
* marshal/unmarshal Null expressions as JSON null ([51fcea8](https://github.com/grindlemire/go-lucene/commit/51fcea8de417249976b8c9c1a5ea73e315498917))
* NOT/-field:null renders as IS NOT NULL ([34222aa](https://github.com/grindlemire/go-lucene/commit/34222aa4c6adb7b130528faac99451e9d6328e85))
* parser recognizes bare null as typed null literal ([7458431](https://github.com/grindlemire/go-lucene/commit/745843185d94e2268a182c36267d260e3001bbc6))
* partition null members out of IN-list rendering ([b5c06a8](https://github.com/grindlemire/go-lucene/commit/b5c06a891197caac1c842c26e9e02442e4122e60))
* reducer folds null into IN-list alongside literals ([12fb54a](https://github.com/grindlemire/go-lucene/commit/12fb54a41a42990294f4443abdfe60e219a2ebf7))
* reject null in range bounds ([fb4e1f2](https://github.com/grindlemire/go-lucene/commit/fb4e1f26fc2e4209e9f309915f015b8b4e69301f))
* reject standalone Null at the renderer entrypoint ([5d490a2](https://github.com/grindlemire/go-lucene/commit/5d490a2bd5d787a3a03a1b75b9154ff11f3dc179))
* render Equals(field, Null) as IS NULL across SQL dialects ([4a81cab](https://github.com/grindlemire/go-lucene/commit/4a81cabb8ade2697cb299e00739b5baeba0b388a))
* validate Null nodes and accept Null in literal-expr checks ([40b8c30](https://github.com/grindlemire/go-lucene/commit/40b8c306bc38fa50173e988d880105c56d3354d6))


### Bug Fixes

* literalToExpr panic on empty string ([311e75b](https://github.com/grindlemire/go-lucene/commit/311e75b1abaf84e1ca84f19f4e92af262dfe80e1))
* validate utf8/null bytes for standalone Regexp renders ([1b175f9](https://github.com/grindlemire/go-lucene/commit/1b175f9a9ac407e163efa39846a77794a8720c71))


### Documentation

* comment why IN-partition hand-rolls the List expression ([d612e08](https://github.com/grindlemire/go-lucene/commit/d612e08a16ed4b6dc387f84f72841f5b460d2a5d))

## [0.1.0](https://github.com/grindlemire/go-lucene/compare/v0.0.33...v0.1.0) (2026-04-23)


### Features

* add MySQL driver ([f80d7ac](https://github.com/grindlemire/go-lucene/commit/f80d7ac963eff48f82c16bb9c8e554e3657ec100))
* add SQLite driver with GLOB wildcards and REGEXP regex ([381bbc1](https://github.com/grindlemire/go-lucene/commit/381bbc14e93fb20676e446698dec5048023b3ac3))
* MySQL and MariaDB driver support ([1fd35e2](https://github.com/grindlemire/go-lucene/commit/1fd35e2c6e5ad045f3fe22c95dd9744530cc6c24))


### Bug Fixes

* exclusive string ranges, SQLite bool params, and test coverage gaps ([718d798](https://github.com/grindlemire/go-lucene/commit/718d7987c4d3359303614ab7d11fee3a333bce7b))
* open-ended float ranges hit BETWEEN fallback ([778e2a4](https://github.com/grindlemire/go-lucene/commit/778e2a44a749527dd632b40d2d48c3b4261b4623))
* route standalone wildcard through dialect in Base.Render ([003d7d2](https://github.com/grindlemire/go-lucene/commit/003d7d2be378be9e22f8dbb1ae87447cca6c7862))
* use capturing group in MySQL regex fallback for 5.7 compatibility ([b0908ad](https://github.com/grindlemire/go-lucene/commit/b0908ad345aa2882909b2331e3d578379c4d9318))


### Miscellaneous

* gitignore local design docs under docs/plans/ ([44854d0](https://github.com/grindlemire/go-lucene/commit/44854d074f8d4a28eedaad984bf240346489dcba))


### Documentation

* add missing imports and safer error handling to regex example ([50dc896](https://github.com/grindlemire/go-lucene/commit/50dc896ef0959f7cf0b84c3cb36792acd5e2b6be))
* clarify GLOB no-escape limitation in SQLite driver ([33982ad](https://github.com/grindlemire/go-lucene/commit/33982ad7432d3a98eba23d1ce4ff753472a6ba6c))
* correct string_range comment in sqlite integration tests ([e921ff7](https://github.com/grindlemire/go-lucene/commit/e921ff7bd74c804d43fa1745ebb71458c346026a))
* document Dialect fallback in Custom SQL Drivers section ([2ace694](https://github.com/grindlemire/go-lucene/commit/2ace694af72b1aa15a9ef491d3b7e47394d5f777))
* document SQLite driver and its differences from Postgres ([3a42c5d](https://github.com/grindlemire/go-lucene/commit/3a42c5d78219953a83643c2feb62aa6fdd049575))
* document the MySQL driver ([e67ec3b](https://github.com/grindlemire/go-lucene/commit/e67ec3bca2a8a733fb81039c7e2ab2402dc1f522))
* explain why expr.Regexp stays in Shared after Dialect refactor ([79bcbc0](https://github.com/grindlemire/go-lucene/commit/79bcbc0e9ce2118e0751eb34dfc7b5d3ed499ebd))
* humanize SQLite readme section ([40d653d](https://github.com/grindlemire/go-lucene/commit/40d653da561b837a968e5be643c61a7e04236016))
* rewrite README with clearer structure and examples ([69f96db](https://github.com/grindlemire/go-lucene/commit/69f96db142ca7f657916e26912a78415d301b752))
* update README for MariaDB and current regex fallback ([dd20eab](https://github.com/grindlemire/go-lucene/commit/dd20eab7c78cd25b080a560f6b682ae305ac7d3c))
