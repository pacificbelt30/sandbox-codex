# codex-dock Makefile

BINARY      := codex-dock
MODULE      := github.com/pacificbelt30/codex-dock
IMAGE       := codex-dock:latest
PREFIX      ?= /usr/local
BINDIR      := $(PREFIX)/bin
# When invoked with sudo, use the original user's home instead of /root
REAL_HOME   := $(if $(SUDO_USER),$(shell getent passwd $(SUDO_USER) | cut -d: -f6),$(HOME))
CONFIG_DIR  := $(REAL_HOME)/.config/codex-dock
CONFIG_FILE := $(CONFIG_DIR)/config.toml

VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS     := -ldflags "-X main.version=$(VERSION)"

.PHONY: all build install install-config install-all docker proxy-docker proxy-run uninstall clean test lint vet tidy help

## all: build binary and Docker image
all: build docker

## build: compile the binary for the current platform
build:
	go build $(LDFLAGS) -o $(BINARY) .

## install: install the binary to $(BINDIR) (default: /usr/local/bin)
install: build
	install -d $(BINDIR)
	install -m 0755 $(BINARY) $(BINDIR)/$(BINARY)
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

## install-all: install binary + default config
install-all: install install-config

## docker: build the sandbox Docker image
docker:
	docker build -t $(IMAGE) -f docker/Dockerfile docker/
	@echo "Docker image built: $(IMAGE)"


## proxy-docker: build auth proxy Docker image
proxy-docker:
	docker build -t codex-dock-auth-proxy:latest -f docker/auth-proxy.Dockerfile .
	@echo "Auth proxy image built: codex-dock-auth-proxy:latest"

## proxy-run: run auth proxy container on dock-net
proxy-run:
	docker network inspect dock-net >/dev/null 2>&1 || docker network create dock-net
	-docker rm -f codex-auth-proxy >/dev/null 2>&1
	docker run -d --name codex-auth-proxy --network dock-net -p 127.0.0.1:18080:18080 codex-dock-auth-proxy:latest
	@echo "Auth proxy running at http://127.0.0.1:18080"
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
		./internal/worktree/... \
		./internal/config/...

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
