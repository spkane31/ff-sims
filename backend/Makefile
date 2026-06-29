.PHONY: build clean run etl migrate migrate-status migrate-down deduplicate help

help: ## Show this help message
	@echo 'Makefile for backend'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build the backend binaries
	@echo "Building the backend..."
	@go build -o bin/backend \
		-ldflags="-X 'backend/pkg/version.GitSHA=$(shell git rev-parse --short=9 HEAD)' -X 'backend/pkg/version.BuildTime=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)'" \
		./cmd/server
	@go build -o bin/etl \
		-ldflags="-X 'backend/pkg/version.GitSHA=$(shell git rev-parse --short=9 HEAD)' -X 'backend/pkg/version.BuildTime=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)'" \
		./cmd/etl
	@go build -o bin/migrate \
		-ldflags="-X 'backend/pkg/version.GitSHA=$(shell git rev-parse --short=9 HEAD)' -X 'backend/pkg/version.BuildTime=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)'" \
		./cmd/migrate

run: build ## Build and run the backend server
	@echo "Running the backend..."
	@./bin/backend

clean: ## Remove built binaries
	@echo "Cleaning up..."
	@rm -rf bin

etl: build ## Run the ETL process
	@echo "Running ETL process..."
	@go run ./cmd/etl --league-id=345674 --platform=ESPN upload --data-dir /Users/seankane/github.com/ff-sims/scripts/data

migrate: build ## Apply all pending goose migrations (requires DATABASE_URL)
	@./bin/migrate up

migrate-status: build ## Show goose migration status (requires DATABASE_URL)
	@./bin/migrate status

migrate-down: build ## Roll back the last migration (requires DATABASE_URL)
	@./bin/migrate down

deduplicate: ## Remove duplicate records from database
	@echo "Running deduplication process..."
	@go run ./cmd/deduplicate