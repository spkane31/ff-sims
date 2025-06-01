.PHONY: build clean run

build:
	@echo "Building the backend..."
	@go build -o bin/backend \
		-ldflags="-X 'backend/pkg/version.GitSHA=$(shell git rev-parse --short=9 HEAD)' -X 'backend/pkg/version.BuildTime=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)'" \
		./cmd/server

run: build
	@echo "Running the backend..."
	@./bin/backend

clean:
	@echo "Cleaning up..."
	@rm -rf bin