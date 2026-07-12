.PHONY: help docker-run worker-host-setup worker-host-setup-archive-db build-worker

help: ## Show this help message
	@echo 'Usage:'
	@echo '  make [target]'
	@echo ''
	@echo 'Targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

docker-build: ## Build the Docker image
	docker build -t ff-sims \
		--build-arg GIT_SHA=$$(git rev-parse --short=9 HEAD 2>/dev/null || echo "unknown") \
		--build-arg BUILD_TIME=$$(date -u +%Y-%m-%dT%H:%M:%SZ) \
		.

docker-run: docker-build ## Build and run the Docker image
	docker run -p 8080:8080 -e DATABASE_URL="$$DATABASE_URL" ff-sims

docker-dev: docker-build ## Build and run the Docker image in development mode with logs
	docker run --rm -p 8080:8080 -e DATABASE_URL="$$DATABASE_URL" ff-sims

docker-stop: ## Stop and remove running ff-sims containers
	docker ps -q --filter "ancestor=ff-sims" | xargs -r docker stop
	docker ps -aq --filter "ancestor=ff-sims" | xargs -r docker rm

build-worker: ## Build backend/worker with a real git-SHA build ID (same ldflags as deploy.sh/setup.sh — plain `go build`/`make build` silently skip cmd/worker)
	cd backend && go build -ldflags "-X 'main.buildID=$$(git rev-parse --short=9 HEAD)' -X 'main.promoteOnStart=true'" -o worker ./cmd/worker

worker-host-setup: ## Set up this machine as a Temporal worker host (run on the host itself, with sudo)
	sudo ./deploy/worker-host/setup.sh

worker-host-setup-archive-db: ## Provision the local archive Postgres DB (run on the host itself, with sudo)
	sudo ./deploy/worker-host/setup-archive-db.sh