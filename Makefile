SHELL := /bin/bash
.DEFAULT_GOAL := help
.PHONY: help infra contracts contracts-test test-contracts executor api scheduler market-data history client

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "};{printf "  \033[36m%-14s\033[0m %s\n",$$1,$$2}'

infra: ## start postgres and redis
	docker compose up -d postgres redis

WASM := wasm32-unknown-unknown
# account first: factory embeds contracts/res/nearly_account.wasm. mocks/* are sandbox-only venues.
CONTRACTS := account factory mocks/ft mocks/lending mocks/dex mocks/staking
contracts: ## build NEAR contracts into contracts/res
	mkdir -p contracts/res
	@for c in $(CONTRACTS); do \
	  p=nearly-$${c//\//-}; \
	  if command -v cargo-near >/dev/null; then \
	    (cd contracts/$$c && cargo near build non-reproducible-wasm --no-abi --out-dir $(CURDIR)/contracts/res) || exit 1; \
	  else \
	    (cd contracts && cargo build -p $$p --target $(WASM) --release && cp target/$(WASM)/release/$${p//-/_}.wasm res/) || exit 1; \
	  fi; \
	done

contracts-test: contracts ## unit + sandbox tests for contracts
	cd contracts && cargo test

test-contracts: contracts-test ## alias of contracts-test

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
