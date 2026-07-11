# Makefile — SGC Angola (M1). Alvos de desenvolvimento, qualidade e infra.
# Toda a saída em PT-PT.

.DEFAULT_GOAL := ajuda
SHELL := /usr/bin/env bash

BINARIO := bin/api
PKG     := ./...

.PHONY: ajuda
ajuda: ## Mostra esta ajuda
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: ## Compila o binário da API
	go build -o $(BINARIO) ./cmd/api

.PHONY: run
run: ## Arranca a API localmente
	go run ./cmd/api

.PHONY: migrate
migrate: ## Aplica as migrations forward-only
	go run ./cmd/api migrate

.PHONY: test
test: ## Corre todos os testes
	go test -race $(PKG)

.PHONY: cover
cover: ## Verifica o gate de cobertura por camada (85/75/60)
	@bash scripts/cobertura.sh

.PHONY: lint
lint: ## Corre golangci-lint + go-arch-lint
	golangci-lint run
	go-arch-lint check

.PHONY: fmt
fmt: ## Formata o código
	gofmt -s -w .

.PHONY: openapi
openapi: ## Gera a spec OpenAPI a partir das anotações swag
	swag init -g cmd/api/main.go -o api/openapi --parseDependency --parseInternal

.PHONY: docker-up
docker-up: ## Sobe a infra local (docker compose)
	docker compose up -d

.PHONY: docker-down
docker-down: ## Pára a infra local
	docker compose down

.PHONY: tidy
tidy: ## Reorganiza dependências
	go mod tidy
