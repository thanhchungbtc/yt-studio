SHELL := /bin/bash
.DEFAULT_GOAL := help

# Everything generated — the binary, the database, the asset store, coverage —
# lives under var/. One directory to ignore.
VAR     := var
# KEEP is the exception: var/resources is operator-supplied media, not output.
# It sits under var/ because it is equally untracked, but deleting it costs
# files that no build step can put back — bg.mp4 alone is a gigabyte that came
# from somewhere else. clean removes var/'s contents around it.
KEEP    := resources
BINARY  := $(VAR)/bin/yt-studio
DESKTOP := $(VAR)/bin/yt-studio-desktop
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
# Where the server will be listening, matching the binary's default. The
# Makefile needs this only to poll for readiness and to print a URL.
LISTEN  ?= 127.0.0.1:8080
GOBIN   := $(shell go env GOPATH)/bin
# The macOS bundle. APP_NAME is what the Finder shows and what the bundle is
# named; the identifier is what launchd and the keychain key off.
#
# Packaging output gets its own directory rather than sitting at the root of
# var/. `make dev` runs with --home var, so var/ is also the installation
# directory a development server reads and writes — the database and the asset
# store are down there. A bundle beside them reads as data, and `rm -rf
# var/desktop` should re-do the packaging without going anywhere near either.
APP_NAME := yt-studio
APP_ID   := com.tbui.yt-studio
APP_DIR  := $(VAR)/desktop
APP      := $(APP_DIR)/$(APP_NAME).app

.PHONY: help dev dev-desktop build desktop run test bench lint fmt generate clean

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

## dev-desktop: the same two processes as dev, in a native window
#
# The window is pointed at Vite rather than at the server, so hot reload works
# exactly as it does in the browser: this is dev with a different frame around
# it, not a different way of running the application.
dev-desktop: web/node_modules $(DESKTOP)
	@test -x $(GOBIN)/air || go install github.com/air-verse/air@latest
	@trap 'kill 0' EXIT INT TERM; \
	$(GOBIN)/air -c .air.toml & \
	printf 'waiting for the server on $(LISTEN)'; \
	for i in $$(seq 1 60); do \
		if curl -sf -o /dev/null http://$(LISTEN)/api/health; then break; fi; \
		printf '.'; sleep 0.5; \
	done; \
	echo; \
	npm --prefix web run dev & \
	printf 'waiting for vite on 127.0.0.1:5173'; \
	for i in $$(seq 1 60); do \
		if curl -sf -o /dev/null http://127.0.0.1:5173; then break; fi; \
		printf '.'; sleep 0.5; \
	done; \
	echo; \
	$(DESKTOP) --url http://127.0.0.1:5173; \
	kill 0

## build: build the single binary with the web UI embedded
build: web/node_modules
	npm --prefix web run build
	@mkdir -p $(VAR)/bin
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=$(VERSION)" -o $(BINARY) ./cmd/server
	@echo "$(BINARY)  $$(du -h $(BINARY) | cut -f1)"

# The window binary. cgo, unavoidably: it is a WKWebView. It is the one reason
# the window is a separate binary at all — the server stays CGO_ENABLED=0, and
# its pure-Go SQLite is why that is worth keeping.
$(DESKTOP): $(shell find cmd/desktop -name '*.go')
	@mkdir -p $(VAR)/bin
	CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o $(DESKTOP) ./cmd/desktop

## desktop: build yt-studio.app, a double-clickable bundle in var/
desktop: build $(DESKTOP)
	@rm -rf $(APP)
	@mkdir -p $(APP)/Contents/MacOS $(APP)/Contents/Resources
	@cp $(BINARY) $(APP)/Contents/MacOS/yt-studio
	@cp $(DESKTOP) $(APP)/Contents/MacOS/yt-studio-desktop
	@printf '%s\n' \
		'<?xml version="1.0" encoding="UTF-8"?>' \
		'<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">' \
		'<plist version="1.0">' \
		'<dict>' \
		'	<key>CFBundleName</key><string>$(APP_NAME)</string>' \
		'	<key>CFBundleDisplayName</key><string>yt-studio</string>' \
		'	<key>CFBundleIdentifier</key><string>$(APP_ID)</string>' \
		'	<key>CFBundleVersion</key><string>$(VERSION)</string>' \
		'	<key>CFBundleShortVersionString</key><string>$(VERSION)</string>' \
		'	<key>CFBundlePackageType</key><string>APPL</string>' \
		'	<key>CFBundleExecutable</key><string>yt-studio-desktop</string>' \
		'	<key>LSMinimumSystemVersion</key><string>11.0</string>' \
		'	<key>NSHighResolutionCapable</key><true/>' \
		'</dict>' \
		'</plist>' > $(APP)/Contents/Info.plist
	@# Ad-hoc, so Gatekeeper lets it run on this machine. Distributing it to
	@# another Mac needs a Developer ID signature and notarisation.
	@codesign --force --deep --sign - $(APP) 2>/dev/null || \
		echo "note: codesign failed; right-click > Open the first time"
	@echo "$(APP)  $$(du -sh $(APP) | cut -f1)"
	@echo
	@echo "  open $(APP)                  run it from here"
	@echo "  cp -r $(APP) /Applications/   install it"
	@echo
	@echo "It keeps its data in ~/.yt-studio, not in $(VAR). Copy the resources"
	@echo "over once - nothing renders without them:"
	@echo
	@echo "  mkdir -p ~/.yt-studio && cp -r $(VAR)/$(KEEP) ~/.yt-studio/"

## run: build, then serve - settings live in the database, not a config file
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

## clean: delete everything generated, keeping var/resources
clean:
	@if [ -d $(VAR) ]; then find $(VAR) -mindepth 1 -maxdepth 1 ! -name $(KEEP) -exec rm -rf {} +; fi
	rm -rf web/dist
	@mkdir -p web/dist && cp web/placeholder.html web/dist/index.html
	go clean -testcache

web/node_modules:
	npm --prefix web ci --no-audit --no-fund
