# Changelog

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
