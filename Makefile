SHELL := /bin/sh

BINARY := rct
MODULE := github.com/hironeko/rct
GO ?= go
VERSION ?= 0.5.0-dev
PREFIX ?= $(HOME)/.local
BINDIR ?= $(PREFIX)/bin
BUILD_DIR ?= bin
LDFLAGS := -s -w -X $(MODULE)/internal/cli.Version=$(VERSION)
NPM_CACHE ?= $(if $(TMPDIR),$(TMPDIR),/tmp)/rct-npm-cache

.PHONY: all build install uninstall test test-race test-installer vet web-install web-check web-build check clean

all: build

build:
	mkdir -p "$(BUILD_DIR)"
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o "$(BUILD_DIR)/$(BINARY)" ./cmd/rct

install: build
	install -d "$(BINDIR)"
	install -m 0755 "$(BUILD_DIR)/$(BINARY)" "$(BINDIR)/$(BINARY)"
	@printf 'Installed %s to %s\n' "$(BINARY)" "$(BINDIR)/$(BINARY)"

uninstall:
	RCT_INSTALL_DIR="$(BINDIR)" sh ./scripts/uninstall.sh

test:
	$(GO) test ./... -count=1

test-race:
	$(GO) test -race ./... -count=1

test-installer: build
	sh ./scripts/test-installer.sh "$(BUILD_DIR)/$(BINARY)"

vet:
	$(GO) vet ./...

web-install:
	npm_config_cache="$(NPM_CACHE)" npm --prefix web ci

web-check:
	npm_config_cache="$(NPM_CACHE)" npm --prefix web run typecheck
	npm_config_cache="$(NPM_CACHE)" npm --prefix web test
	npm_config_cache="$(NPM_CACHE)" npm --prefix web run build
	git diff --exit-code -- web/dist

web-build:
	npm_config_cache="$(NPM_CACHE)" npm --prefix web run build

check: test-race vet web-check test-installer

clean:
	rm -f "$(BUILD_DIR)/$(BINARY)"
