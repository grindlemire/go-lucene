# Changelog

## [0.3.0](https://github.com/grindlemire/go-lucene/compare/v0.2.1...v0.3.0) (2026-11-09)


### ⚠ BREAKING CHANGES

* a backslash in a pattern built in code, with expr.LIKE or expr.WILD or read from JSON, now makes the next character literal in every dialect. MySQL and SQLite used to match it as a literal backslash, so expr.LIKE("v", `C:\Users\*`) matched text starting with C:\Users\, and it now matches only the exact text C:Users*. Write \\ for a literal backslash, or pass text through expr.EscapeWildcard.

### Features

* case-insensitive matching for listed fields ([e271ecd](https://github.com/grindlemire/go-lucene/commit/e271ecd2de56a33c2884d9069fe4c12799329874))
* case-insensitive matching for listed fields ([6766bbe](https://github.com/grindlemire/go-lucene/commit/6766bbe5e3beac7c83c9d02d49dd00669561906c))
* render JSON sub-field paths in SQL dialects ([df7fb9f](https://github.com/grindlemire/go-lucene/commit/df7fb9fa1b77a0ed59c10322c329e93bc1fee797))
* render JSON sub-field paths in SQL dialects ([2b1d354](https://github.com/grindlemire/go-lucene/commit/2b1d354a6d40fe75143542000f21be1499727f73))


### Bug Fixes

* add expr.EscapeWildcard for text in a pattern built in code ([3bae41e](https://github.com/grindlemire/go-lucene/commit/3bae41e64b87c432da233020908cdf6b57484b40))
* apply a field group's field to every term inside it ([29fc510](https://github.com/grindlemire/go-lucene/commit/29fc510b35b6170e47b16c7789c7378e3446d6b7))
* apply a field group's field to every term inside it ([027b0d0](https://github.com/grindlemire/go-lucene/commit/027b0d0b98e493a5f503e6628f2f3f0ef1ab18cf))
* check a number's text where the driver writes it into the SQL ([543894d](https://github.com/grindlemire/go-lucene/commit/543894ddfe2aea9205d53ea94d1e3d9f2f96dbbb))
* check missing operands inside groups and LISTs too ([88676ef](https://github.com/grindlemire/go-lucene/commit/88676ef5f40e4d57a05bd29a00bb5e4b52ea05bb))
* compare against column range bounds in parameterized mode ([945e126](https://github.com/grindlemire/go-lucene/commit/945e12694fe25904a21c9fc23fececb54303dbd7))
* compare numbers a float64 can't hold as written ([495ecfa](https://github.com/grindlemire/go-lucene/commit/495ecfab9fabca7401cec88e936edabf38502502))
* drop a number's text once its value no longer equals it ([7b38388](https://github.com/grindlemire/go-lucene/commit/7b383889871ab96392c20b2f219c62fc755be1e2))
* drop the escapes from a wildcard used as a field ([4680ea0](https://github.com/grindlemire/go-lucene/commit/4680ea0f7ae4450ba16c72dca29d8ef153c92551))
* drop the escapes from a wildcard used as a field ([4ad235c](https://github.com/grindlemire/go-lucene/commit/4ad235c357df333d39c7f280993f1c539e8bbf88))
* give each term of a hand-built field group its field when rendering ([cbf25c1](https://github.com/grindlemire/go-lucene/commit/cbf25c18de2dfc314b5c810c8d8abf5b22c1b0e5))
* give the default field to wildcards, regexes, nulls and ranges ([8a0234d](https://github.com/grindlemire/go-lucene/commit/8a0234d8d9115caa531dd4abf91596249bd64c7e))
* give the default field to wildcards, regexes, nulls and ranges ([b23ff55](https://github.com/grindlemire/go-lucene/commit/b23ff558af116b527a39d2f290f1d2f0cb077c57))
* insert implicit AND before a parenthesised group ([00d1e36](https://github.com/grindlemire/go-lucene/commit/00d1e367c8f60fcdd369c9d630f4e37295f2b523))
* insert implicit AND before a parenthesised group ([2297401](https://github.com/grindlemire/go-lucene/commit/2297401282d89db82785a329f2a5c33100aa821a))
* keep a + at the end of the query out of the number before it ([a47116a](https://github.com/grindlemire/go-lucene/commit/a47116ab55b20a105fe9eaf33094705de567ea94))
* keep a + inside a term in the term ([4ccce75](https://github.com/grindlemire/go-lucene/commit/4ccce759f35eb5e96760ee13802c9719d7d86720))
* keep a + inside a term in the term ([59940ac](https://github.com/grindlemire/go-lucene/commit/59940aca93e7a326711641433e2ab33492003487))
* keep a literal * or ? a literal through JSON ([2ba90e6](https://github.com/grindlemire/go-lucene/commit/2ba90e6a8fe357b27a07310208d200abcc2b1f2f))
* keep a nonzero number too small for a float64 as text ([a1aea78](https://github.com/grindlemire/go-lucene/commit/a1aea78755e1a0c74be19395f488a722eed63252))
* keep a number's written text and let plain-text JSON matches use an index ([eb181c6](https://github.com/grindlemire/go-lucene/commit/eb181c6053690e4083e948b9b01692bd10ab3d9e))
* keep a number's written text through JSON and compare a bool as text ([5d64fa5](https://github.com/grindlemire/go-lucene/commit/5d64fa59772421e329598416b850ba3dcf2ff8af))
* keep a wildcard or regex built in code the same kind through JSON ([df84f77](https://github.com/grindlemire/go-lucene/commit/df84f777ce387bcf4141b58f6737d3016475f62f))
* keep an escaped backslash in a term ([685a33a](https://github.com/grindlemire/go-lucene/commit/685a33a0ca3d62538db5ecc2ee87b7d83dce7e94))
* keep escapes inside a bracket expression of a pattern built in code ([acff425](https://github.com/grindlemire/go-lucene/commit/acff425da10c084f23c35c7591297e923d4c05bf))
* keep floats as floats in String() ([3132e5e](https://github.com/grindlemire/go-lucene/commit/3132e5e75585d3380cfa460409809100c0723793))
* keep only a valid number spelling as written text ([4dc07b7](https://github.com/grindlemire/go-lucene/commit/4dc07b7d1e51e9439a476c166af25591e679cd9d))
* keep parentheses around grouped expressions in String() ([cfe07c4](https://github.com/grindlemire/go-lucene/commit/cfe07c4c760c206f86c15f3af0afbb756c7c7034))
* keep parentheses around grouped expressions in String() ([bf8466f](https://github.com/grindlemire/go-lucene/commit/bf8466f03f45dbcd25013785e3e022e281108a13))
* keep parentheses around nested prefix, boost and fuzzy operands ([21e28f2](https://github.com/grindlemire/go-lucene/commit/21e28f284312514be04e04f2e5b457025bc9dfe6))
* keep the + of an exponent literal in a wildcard ([aca5477](https://github.com/grindlemire/go-lucene/commit/aca5477a67fc6585f00eb9e31a04f64201edbda6))
* keep the default field out of field groups ([d5bc651](https://github.com/grindlemire/go-lucene/commit/d5bc65185a10ad290feb5accf43f52d434227ed3))
* keep the default field out of field groups ([360a378](https://github.com/grindlemire/go-lucene/commit/360a378a7a5cb9dc2cbc9a158e3d6fc03b16e6fe)), closes [#67](https://github.com/grindlemire/go-lucene/issues/67)
* keep the digits of a bare JSON number ([3538d19](https://github.com/grindlemire/go-lucene/commit/3538d192367926b1f041e10c4e2f4b40fe689302))
* keep the digits of a bare JSON number ([35b98d2](https://github.com/grindlemire/go-lucene/commit/35b98d2c114d546589b867b4a68bf72397378b22))
* keep the parentheses around field groups and negated numbers ([3ebb04f](https://github.com/grindlemire/go-lucene/commit/3ebb04fdece12495c3513a0a2f71db8b5901319b))
* lex 1E+3 and .5 as numbers and reject a trailing backslash ([11e60a8](https://github.com/grindlemire/go-lucene/commit/11e60a88039ca24ab55aa6829fcc799a72491f75))
* lex numbers written as 1E+3 or .5 ([adc9eff](https://github.com/grindlemire/go-lucene/commit/adc9eff527825fb771bec48606184ebeaae0755c))
* make JSON path comparisons type-safe and round-trip in JSON ([9e25fc6](https://github.com/grindlemire/go-lucene/commit/9e25fc67003fc619c1237e40987b6c81a1016764))
* make String() print Lucene syntax that parses back ([0510b6b](https://github.com/grindlemire/go-lucene/commit/0510b6b1013bc7120564bacefe7c8ed4f0deae78))
* match a case-insensitive JSON path by its plain dotted name ([6a0d2cf](https://github.com/grindlemire/go-lucene/commit/6a0d2cf5880c5eb324feaf7a71a307700449f90c))
* match JSON numbers stored as text and check paths at parse time ([4884543](https://github.com/grindlemire/go-lucene/commit/488454393ee6bf9f3de9e39326f3b691f95b7528))
* name a JSON path with an empty key by its plain dotted name ([af714b9](https://github.com/grindlemire/go-lucene/commit/af714b9099d89e2b60ae24f5e8951769e5190c7a))
* print a number's written text in String() ([737eebd](https://github.com/grindlemire/go-lucene/commit/737eebd312872aea79de4c3265d41a4c456041ac))
* print a range with no field without a &lt;nil&gt; prefix in %#v ([c3ca8da](https://github.com/grindlemire/go-lucene/commit/c3ca8da0aed3c3e0946419d6658770edaef6dce2))
* print a regex built in code as a /.../ term ([97b0d29](https://github.com/grindlemire/go-lucene/commit/97b0d2981def9cd31d7f07cd43c605ff8e435eb1))
* print a term with + after its first character without quotes ([a5aa91b](https://github.com/grindlemire/go-lucene/commit/a5aa91b9c7582949507e24158b9a10fc69d1dbc9))
* print a wildcard built in code as one term ([273e95c](https://github.com/grindlemire/go-lucene/commit/273e95cf212b51fd32014dd2bdbff52bef4ad5fc))
* print comparisons, wildcards, regexes and IN lists as Lucene syntax ([81f854d](https://github.com/grindlemire/go-lucene/commit/81f854db3e8bf7006766b0a7475a1f053d4e5f34))
* print the exact boost power and fuzzy distance in String() ([444bc5c](https://github.com/grindlemire/go-lucene/commit/444bc5cfceef81bcf8d290dabbaa3b955edde5f6))
* print the literal on the right of a LIKE as a wildcard term ([96649cf](https://github.com/grindlemire/go-lucene/commit/96649cfa9853f36e47465fc5c84d2675aaf153e8))
* print wildcards and regexes built in code as terms that parse back ([7547ce3](https://github.com/grindlemire/go-lucene/commit/7547ce3b68915c3c680adfea38aa42c50790dce9))
* quote a JSON-path field in String() when it is not one word ([63ccab1](https://github.com/grindlemire/go-lucene/commit/63ccab19ccf869cb02da692eee7968f9dcab5c72))
* quote a JSON-path field in String() when it is not one word ([9cc4eb1](https://github.com/grindlemire/go-lucene/commit/9cc4eb16c37294fc95084444f56a4d1e80d2153e))
* quote a NULL or Null field name in String() ([134b8ec](https://github.com/grindlemire/go-lucene/commit/134b8eca362fdfc9c4e08078256f6d1ecaeb6fb3))
* quote terms in String() that would not parse back as the same term ([bccb403](https://github.com/grindlemire/go-lucene/commit/bccb4033fd88f621ca3b61b7f961fe747eb95fa8))
* read a fuzzy distance only from an unquoted number ([03b7f98](https://github.com/grindlemire/go-lucene/commit/03b7f987cca298085feabc11a79bf3f38ee69257))
* read a null field from JSON as no field ([b25a1f7](https://github.com/grindlemire/go-lucene/commit/b25a1f7accda4049bda0723a370cc0b384bdc394))
* reject a backslash at the end of the query ([3fdd1a5](https://github.com/grindlemire/go-lucene/commit/3fdd1a535007753c8d20ca767db48effef2946d4))
* reject a comparison against a group or a range ([b3c1160](https://github.com/grindlemire/go-lucene/commit/b3c11600ad684f99521193ec1e0783feb1a8f545))
* reject a field operator that has no field ([9dd67e1](https://github.com/grindlemire/go-lucene/commit/9dd67e1fbf76b168134b535cbc38487716b72115))
* reject a group where a field operator needs a single term ([a7e08df](https://github.com/grindlemire/go-lucene/commit/a7e08df9e895c59b9410b418b96e54ee5ed61a09))
* reject an escaped dot in a JSON field before = and inside parentheses ([fb0cbb0](https://github.com/grindlemire/go-lucene/commit/fb0cbb029f55d9bd4ae6834cec23ceebd7857b40))
* reject compound range bounds and unescaped values in generated SQL ([a945779](https://github.com/grindlemire/go-lucene/commit/a945779684713f581e96b680b3a13f7270f296d6))
* reject compound range bounds and unescaped values in generated SQL ([f4a97e0](https://github.com/grindlemire/go-lucene/commit/f4a97e0a3cb6fa3778b8b220f234dcc4adfd0d0b))
* report missing operands in hand-built trees instead of panicking ([7b96454](https://github.com/grindlemire/go-lucene/commit/7b964546ac85235c40594a7390d01d142586439e))
* report missing operands in hand-built trees instead of panicking ([afba164](https://github.com/grindlemire/go-lucene/commit/afba1647e930ab3e781805f5fd1a7eff8405b4ad))
* report the lexer's error when a query fails to lex ([f9b6cb9](https://github.com/grindlemire/go-lucene/commit/f9b6cb91d3298fd70b481477d763fde980b5ebfe))
* require a single term in a comparison, IN list item and field ([213e323](https://github.com/grindlemire/go-lucene/commit/213e323b1325c0f3519099f62a266e9082fb2d15))
* return nil from WrapDefaultField for a nil expression ([6942ecb](https://github.com/grindlemire/go-lucene/commit/6942ecb6f1e3339f128c4432d9decd2a94e00bba))
* spread a hand-built field group over its terms when rendering ([22f77fd](https://github.com/grindlemire/go-lucene/commit/22f77fd0d822c002c41eb579197ef8c6f714d39f))
* treat a number used as a field as a column with = and in parentheses ([9a94f67](https://github.com/grindlemire/go-lucene/commit/9a94f67bccba710ea4f00c7902ff3e02d0287c63))
* treat a number used as a field as a column with = and in parentheses ([172d8f2](https://github.com/grindlemire/go-lucene/commit/172d8f217a01848fd087aa43b51c9e1f99d00917))
* treat an escaped * or ? as a literal character ([315512f](https://github.com/grindlemire/go-lucene/commit/315512f1aa0541036e191b67c58946df1530dd38))
* treat only a bare * as an open range bound ([f7660ba](https://github.com/grindlemire/go-lucene/commit/f7660ba9b5144d1c2f5c697a7325c2f3e584dc9d))
* treat only plain decimal numbers as numbers ([c66255c](https://github.com/grindlemire/go-lucene/commit/c66255ca64dda996fd37938eebf4e46d8c90a087))
* treat only plain decimal numbers as numbers ([a971d24](https://github.com/grindlemire/go-lucene/commit/a971d2434162c0d3dd4cb3956e3d4a805c8ec7b3))
* write a float without kept text in plain decimal form in JSON ([bf73d35](https://github.com/grindlemire/go-lucene/commit/bf73d357627f65bece4bf59716c58cf02cac5f1d))
* write a number a float64 rounds into the SQL as written ([ad87b18](https://github.com/grindlemire/go-lucene/commit/ad87b183c81d8b8b6a562e53818b05c52e2ec602))
* write only the parentheses String() needs ([782e916](https://github.com/grindlemire/go-lucene/commit/782e9167b0e43e16d9f8c443020610217c15b893))


### Miscellaneous

* bump the minor version for a breaking change before 1.0 ([0b659a2](https://github.com/grindlemire/go-lucene/commit/0b659a263704c4e09410e7885277cddc00888743))
* bump the minor version for a breaking change before 1.0 ([568ffb5](https://github.com/grindlemire/go-lucene/commit/568ffb58b3213e1c4268b42669dc4373331f21fc))
* keep the release config's original formatting ([49b0c17](https://github.com/grindlemire/go-lucene/commit/49b0c17a9d2fee591335f22022b68d0d2350e3b8))


### Documentation

* describe backslashes and expr.EscapeWildcard in patterns built in code ([c1cb8cf](https://github.com/grindlemire/go-lucene/commit/c1cb8cfc72e15f851c766dd0c5c7c6b18ccecb8d))
* describe escaped wildcard characters ([3600b7a](https://github.com/grindlemire/go-lucene/commit/3600b7a568734bd65d626bc3ea8885abc089bb68))
* describe wildcards built in code ([f1eabf5](https://github.com/grindlemire/go-lucene/commit/f1eabf5f01f86fb8720ea60f75b7db5c4cd73b1c))
* document case-insensitive fields ([d6b8061](https://github.com/grindlemire/go-lucene/commit/d6b8061deca5acd1de78f6d33b7a86e22103de5c))
* note that non-parameterized MySQL output assumes utf8mb4 ([659687e](https://github.com/grindlemire/go-lucene/commit/659687e708427c5085f36b3f0837b9576a6a5602))
* note that PrepareLikePattern now gets the raw pattern ([8f63ef9](https://github.com/grindlemire/go-lucene/commit/8f63ef9fd6468590aeb6b741f681cadbef9b18fe))
* say how a field group built in code renders ([aeda48a](https://github.com/grindlemire/go-lucene/commit/aeda48ac8f946d6af7a330c4c9790fb75cee1b34))
* say how numbers a float64 can't hold render ([a324b59](https://github.com/grindlemire/go-lucene/commit/a324b59cd7efeef410c4f69e16068199c98b9843))
* say how to fold case on a JSON key with a space ([e7241b4](https://github.com/grindlemire/go-lucene/commit/e7241b4cecfc030063479e5471194566dde6e349))
* say how to write a JSON key with a space or colon ([68bdbb1](https://github.com/grindlemire/go-lucene/commit/68bdbb1109735dbcd0b7b2815095097d9490eab3))
* say that + and - inside a term are part of it ([f36fb85](https://github.com/grindlemire/go-lucene/commit/f36fb855bd9e4545204962a6f5eaa01872e60c66))
* say that a + in a wildcard term is literal ([b111fd8](https://github.com/grindlemire/go-lucene/commit/b111fd8d26527c5a21a575c791cfb7102d222d2c))
* say that a dotted column prints as a JSON path field ([441553d](https://github.com/grindlemire/go-lucene/commit/441553df76722dc6b66e327bbd08a34e86b045a2))
* say that a dotted column prints as a JSON path field ([016180e](https://github.com/grindlemire/go-lucene/commit/016180e90a9a7b42b6c4c394484be8e2a70030d0))
* say that a hand-built EQUALS keeps a nested field and needs a field ([5e4f721](https://github.com/grindlemire/go-lucene/commit/5e4f72110e01d60f3c538618d5be22f181c13a26))
* say what String() does with a dot inside a JSON-path name ([9e1aad6](https://github.com/grindlemire/go-lucene/commit/9e1aad6539d3357fa128a8128ada32888bff7808))
* say which terms parse as numbers ([0380e4f](https://github.com/grindlemire/go-lucene/commit/0380e4ff86fa1710f07a1cb33d1d6d3ab6db60ff))
* say which terms the default field applies to ([6eeffe4](https://github.com/grindlemire/go-lucene/commit/6eeffe4f15e6e56aa6ebde890fa1b5ddf7d3668f))
* show a field group with AND and - in the operator reference ([9c1dac9](https://github.com/grindlemire/go-lucene/commit/9c1dac9c491da2a53a16d65fe4ae18b994d09a5b))

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
