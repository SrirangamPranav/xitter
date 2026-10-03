.PHONY: help dev build test docker-up docker-down generate seed

help:
	@echo "Twitter-like Backend Development Commands:"
	@echo "  make dev          - Run Go backend server locally"
	@echo "  make build        - Compile server binary into bin/server"
	@echo "  make test         - Run unit & integration tests"
	@echo "  make docker-up    - Start PostgreSQL, Session Redis & Cache Redis in Docker"
	@echo "  make docker-down  - Stop all running Docker containers"
	@echo "  make generate     - Re-generate sqlc queries and gqlgen schema"

dev:
	go run ./cmd/server

build:
	go build -o bin/server ./cmd/server

test:
	go test -v ./...

docker-up:
	docker compose up -d

docker-down:
	docker compose down

generate:
	sqlc generate
	go run github.com/99designs/gqlgen generate

seed:
	@echo "To seed dummy data, send requests to /auth/dev/login?handle=alice and /auth/dev/login?handle=bob"
