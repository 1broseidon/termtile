.PHONY: help build install test vet lint check clean publish publish-dry-run brainfile-validate
.DEFAULT_GOAL := help

BIN     := termtile
CMD     := ./cmd/termtile
INSTALL := $(HOME)/.local/bin/$(BIN)

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*##' $(MAKEFILE_LIST) | awk -F ':.*## ' '{printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

build: ## Build the binary
	go build -o $(BIN) $(CMD)

install: build ## Install binary and reload systemd service
	install -m 755 $(BIN) $(INSTALL)
	systemctl --user daemon-reload
	systemctl --user restart $(BIN).service

test: ## Run all tests
	go test ./...

vet: ## Run go vet
	go vet ./...

lint: vet ## Run vet + staticcheck
	@command -v staticcheck >/dev/null 2>&1 && staticcheck ./... || echo "staticcheck not installed, skipping"

check: lint test ## Run lint and tests

clean: ## Remove build artifacts
	rm -f $(BIN)

publish: ## Create release commit+tag and push (BUMP=major|minor|patch|none or VERSION=vX.Y.Z)
	./scripts/publish.sh

publish-dry-run: ## Preview release commands without changing git state
	DRY_RUN=1 ./scripts/publish.sh

brainfile-validate: ## Validate .brainfile/brainfile.md task state consistency
	bash .github/scripts/brainfile-validate.sh
