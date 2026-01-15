.PHONY: build run clean etl reset-db

run:
	go run ./cmd/server

etl:
	@echo "Running ETL process..."
	go run ./cmd/etl upload --directory /Users/seankane/github.com/ff-sims/scripts/data --multiple-leagues

clean:
	@echo "Cleaning build artifacts..."
	rm -rf bin/
	@echo "Clean complete!"

reset-db:
	@echo "Resetting database..."
	./scripts/reset_db.sh

help:
	@echo "Available commands:"
	@echo "  make build     - Build server and ETL binaries"
	@echo "  make run       - Build and run the server"
	@echo "  make etl       - Run the ETL process"
	@echo "  make clean     - Remove build artifacts"
	@echo "  make reset-db  - Drop and recreate database with fresh schema"
	@echo "  make help      - Show this help message"
