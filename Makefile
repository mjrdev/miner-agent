.DEFAULT_GOAL := help

COMPOSE ?= docker compose
IMAGE   ?= miner-api
SQLC    ?= sqlc/sqlc:1.30.0

AGENT_IMAGE ?= miner-agent:default

.PHONY: help agent-image test-agent-image up down reset logs ps psql migrate migrate-down sqlc test test-integration fmt lint build

help: ## List available targets
	@grep -E '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*## "}; {printf "  %-16s %s\n", $$1, $$2}'

agent-image: ## Build the default image the runs execute in
	docker build -t $(AGENT_IMAGE) build/agent

test-agent-image: agent-image ## Check the run entrypoint of the agent image with a fake agent
	build/agent/test.sh $(AGENT_IMAGE)

up: agent-image ## Build the agent image and start the stack in the background
	@mkdir -p runs
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

migrate: ## Apply pending database migrations
	$(COMPOSE) run --rm migrate go run ./cmd/migrate up

migrate-down: ## Roll back the last database migration
	$(COMPOSE) run --rm migrate go run ./cmd/migrate down

sqlc: ## Regenerate the database code from internal/task/postgres/queries
	docker run --rm -u "$$(id -u):$$(id -g)" -v "$(CURDIR):/src" -w /src $(SQLC) generate

test: ## Run the Go tests
	go test ./...

test-integration: ## Run every test, including integration, against the compose postgres and rabbitmq
	$(COMPOSE) up -d --wait postgres rabbitmq
	$(COMPOSE) exec -T postgres sh -c 'psql -U "$$POSTGRES_USER" -d "$$POSTGRES_DB" -tAc "SELECT 1 FROM pg_database WHERE datname = '"'"'miner_test'"'"'" | grep -q 1 || createdb -U "$$POSTGRES_USER" miner_test'
	$(COMPOSE) exec -T rabbitmq sh -c 'rabbitmqctl -q list_vhosts | grep -qx miner_test || rabbitmqctl -q add_vhost miner_test; rabbitmqctl -q set_permissions -p miner_test "$$RABBITMQ_DEFAULT_USER" ".*" ".*" ".*"'
	$(COMPOSE) run --rm test

fmt: ## Format and vet the code
	gofmt -l -w .
	go vet ./...

lint: ## Run golangci-lint (when installed)
	@command -v golangci-lint >/dev/null 2>&1 || { echo "golangci-lint not installed: https://golangci-lint.run/welcome/install/"; exit 1; }
	golangci-lint run ./...

build: ## Build the production runtime image
	docker build --target runtime -t $(IMAGE) .
