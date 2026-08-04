SHELL := /bin/bash
.DEFAULT_GOAL := help

# Everything generated — the binary, the database, the asset store, coverage —
# lives under var/. One directory to ignore, one directory to delete.
VAR     := var
BINARY  := $(VAR)/bin/yt-studio
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
# Where the daemon will be listening. The Makefile needs this only to poll for
# readiness and to print a URL — it deliberately does not pass --listen, because
# a flag beats .env and would silently ignore what the file says. Read, never
# sourced: sourcing a configuration file executes whatever is in it.
ENV_LISTEN := $(shell grep -E '^[[:space:]]*(export[[:space:]]+)?YTS_LISTEN[[:space:]]*=' .env 2>/dev/null \
                | tail -1 | sed -e 's/.*=[[:space:]]*//' -e 's/[[:space:]]*$$//' | tr -d "\"'")
LISTEN  ?= $(or $(ENV_LISTEN),127.0.0.1:8080)
GOBIN   := $(shell go env GOPATH)/bin

# The demo runs the mocks slowly on purpose. `mock.latency_ms` is scaled per
# task kind inside the providers (x1 for a clip, x4 for a blueprint), so 1000
# puts a single task between one and four seconds -- slow enough to watch a
# task go ready -> running -> done in the UI, fast enough that a whole video
# still finishes while you are looking at it.
DEMO_LATENCY ?= 1000
DEMO_FAILURES ?= 0
DEMO_CHAPTERS ?= 12

.PHONY: help dev build run demo test bench lint fmt generate clean

## help: list the targets
help:
	@echo "yt-studio $(VERSION)"
	@echo
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## /  make /' | column -t -s ':'

## dev: hot-reload the daemon and the web UI together
dev: web/node_modules
	@test -x $(GOBIN)/air || go install github.com/air-verse/air@latest
	@trap 'kill 0' EXIT INT TERM; \
	$(GOBIN)/air -c .air.toml & \
	printf 'waiting for the daemon on $(LISTEN)'; \
	up=0; \
	for i in $$(seq 1 60); do \
		if curl -sf -o /dev/null http://$(LISTEN)/api/health; then up=1; break; fi; \
		printf '.'; sleep 0.5; \
	done; \
	echo; \
	if [ $$up -eq 1 ]; then \
		echo "daemon  http://$(LISTEN)"; \
	else \
		echo "daemon did not start - see the build output above (is $(LISTEN) already in use?)"; \
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

## demo: serve with slow mocks and seeded videos, to watch the UI live
#
# Seeds two videos and lets the first through its blueprint gate, so there is
# work in flight the moment the browser opens. The second is left sitting at
# its gate, so the Approve button has something to do.
#
# Its own database and asset root arrive as environment variables rather than
# flags: an exported variable still beats .env, so the demo cannot clobber the
# real database, while a flag would also have overridden the address the file
# names.
demo: build
	@# Checked before the trap below is installed: a daemon that cannot bind
	@# would otherwise leave the seeding to hit whatever else is on the port.
	@if curl -sf -o /dev/null http://$(LISTEN)/api/health; then \
		echo "something is already serving on $(LISTEN) - stop it first"; exit 1; \
	fi
	@rm -rf $(VAR)/demo.db $(VAR)/demo.db-wal $(VAR)/demo.db-shm $(VAR)/demo-assets
	@trap 'kill 0' EXIT INT TERM; \
	set -e; \
	base=http://$(LISTEN); \
	YTS_DB=$(VAR)/demo.db YTS_ASSETS=$(VAR)/demo-assets $(BINARY) serve & \
	printf 'waiting for the daemon on $(LISTEN)'; \
	for i in $$(seq 1 60); do \
		if curl -sf -o /dev/null $$base/api/health; then break; fi; \
		printf '.'; sleep 0.5; \
	done; \
	echo; \
	curl -sf -o /dev/null -X PUT $$base/api/settings/mock.latency_ms \
		-H 'content-type: application/json' -d '{"value":"$(DEMO_LATENCY)"}'; \
	curl -sf -o /dev/null -X PUT $$base/api/settings/mock.failure_rate_percent \
		-H 'content-type: application/json' -d '{"value":"$(DEMO_FAILURES)"}'; \
	first=$$(curl -sf -X POST $$base/api/videos -H 'content-type: application/json' \
		-H 'Idempotency-Key: demo-1' \
		-d '{"channel":"deep-sleep-stories","title":"The Long Winter of the Harbour","topic":"a northern port town over one winter, told through its shipping ledgers","chapterCount":$(DEMO_CHAPTERS),"imagesPerChapter":2,"start":true}' \
		| sed -n 's/.*"ref":"\([^"]*\)".*/\1/p'); \
	second=$$(curl -sf -X POST $$base/api/videos -H 'content-type: application/json' \
		-H 'Idempotency-Key: demo-2' \
		-d '{"channel":"history-explained","title":"The Salt Roads of the Adriatic","topic":"the medieval salt trade and the towns it made","chapterCount":$(DEMO_CHAPTERS),"imagesPerChapter":2,"start":true}' \
		| sed -n 's/.*"ref":"\([^"]*\)".*/\1/p'); \
	sleep $$(( ($(DEMO_LATENCY) * 4) / 1000 + 2 )); \
	curl -sf -o /dev/null -X POST $$base/api/videos/$$first/approve \
		-H 'content-type: application/json' -d '{"gate":"blueprint"}'; \
	echo; \
	echo "  mocks     $(DEMO_LATENCY) ms per unit of work (x1 clip .. x4 blueprint), $(DEMO_FAILURES)% injected failures"; \
	echo "  seeded    $$first is running, $$second is waiting at its blueprint gate"; \
	echo; \
	echo "  open      $$base/videos"; \
	echo "  watch     tasks go ready -> running -> done; the pools fill in the status bar"; \
	echo "  try       approve $$second's gate, then cancel it mid-render"; \
	echo "  tune      make demo DEMO_LATENCY=3000   (slower)   DEMO_LATENCY=200 (faster)"; \
	echo "            make demo DEMO_FAILURES=15    (watch tasks fail and retry)"; \
	echo; \
	wait

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
