SHELL := /usr/bin/env bash
.DEFAULT_GOAL := help

GO ?= go
PNPM ?= pnpm

.PHONY: help bootstrap build build-image server web-embed web-embed-check dev dev-down smoke contracts contracts-check doctor clean test test-contracts test-repository db-generate db-generate-check db-test db-migrate-up db-migrate-down db-migrate-version format-check lint typecheck check-source check-smoke security-check check sbom release

help:
	@printf '%s\n' \
		'Semlia development commands:' \
		'  make doctor          Check required tools and local capabilities' \
		'  make bootstrap       Install dependencies from lockfiles' \
		'  make build           Build the Semlia control-plane binary' \
		'  make check           Run every required pull-request gate locally' \
		'  make check-source    Run source, test, drift, and build gates' \
		'  make check-smoke     Run the complete stack smoke gate and clean up' \
		'  make security-check  Scan dependencies, secrets, and release image' \
		'  make sbom            Generate a CycloneDX SBOM for the local binary' \
		'  make release         Build a versioned, checksummed release bundle' \
		'  make server          Run the Semlia control-plane server' \
		'  make dev             Start native Go server, worker and Vite with existing PostgreSQL' \
		'  make smoke           Verify the running local stack' \
		'  make dev-down        Stop owned native processes and preserve PostgreSQL/data' \
		'  make contracts       Generate Go and TypeScript contract artifacts' \
		'  make contracts-check Verify committed contract artifacts are current' \
		'  make db-generate      Generate typed PostgreSQL queries with sqlc' \
		'  make db-test          Run isolated PostgreSQL integration tests' \
		'  make db-migrate-up    Apply migrations to SEMLIA_DATABASE_URL' \
		'  make db-migrate-down  Revert migrations on SEMLIA_DATABASE_URL' \
		'  make test-contracts  Run public contract tests' \
		'  make test-repository Run the repository contract' \
		'  make clean           Remove generated local state'

bootstrap:
	$(GO) mod download
	$(PNPM) install --frozen-lockfile

doctor:
	@./scripts/doctor.sh

web-embed:
	@./scripts/dev/sync-web.sh

web-embed-check:
	@before="$$(find internal/platform/web/static -type f -exec cksum {} + | sort)"; \
	./scripts/dev/sync-web.sh; \
	after="$$(find internal/platform/web/static -type f -exec cksum {} + | sort)"; \
	test "$$before" = "$$after"

build: web-embed
	@mkdir -p build
	$(GO) build -o build/semlia ./cmd/semlia

build-image:
	docker build --file deploy/local/Dockerfile --tag semlia:security .

server: build
	./build/semlia server

dev:
	@node scripts/dev/native.mjs up

dev-down:
	@node scripts/dev/native.mjs down

smoke:
	@node scripts/dev/native.mjs status

contracts:
	@./scripts/generate-contracts.sh --write

contracts-check:
	@./scripts/generate-contracts.sh --check

db-generate:
	$(GO) tool sqlc generate -f db/sqlc.yaml

db-generate-check:
	@before="$$(find internal/adapters/postgres/sqlc -type f -exec cksum {} + | sort)"; \
	$(GO) tool sqlc generate -f db/sqlc.yaml; \
	after="$$(find internal/adapters/postgres/sqlc -type f -exec cksum {} + | sort)"; \
	test "$$before" = "$$after"

db-test:
	$(GO) test ./tests/integration/db/... ./tests/integration/worker/...

db-migrate-up:
	SEMLIA_MIGRATIONS_PATH="$(CURDIR)/migrations" $(GO) run ./cmd/semlia migrate up

db-migrate-down:
	SEMLIA_MIGRATIONS_PATH="$(CURDIR)/migrations" $(GO) run ./cmd/semlia migrate down

db-migrate-version:
	SEMLIA_MIGRATIONS_PATH="$(CURDIR)/migrations" $(GO) run ./cmd/semlia migrate version

test-contracts:
	$(GO) test ./tests/contracts/...

test-repository:
	$(GO) test ./tests/repository

format-check:
	@./scripts/ci/format-check.sh

lint:
	$(GO) vet ./...
	$(PNPM) -r --if-present lint

typecheck:
	$(PNPM) -r --if-present typecheck

test:
	$(GO) test ./...
	$(PNPM) -r --if-present test

check-source:
	@./scripts/ci/check-source.sh

.PHONY: browser-bootstrap check-browser
browser-bootstrap:
	$(PNPM) --dir web exec playwright install --with-deps chromium

check-browser:
	bash scripts/dev/browser-acceptance.sh

check-smoke:
	@./scripts/ci/check-smoke.sh

security-check: build-image
	@./scripts/ci/security-check.sh

check:
	$(MAKE) --no-print-directory check-source
	$(MAKE) --no-print-directory check-browser
	$(MAKE) --no-print-directory check-smoke
	$(MAKE) --no-print-directory security-check

sbom: build
	@./scripts/release/sbom.sh build/semlia build/release/semlia.sbom.cdx.json

release:
	@./scripts/release/build.sh

clean:
	rm -rf node_modules dist build coverage .cache
	rm -f coverage.out
