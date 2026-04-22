.PHONY: test integration integration-mysql integration-sqlite integration-matrix

# Unit tests for the main module. Fast, no containers.
test:
	go test ./...

# Integration tests: both drivers, default images. SQLite uses an in-process
# pure-Go database; MySQL spins up a container (Docker or Podman).
integration: integration-mysql integration-sqlite

# MySQL integration against a single image. Override with MYSQL_IMAGE=...
# Examples:
#   make integration-mysql
#   make integration-mysql MYSQL_IMAGE=mariadb:10.4
#   make integration-mysql MYSQL_IMAGE=mysql:5.7
MYSQL_IMAGE ?= mysql:8.0
integration-mysql:
	INTEGRATION=1 MYSQL_IMAGE=$(MYSQL_IMAGE) go test -v ./integration/mysql/...

integration-sqlite:
	INTEGRATION=1 go test -v ./integration/sqlite/...

# Run the MySQL suite against every engine family we care about:
#   mysql:8.0      -> ICU regex
#   mariadb:10.4   -> Henry Spencer POSIX regex (same family as MySQL 5.7)
#   mariadb:10.11  -> PCRE2 regex
# MySQL 5.7 is intentionally omitted because it has no arm64 image and
# segfaults under QEMU emulation on Apple Silicon; add it manually via
# `make integration-mysql MYSQL_IMAGE=mysql:5.7` on amd64 hosts or with
# Rosetta enabled.
integration-matrix:
	@for img in mysql:8.0 mariadb:10.4 mariadb:10.11; do \
		echo "=== $$img ==="; \
		INTEGRATION=1 MYSQL_IMAGE=$$img go test -v ./integration/mysql/... || exit $$?; \
	done
	INTEGRATION=1 go test -v ./integration/sqlite/...
