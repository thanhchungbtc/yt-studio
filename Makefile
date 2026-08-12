SHELL := /bin/bash
.DEFAULT_GOAL := help

# Everything untracked lives under var/, split by what it would cost to lose.
#
#   var/build/  output. Every file in it is reproducible from a command, so
#               clean deletes the whole directory without asking.
#   var/home/   the development installation, laid out exactly as ~/.yt-studio
#               is: db/, assets/, resources/, transcripts/, log/. `make dev`
#               passes --home var/home, so what a checkout runs against has the
#               same shape as what the bundle ships. clean does not touch it.
#
# The split is the safeguard. var/home/resources holds operator-supplied media
# no build step can put back — bg.mp4 alone is a gigabyte that came from
# somewhere else — and a clean that has to remember an exception is a clean that
# eventually forgets one.
VAR      := var
BUILD    := $(VAR)/build
DEV_HOME := $(VAR)/home
BINARY   := $(BUILD)/bin/yt-studio
DESKTOP  := $(BUILD)/bin/yt-studio-desktop
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
# Where the server will be listening, matching the binary's default. The
# Makefile needs this only to poll for readiness and to print a URL.
LISTEN   ?= 127.0.0.1:8080
GOBIN    := $(shell go env GOPATH)/bin
# The macOS bundle. APP_NAME is what the Finder shows and what the bundle is
# named; the identifier is what launchd and the keychain key off.
APP_NAME := yt-studio
APP_ID   := com.tbui.yt-studio
APP_DIR  := $(BUILD)/desktop
APP      := $(APP_DIR)/$(APP_NAME).app

.PHONY: help dev dev-desktop build desktop run test bench lint fmt generate clean reset distclean

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
	@mkdir -p $(BUILD)/bin
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=$(VERSION)" -o $(BINARY) ./cmd/server
	@echo "$(BINARY)  $$(du -h $(BINARY) | cut -f1)"

# The window binary. cgo, unavoidably: it is a WKWebView. It is the one reason
# the window is a separate binary at all — the server stays CGO_ENABLED=0, and
# its pure-Go SQLite is why that is worth keeping.
$(DESKTOP): $(shell find cmd/desktop -name '*.go')
	@mkdir -p $(BUILD)/bin
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
	@echo "  mkdir -p ~/.yt-studio && cp -r $(DEV_HOME)/resources ~/.yt-studio/"

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

## clean: delete the build output, leaving the installation untouched
#
# Only var/build. The development installation under var/home is not output, so
# rebuilding never costs a database — that is what reset is for.
clean:
	rm -rf $(BUILD)
	rm -rf web/dist
	@mkdir -p web/dist && cp web/placeholder.html web/dist/index.html
	go clean -testcache

## reset: empty the development installation, keeping var/home/resources
#
# The counterpart to clean: clean deletes what building made, reset deletes what
# running made — the database, the asset store, the transcripts, the log. What
# survives is resources, the one thing under var/home that no command can put
# back.
#
# It names what to delete rather than what to keep. An exclusion is only correct
# until somebody adds a directory the author of the exclusion never saw, and
# then it deletes a gigabyte of somebody else's video; a list of four names can
# only ever be incomplete, which costs a `rm -rf` and not a re-download.
RESET_DIRS := db assets transcripts log

reset:
	rm -rf $(addprefix $(DEV_HOME)/,$(RESET_DIRS))
	@echo "emptied $(DEV_HOME) - kept resources ($$(du -sh $(DEV_HOME)/resources 2>/dev/null | cut -f1 || echo none))"

## distclean: clean and reset together - everything but var/home/resources
distclean: clean reset

web/node_modules:
	npm --prefix web ci --no-audit --no-fund
