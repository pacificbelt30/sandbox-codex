# codex-dock Makefile

BINARY      := codex-dock
WORKER_GUARD := codex-dock-worker-guard
IO_LOGGER := codex-dock-io-logger
MODULE      := github.com/pacificbelt30/codex-dock
IMAGE       := codex-dock:latest
PROXY_IMAGE := codex-dock-proxy:latest
PREFIX      ?= /usr/local
BINDIR      := $(PREFIX)/bin
# When invoked with sudo, use the original user's home instead of /root
REAL_HOME   := $(if $(SUDO_USER),$(shell getent passwd $(SUDO_USER) | cut -d: -f6),$(HOME))
CONFIG_DIR    := $(REAL_HOME)/.config/codex-dock
CONFIG_FILE   := $(CONFIG_DIR)/config.toml
TEMPLATES_DIR := $(CONFIG_DIR)/templates

VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS     := -ldflags "-X main.version=$(VERSION)"

.PHONY: all build worker-guard io-logger install install-config install-templates install-all image proxy-image docker proxy-docker uninstall clean test kata-e2e lint vet tidy help

## all: build binary and OCI images
all: build image proxy-image

## build: compile the binary for the current platform
build: worker-guard io-logger
	go build $(LDFLAGS) -o $(BINARY) .

## worker-guard: build the static Linux x86_64 Kata bootstrap
worker-guard:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o $(WORKER_GUARD) ./cmd/worker-guard

## io-logger: build the shim-owned static Linux console logger
io-logger:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o $(IO_LOGGER) ./cmd/io-logger

## install: install the binary to $(BINDIR) (default: /usr/local/bin)
install: build
	install -d $(BINDIR)
	install -m 0755 $(BINARY) $(BINDIR)/$(BINARY)
	install -m 0755 $(IO_LOGGER) $(BINDIR)/$(IO_LOGGER)
	@echo "Installed $(BINDIR)/$(BINARY)"

## install-config: place the default config file (skips if already exists)
install-config:
	@install -d $(CONFIG_DIR)
	@if [ -f "$(CONFIG_FILE)" ]; then \
		echo "Config already exists at $(CONFIG_FILE), skipping."; \
	else \
		install -m 0600 configs/config.toml.example $(CONFIG_FILE); \
		echo "Default config installed at $(CONFIG_FILE)"; \
	fi

## install-templates: place copies of the built-in templates under ~/.config/codex-dock/templates (skips entries that already exist)
install-templates:
	@install -d $(TEMPLATES_DIR)
	@if [ -f "$(TEMPLATES_DIR)/plain/Dockerfile" ]; then \
		echo "Template 'plain' already exists at $(TEMPLATES_DIR)/plain/Dockerfile, skipping."; \
	else \
		install -d $(TEMPLATES_DIR)/plain; \
		install -m 0644 docker/sandbox/Dockerfile $(TEMPLATES_DIR)/plain/Dockerfile; \
		echo "Default template 'plain' installed at $(TEMPLATES_DIR)/plain/Dockerfile"; \
	fi
	@for name in $(notdir $(wildcard docker/templates/*)); do \
		if [ -f "$(TEMPLATES_DIR)/$$name/Dockerfile" ]; then \
			echo "Template '$$name' already exists at $(TEMPLATES_DIR)/$$name/Dockerfile, skipping."; \
		else \
			install -d $(TEMPLATES_DIR)/$$name; \
			install -m 0644 docker/templates/$$name/Dockerfile $(TEMPLATES_DIR)/$$name/Dockerfile; \
			echo "Default template '$$name' installed at $(TEMPLATES_DIR)/$$name/Dockerfile"; \
		fi; \
	done

## install-all: install binary + default config + built-in template copies
install-all: install install-config install-templates

## image: build the worker OCI image with standalone BuildKit
image: build
	./$(BINARY) build -t $(IMAGE) -f docker/sandbox/Dockerfile

docker: image

## proxy-image: build the proxy OCI image with standalone BuildKit
proxy-image: build
	./$(BINARY) proxy build -t $(PROXY_IMAGE) -f docker/proxy/Dockerfile

proxy-docker: proxy-image

## uninstall: remove the installed binary
uninstall:
	rm -f $(BINDIR)/$(BINARY)
	@echo "Removed $(BINDIR)/$(BINARY)"

## clean: remove build artifacts
clean:
	rm -f $(BINARY) codex-dock-* coverage.out

## test: run tests with race detection and coverage
test:
	go test \
		-race \
		-coverprofile=coverage.out \
		-covermode=atomic \
		./cmd/... \
		./internal/sandbox/... \
		./internal/authproxy/... \
		./internal/network/... \
		./internal/networkpolicy/... \
		./internal/kataguard/... \
		./internal/engine/... \
		./internal/worktree/... \
		./internal/config/... \
		./internal/template/...

## kata-e2e: run real Kata acceptance on a configured execution host (no prerequisite skips)
kata-e2e: build
	CODEX_DOCK_KATA_E2E=1 CODEX_DOCK_KATA_TEST_BINARY="$(CURDIR)/$(BINARY)" go test ./internal/sandbox -run '^TestKata(Acceptance|WorkspaceSharing)$$' -count=1 -v -timeout=15m

## lint: run golangci-lint
lint:
	golangci-lint run --timeout=5m

## vet: run go vet
vet:
	go vet ./...

## tidy: tidy go modules
tidy:
	go mod tidy

# Cross-compilation targets (mirrors CI)
build-darwin-arm64:
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BINARY)-darwin-arm64 .

build-darwin-amd64:
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BINARY)-darwin-amd64 .

build-linux-arm64:
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BINARY)-linux-arm64 .

## build-all: cross-compile for all platforms
build-all: build build-darwin-arm64 build-darwin-amd64 build-linux-arm64

## help: show this help
help:
	@grep -E '^## ' Makefile | sed 's/^## //'
