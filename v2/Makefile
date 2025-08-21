.PHONY: help docker-run

help: ## Show this help message
	@echo 'Usage:'
	@echo '  make [target]'
	@echo ''
	@echo 'Targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

docker-run: ## Build and run the Docker image
	docker build -t ff-sims .
	docker run -p 3000:3000 -p 8080:8080 ff-sims