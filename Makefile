SHELL := /bin/bash
.DEFAULT_GOAL := help

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "};{printf "  \033[36m%-14s\033[0m %s\n",$$1,$$2}'

infra: ## start postgres and redis
	docker compose up -d postgres redis

contracts: ## build NEAR contracts
	cd contracts && cargo build --target wasm32-unknown-unknown --release

executor: ## run executor
	cd executor && cargo run

api: ## run api
	go run ./services/api/cmd/api

scheduler: ## run scheduler
	go run ./services/scheduler/cmd/scheduler

market-data: ## run market-data
	go run ./services/market-data/cmd/market-data

history: ## run history
	go run ./services/history/cmd/history

client: ## run client
	cd client && bun install && bun dev

test: ## go vet + unit tests
	go vet ./... && go test ./...

test-integration: ## go tests against make infra postgres
	DATABASE_URL=$${DATABASE_URL:-postgres://nearly:nearly@localhost:5432/nearly?sslmode=disable} go test -tags integration ./...
