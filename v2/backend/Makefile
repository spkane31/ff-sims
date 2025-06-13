.PHONY: build clean run

help:
	@echo "Makefile for backend"
	@echo "Available targets:"
	@echo "  build - Build the backend binaries"
	@echo "  run - Build and run the backend server"
	@echo "  clean - Remove built binaries"
	@echo "  etl - Run the ETL process"

build:
	@echo "Building the backend..."
	@go build -o bin/backend \
		-ldflags="-X 'backend/pkg/version.GitSHA=$(shell git rev-parse --short=9 HEAD)' -X 'backend/pkg/version.BuildTime=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)'" \
		./cmd/server
	@go build -o bin/etl \
		-ldflags="-X 'backend/pkg/version.GitSHA=$(shell git rev-parse --short=9 HEAD)' -X 'backend/pkg/version.BuildTime=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)'" \
		./cmd/etl

run: build
	@echo "Running the backend..."
	@./bin/backend

clean:
	@echo "Cleaning up..."
	@rm -rf bin

etl: build
	@echo "Running ETL process..."
	@go run ./cmd/etl upload --data-dir /Users/seankane/github.com/ff-sims/scripts/data