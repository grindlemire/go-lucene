# Changelog

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
