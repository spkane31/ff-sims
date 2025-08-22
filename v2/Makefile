.PHONY: help docker-run

help: ## Show this help message
	@echo 'Usage:'
	@echo '  make [target]'
	@echo ''
	@echo 'Targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

docker-run: ## Build and run the Docker image
	docker build -t ff-sims \
		--build-arg GIT_SHA=$$(git rev-parse --short=9 HEAD 2>/dev/null || echo "unknown") \
		--build-arg BUILD_TIME=$$(date -u +%Y-%m-%dT%H:%M:%SZ) \
		.
	docker run -p 8000:80 -e DATABASE_URL="$$DATABASE_URL" ff-sims