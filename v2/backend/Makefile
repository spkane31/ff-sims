.PHONY: build clean run etl migrate deduplicate help

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

run: build ## Build and run the backend server
	@echo "Running the backend..."
	@./bin/backend

clean: ## Remove built binaries
	@echo "Cleaning up..."
	@rm -rf bin

etl: build ## Run the ETL process
	@echo "Running ETL process..."
	@go run ./cmd/etl upload --data-dir /Users/seankane/github.com/ff-sims/scripts/data

migrate: ## Run all database migration files (requires DATABASE_URL)
	@echo "Running database migrations..."
	@if [ -z "$(DATABASE_URL)" ]; then \
		echo "ERROR: DATABASE_URL environment variable is not set"; \
		exit 1; \
	fi
	@for file in migrations/*.sql; do \
		if [ -f "$$file" ]; then \
			echo "Applying migration: $$file"; \
			psql $(DATABASE_URL) -f "$$file" || exit 1; \
		fi; \
	done
	@echo "All migrations applied successfully"

deduplicate: ## Remove duplicate records from database
	@echo "Running deduplication process..."
	@go run ./cmd/deduplicate