.PHONY: build run test lint docker deps deps-down migrate-up migrate-down migrate-vectorstore-up migrate-vectorstore-down

build:
	go build -o bin/agentsip ./cmd/engine

run: build
	./bin/agentsip

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

docker:
	docker build --build-arg VERSION="$(VERSION)" -t agentsip:$(VERSION) -t agentsip:local .

# Postgres for development and the integration suite.
deps:
	docker compose up -d --wait

deps-down:
	docker compose down

test:
	go test ./...

lint:
	gofmt -l . | grep . && exit 1 || true
	go vet ./...

# Requires the golang-migrate CLI: https://github.com/golang-migrate/migrate
#
# Two independent migration sequences, because db/migrations_vectorstore/ is
# meant to run against a possibly different database
# (VECTORSTORE_DATABASE_URL) — see that directory's migration file for why
# mixing the two sequences under one -path would be wrong.
migrate-up:
	migrate -database "$${DATABASE_URL}" -path db/migrations up

migrate-down:
	migrate -database "$${DATABASE_URL}" -path db/migrations down 1

migrate-vectorstore-up:
	migrate -database "$${VECTORSTORE_DATABASE_URL:-$$DATABASE_URL}" -path db/migrations_vectorstore up

migrate-vectorstore-down:
	migrate -database "$${VECTORSTORE_DATABASE_URL:-$$DATABASE_URL}" -path db/migrations_vectorstore down 1
