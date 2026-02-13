.RECIPEPREFIX := >

BINARY := argus
BIN_DIR := bin
GO ?= go

.PHONY: all build test test-security lint fmt vuln clean install-deps

all: fmt lint test

build:
>mkdir -p $(BIN_DIR)
>$(GO) build -o $(BIN_DIR)/$(BINARY) ./cmd/argus

test:
>$(GO) test ./... -race -v

test-security:
>$(GO) test ./internal/core/tools -run 'Sandbox|Security|Path|Symlink' -v

lint:
>golangci-lint run

fmt:
>$(GO) fmt ./...
>goimports -w .

vuln:
>govulncheck ./...

clean:
>rm -rf $(BIN_DIR)
>$(GO) clean -cache -testcache

install-deps:
>@if command -v apt-get >/dev/null 2>&1; then \
>  sudo apt-get update && sudo apt-get install -y git ripgrep fd-find; \
>elif command -v dnf >/dev/null 2>&1; then \
>  sudo dnf install -y git ripgrep fd-find; \
>elif command -v pacman >/dev/null 2>&1; then \
>  sudo pacman -Sy --noconfirm git ripgrep fd; \
>elif command -v brew >/dev/null 2>&1; then \
>  brew update && brew install git ripgrep fd; \
>else \
>  echo "Unsupported package manager. Install git, ripgrep, and fd manually."; \
>  exit 1; \
>fi
>@if ! command -v fd >/dev/null 2>&1 && command -v fdfind >/dev/null 2>&1; then \
>  sudo ln -sf "$$(command -v fdfind)" /usr/local/bin/fd || true; \
>fi
>@command -v git >/dev/null 2>&1 || (echo "git is required" && exit 1)
>@command -v rg >/dev/null 2>&1 || (echo "ripgrep (rg) is required" && exit 1)
>@command -v fd >/dev/null 2>&1 || command -v fdfind >/dev/null 2>&1 || (echo "fd (or fdfind) is required" && exit 1)
>@command -v golangci-lint >/dev/null 2>&1 || (echo "golangci-lint is required" && exit 1)
>@command -v govulncheck >/dev/null 2>&1 || (echo "govulncheck is required" && exit 1)
