SHELL := /usr/bin/env bash
.DEFAULT_GOAL := help

GO ?= go
PNPM ?= pnpm

.PHONY: help bootstrap doctor clean test-repository

help:
	@printf '%s\n' \
		'Semlia development commands:' \
		'  make doctor          Check required tools and local capabilities' \
		'  make bootstrap       Install dependencies from lockfiles' \
		'  make test-repository Run the repository contract' \
		'  make clean           Remove generated local state'

bootstrap:
	$(GO) mod download
	$(PNPM) install --frozen-lockfile

doctor:
	@./scripts/doctor.sh

test-repository:
	$(GO) test ./tests/repository

clean:
	rm -rf node_modules dist build coverage .cache
	rm -f coverage.out
