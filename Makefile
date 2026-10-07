WAILS ?= $(shell command -v wails3 || echo $(HOME)/go/bin/wails3)
GOBIN := $(shell go env GOPATH)/bin

# A checkout runs against its own installation, laid out exactly as
# ~/.yt-studio is. Releases and `make install` use ~/.yt-studio.
DEV_HOME := $(CURDIR)/var/home

.PHONY: help dev build release install sweep test lint fmt generate icon reset clean

## help: list the targets
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## /  make /' | column -t -s ':'

## dev: run the app with hot reload (Go and frontend), against var/home
dev:
	YTS_HOME=$(DEV_HOME) $(WAILS) dev -config ./build/config.yml -port 9246

## build: build the macOS app bundle, bin/yt-studio.app
build:
	$(WAILS) task package

## release: release the next version; the installed app offers to restart into it
# Bumps the patch version; BUMP=minor|major or VERSION=x.y.z to choose.
# ALLOW_DIRTY=1 releases uncommitted changes (untagged).
release:
	@go run ./internal/cmd/release -wails="$(WAILS)" -bump="$(or $(BUMP),patch)" -version="$(VERSION)" $(if $(ALLOW_DIRTY),-allow-dirty)

## install: put the newest release in /Applications (first time; later releases update in place)
install:
	@go run ./internal/cmd/release install

## sweep: report asset files nothing uses (APPLY=1 deletes them); quit the app first
sweep:
	@go run ./internal/cmd/sweep $(if $(APPLY),-apply)

## test: every Go test, with the race detector
test:
	go test -race -count=1 ./internal/...

## lint: go vet, golangci-lint, and the frontend typecheck and lint
lint:
	@command -v $(GOBIN)/golangci-lint >/dev/null || go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
	go vet ./...
	$(GOBIN)/golangci-lint run ./...
	npm --prefix frontend run lint
	@# sqlc's SQLite dialect truncates generated statements when a .sql file
	@# contains multi-byte characters.
	@! grep -rlP '[^\x00-\x7F]' internal/adapters/sqlite/queries internal/adapters/sqlite/migrations 2>/dev/null \
		|| { echo "non-ASCII in SQL corrupts sqlc output (see the files above)"; exit 1; }

## fmt: format Go and TypeScript
fmt:
	gofmt -w main.go internal
	npm --prefix frontend run format

## generate: regenerate the sqlc query layer and the TypeScript bindings
generate:
	@command -v $(GOBIN)/sqlc >/dev/null || go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
	$(GOBIN)/sqlc generate
	$(WAILS) generate bindings -clean=true -ts -i

## icon: redraw the application icon (build/darwin/icons.icns)
icon:
	go run ./internal/cmd/icon --preview var/icon-preview.png

## reset: empty the development installation, keeping resources and credentials
RESET_DIRS := db assets transcripts log tmp window.json
reset:
	rm -rf $(addprefix $(DEV_HOME)/,$(RESET_DIRS))

## clean: delete build output
clean:
	rm -rf bin frontend/dist/app frontend/dist/index.html .task
	go clean -testcache
