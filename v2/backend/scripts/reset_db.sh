#!/bin/bash
# Script to drop and recreate the database with fresh schema from GORM models

set -e

DB_NAME="ffsims"
DB_USER="postgres"

echo "⚠️  WARNING: This will destroy all data in the '$DB_NAME' database!"
read -p "Are you sure you want to continue? (yes/no): " confirm

if [ "$confirm" != "yes" ]; then
    echo "Aborted."
    exit 1
fi

echo "🗑️  Dropping database '$DB_NAME'..."
psql -U $DB_USER -c "DROP DATABASE IF EXISTS $DB_NAME WITH (FORCE);"

echo "📦 Creating database '$DB_NAME'..."
psql -U $DB_USER -c "CREATE DATABASE $DB_NAME;"

echo "🔧 Running GORM AutoMigrate..."
cd "$(dirname "$0")/.."
DB_MIGRATE=true go run cmd/server/main.go &
SERVER_PID=$!

# Wait for server to start and run migrations
sleep 3

# Kill the server
kill $SERVER_PID 2>/dev/null || true

echo "✅ Database reset complete!"
echo ""
echo "To populate with data, run:"
echo "  make etl"
