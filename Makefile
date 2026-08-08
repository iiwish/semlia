SHELL := /usr/bin/env bash
.DEFAULT_GOAL := help

GO ?= go
PNPM ?= pnpm

.PHONY: help bootstrap contracts contracts-check doctor clean test-contracts test-repository

help:
	@printf '%s\n' \
		'Semlia development commands:' \
		'  make doctor          Check required tools and local capabilities' \
		'  make bootstrap       Install dependencies from lockfiles' \
		'  make contracts       Generate Go and TypeScript contract artifacts' \
		'  make contracts-check Verify committed contract artifacts are current' \
		'  make test-contracts  Run public contract tests' \
		'  make test-repository Run the repository contract' \
		'  make clean           Remove generated local state'

bootstrap:
	$(GO) mod download
	$(PNPM) install --frozen-lockfile

doctor:
	@./scripts/doctor.sh

contracts:
	@./scripts/generate-contracts.sh --write

contracts-check:
	@./scripts/generate-contracts.sh --check

test-contracts:
	$(GO) test ./tests/contracts/...

test-repository:
	$(GO) test ./tests/repository

clean:
	rm -rf node_modules dist build coverage .cache
	rm -f coverage.out
