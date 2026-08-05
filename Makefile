SHELL := /bin/bash
.DEFAULT_GOAL := help

# Everything generated — the binary, the database, the asset store, coverage —
# lives under var/. One directory to ignore, one directory to delete.
VAR     := var
BINARY  := $(VAR)/bin/yt-studio
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
# Where the server will be listening. The Makefile needs this only to poll for
# readiness and to print a URL — it deliberately does not pass --listen, because
# a flag beats .env and would silently ignore what the file says. Read, never
# sourced: sourcing a configuration file executes whatever is in it.
ENV_LISTEN := $(shell grep -E '^[[:space:]]*(export[[:space:]]+)?YTS_LISTEN[[:space:]]*=' .env 2>/dev/null \
                | tail -1 | sed -e 's/.*=[[:space:]]*//' -e 's/[[:space:]]*$$//' | tr -d "\"'")
LISTEN  ?= $(or $(ENV_LISTEN),127.0.0.1:8080)
GOBIN   := $(shell go env GOPATH)/bin

.PHONY: help dev build run test bench lint fmt generate clean

## help: list the targets
help:
	@echo "yt-studio $(VERSION)"
	@echo
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## /  make /' | column -t -s ':'

## dev: hot-reload the server and the web UI together
dev: web/node_modules
	@test -x $(GOBIN)/air || go install github.com/air-verse/air@latest
	@trap 'kill 0' EXIT INT TERM; \
	$(GOBIN)/air -c .air.toml & \
	printf 'waiting for the server on $(LISTEN)'; \
	up=0; \
	for i in $$(seq 1 60); do \
		if curl -sf -o /dev/null http://$(LISTEN)/api/health; then up=1; break; fi; \
		printf '.'; sleep 0.5; \
	done; \
	echo; \
	if [ $$up -eq 1 ]; then \
		echo "server  http://$(LISTEN)"; \
	else \
		echo "server did not start - see the build output above (is $(LISTEN) already in use?)"; \
	fi; \
	echo "web ui  http://127.0.0.1:5173"; \
	npm --prefix web run dev & \
	wait || true

## build: build the single binary with the web UI embedded
build: web/node_modules
	npm --prefix web run build
	@mkdir -p $(VAR)/bin
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=$(VERSION)" -o $(BINARY) ./cmd/server
	@echo "$(BINARY)  $$(du -h $(BINARY) | cut -f1)"

## run: build, then serve - .env names the database, the assets and the address
run: build
	$(BINARY) serve

## test: every test, with the race detector
test:
	go test ./... -race -count=1 -timeout 900s

## bench: the performance budgets
bench:
	go test ./... -run '^$$' -bench . -benchmem -benchtime 200x

## lint: go vet, golangci-lint, and the web typecheck
lint: web/node_modules
	@command -v $(GOBIN)/golangci-lint >/dev/null || go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
	go vet ./...
	$(GOBIN)/golangci-lint run ./...
	npm --prefix web run typecheck
	@# sqlc's SQLite dialect truncates generated statements when a .sql file
	@# contains multi-byte characters. Keeping them ASCII is cheap.
	@! grep -rlP '[^\x00-\x7F]' adapters/sqlite/queries adapters/sqlite/migrations 2>/dev/null \
		|| { echo "non-ASCII in SQL corrupts sqlc output (see the files above)"; exit 1; }

## fmt: format Go and TypeScript
fmt: web/node_modules
	gofmt -w .
	npm --prefix web exec -- prettier --write --log-level warn "web/src/**/*.{ts,tsx,css}"

## generate: regenerate the sqlc query layer
generate:
	@command -v $(GOBIN)/sqlc >/dev/null || go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
	$(GOBIN)/sqlc generate
	go build ./...

## clean: delete everything generated
clean:
	rm -rf $(VAR) web/dist
	@mkdir -p web/dist && cp web/placeholder.html web/dist/index.html
	go clean -testcache

web/node_modules:
	npm --prefix web ci --no-audit --no-fund
