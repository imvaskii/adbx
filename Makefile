GO          := go
BINARY_NAME := adbx
BUILD_DIR   := bin
BIN         := $(BUILD_DIR)/$(BINARY_NAME)
INSTALL     := $(HOME)/.local/bin/adbx
MAIN_PKG    := .

VERSION_VAR     := main.version
GIT_COMMIT_VAR  := main.gitCommit
REPO_VERSION    := $(shell git describe --tags --always 2>/dev/null || echo dev)
GIT_HASH        := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)

# Semver tag helper — usage: make tag [patch|minor|major] (default: patch)

# Build flags
# Static linking (-extldflags=-static) only works for native Linux builds.
# Cross-compilation targets use portable flags without static linking.
LDFLAGS_VERSION := -X $(VERSION_VAR)=$(REPO_VERSION) -X $(GIT_COMMIT_VAR)=$(GIT_HASH)
GOBUILD_ARGS        := -ldflags "-s -w -extldflags=-static $(LDFLAGS_VERSION)"
GOBUILD_ARGS_CROSS  := -ldflags "-s -w $(LDFLAGS_VERSION)"

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}' | \
		sort

.PHONY: tag patch minor major
tag: ## Bump and push semver tag: make tag [patch|minor|major] (default: patch)
	@bump=$(or $(filter patch minor major, $(MAKECMDGOALS)), patch); \
	last=$$(git tag --sort=-version:refname | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$$' | head -1); \
	last=$${last:-v0.0.0}; \
	major=$$(echo $$last | sed 's/v\([0-9]*\)\..*/\1/'); \
	minor=$$(echo $$last | sed 's/v[0-9]*\.\([0-9]*\)\..*/\1/'); \
	patch=$$(echo $$last | sed 's/v[0-9]*\.[0-9]*\.\([0-9]*\).*/\1/'); \
	case "$$bump" in \
	  major) next="v$$((major+1)).0.0" ;; \
	  minor) next="v$$major.$$((minor+1)).0" ;; \
	  *)     next="v$$major.$$minor.$$((patch+1))" ;; \
	esac; \
	echo "Current: $$last  →  Next: $$next"; \
	git tag $$next; \
	git push origin $$next

patch minor major:
	@:

.PHONY: tag-push
tag-push: ## Push the latest local tag to origin
	@latest=$$(git tag --sort=-version:refname | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$$' | head -1); \
	if [ -n "$$latest" ]; then \
	  if git ls-remote --tags origin | grep -q "$$latest"; then \
	    echo "Tag $$latest already exists on origin"; \
	  else \
	    echo "Pushing tag $$latest to origin"; \
	    git push origin "$$latest"; \
	  fi; \
	else \
	  echo "No semver tags found locally"; \
	fi

.PHONY: build
build: ## Build binary for the current platform
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(GOBUILD_ARGS) -o $(BIN) $(MAIN_PKG)

.PHONY: install
install: build ## Build and install to $(INSTALL)
	@cp $(BIN) $(INSTALL)
	@echo "Installed to $(INSTALL)"

.PHONY: build-all
build-all: ## Build for all platforms (linux/darwin/windows)
	@mkdir -p $(BUILD_DIR)
	GOOS=linux  GOARCH=amd64 CGO_ENABLED=0 $(GO) build $(GOBUILD_ARGS)       -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64  $(MAIN_PKG)
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 $(GO) build $(GOBUILD_ARGS_CROSS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 $(MAIN_PKG)
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 $(GO) build $(GOBUILD_ARGS_CROSS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(MAIN_PKG)

.PHONY: lint
lint: ## Run golangci-lint
	@if ! command -v golangci-lint &> /dev/null; then \
		echo "golangci-lint not found. Run 'mise install' or 'make install-tools' first"; \
		exit 1; \
	fi
	golangci-lint run ./...

.PHONY: lint-fix
lint-fix: ## Run golangci-lint with auto-fix
	@if ! command -v golangci-lint &> /dev/null; then \
		echo "golangci-lint not found. Run 'mise install' or 'make install-tools' first"; \
		exit 1; \
	fi
	golangci-lint run --fix ./...

.PHONY: vet
vet: ## Run go vet
	$(GO) vet ./...

.PHONY: fmt
fmt: ## Format code with gofmt and goimports
	$(GO) fmt ./...
	@if command -v goimports &> /dev/null; then \
		goimports -w -local github.com/imvaskii/adbx .; \
	else \
		echo "goimports not found. Run 'mise install' or 'make install-tools' first"; \
	fi

.PHONY: test
test: ## Run all tests
	$(GO) test -v -count=1 ./...

.PHONY: clean
clean: ## Remove build artifacts
	@rm -rf $(BUILD_DIR)

.PHONY: install-tools
install-tools: ## Install all development tools
	@echo "Installing development tools..."
	$(GO) install golang.org/x/tools/cmd/goimports@latest
	$(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	$(GO) install github.com/air-verse/air@latest
	@echo "All tools installed successfully"

