.PHONY: help docker-run

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