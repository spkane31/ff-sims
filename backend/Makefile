.PHONY: build build-migrate clean run etl migrate migrate-status migrate-down migrate-archive migrate-archive-status migrate-archive-down deduplicate help

help: ## Show this help message
	@echo 'Makefile for backend'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: build-migrate ## Build the backend binaries
	@echo "Building the backend..."
	@go build -o bin/backend \
		-ldflags="-X 'backend/pkg/version.GitSHA=$(shell git rev-parse --short=9 HEAD)' -X 'backend/pkg/version.BuildTime=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)'" \
		./cmd/server
	@go build -o bin/etl \
		-ldflags="-X 'backend/pkg/version.GitSHA=$(shell git rev-parse --short=9 HEAD)' -X 'backend/pkg/version.BuildTime=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)'" \
		./cmd/etl

build-migrate: ## Build only the migrate CLI binary (fast — skips server/etl)
	@echo "Building migrate..."
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

migrate: build-migrate ## Apply all pending goose migrations (requires DATABASE_URL)
	@./bin/migrate up

migrate-status: build-migrate ## Show goose migration status (requires DATABASE_URL)
	@./bin/migrate status

migrate-down: build-migrate ## Roll back the last migration (requires DATABASE_URL)
	@./bin/migrate down

migrate-archive: build-migrate ## Apply all pending goose migrations to the archive DB (requires ARCHIVE_DATABASE_URL)
	@./bin/migrate -db=archive up

migrate-archive-status: build-migrate ## Show goose migration status for the archive DB (requires ARCHIVE_DATABASE_URL)
	@./bin/migrate -db=archive status

migrate-archive-down: build-migrate ## Roll back the last archive migration (requires ARCHIVE_DATABASE_URL)
	@./bin/migrate -db=archive down

deduplicate: ## Remove duplicate records from database
	@echo "Running deduplication process..."
	@go run ./cmd/deduplicate