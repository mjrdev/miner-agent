.DEFAULT_GOAL := help

COMPOSE ?= docker compose
IMAGE   ?= miner-api

.PHONY: help up down reset logs ps psql test fmt lint build

help: ## List available targets
	@grep -E '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*## "}; {printf "  %-8s %s\n", $$1, $$2}'

up: ## Build and start the api and postgres in the background
	$(COMPOSE) up -d --build

down: ## Stop the stack, keeping database data
	$(COMPOSE) down

reset: ## Stop the stack and delete database data
	$(COMPOSE) down -v

logs: ## Follow the api logs
	$(COMPOSE) logs -f api

ps: ## Show the stack status
	$(COMPOSE) ps

psql: ## Open a psql shell in the database
	$(COMPOSE) exec postgres sh -c 'psql -U "$$POSTGRES_USER" -d "$$POSTGRES_DB"'

test: ## Run the Go tests
	go test ./...

fmt: ## Format and vet the code
	gofmt -l -w .
	go vet ./...

lint: ## Run golangci-lint (when installed)
	@command -v golangci-lint >/dev/null 2>&1 || { echo "golangci-lint not installed: https://golangci-lint.run/welcome/install/"; exit 1; }
	golangci-lint run ./...

build: ## Build the production runtime image
	docker build --target runtime -t $(IMAGE) .
