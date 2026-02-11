.PHONY: build ci deps lint test coverage help

export GO111MODULE=on

DIST_DIR ?= dist
BIN_DIR ?= bin
TOOLS_BIN_DIR ?= $(BIN_DIR)/tools
COVERAGE_PROFILE ?= coverage.out

GOLANGCILINT_VERSION = 2.5.0

help: ## Show this help.
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+%?:.*?## / {sub("\\\\n",sprintf("\n%22c"," "), $$2);printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build the CLI binary
	CGO_ENABLED=0 go build -o $(DIST_DIR)/eszip ./cmd/eszip

tools: ## Install tools needed for development
	@mkdir -p $(TOOLS_BIN_DIR)
	@echo "installing golangci-lint"
	@curl -sfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(TOOLS_BIN_DIR) v$(GOLANGCILINT_VERSION) > /dev/null 2>&1

deps: tools ## Install dependencies
	go mod download -x

lint: tools ## Lint Go code
	$(TOOLS_BIN_DIR)/golangci-lint run

test: ## Run tests
	go test -race -covermode=atomic -coverprofile $(COVERAGE_PROFILE) -count=1 ./...

coverage: ## Open coverage report in browser
	go tool cover -html $(COVERAGE_PROFILE)

ci: lint test ## Run lint and tests (CI)
