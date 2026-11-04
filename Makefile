# Root convenience wrapper around backend/ and frontend/ — each has its own
# Makefile/package.json with the real targets; this just saves cd'ing.

.PHONY: deps deps-down migrate-up migrate-vectorstore-up backend-build backend-run backend-test backend-lint frontend-install frontend-dev frontend-build

deps:
	$(MAKE) -C backend deps

deps-down:
	$(MAKE) -C backend deps-down

migrate-up:
	$(MAKE) -C backend migrate-up

migrate-vectorstore-up:
	$(MAKE) -C backend migrate-vectorstore-up

backend-build:
	$(MAKE) -C backend build

backend-run:
	$(MAKE) -C backend run

backend-test:
	$(MAKE) -C backend test

backend-lint:
	$(MAKE) -C backend lint

frontend-install:
	cd frontend && npm install

frontend-dev:
	cd frontend && npm run dev

frontend-build:
	cd frontend && npm run build
