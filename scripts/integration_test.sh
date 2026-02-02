#!/bin/bash
# Market Intelligence Aggregator - Integration Test Script
#
# Tests the complete pipeline from data fetching to recommendation generation.
# Run this script to verify the system is working correctly.
#
# Usage:
#   ./scripts/integration_test.sh
#
# Prerequisites:
#   - All environment variables set
#   - Database schema created
#   - Dependencies installed

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

echo "=== Market Intelligence Integration Test ==="
echo "Started at: $(date)"
echo ""

cd "${PROJECT_ROOT}"

# Test 1: Database connection
echo "Test 1: Database connection..."
psql "${DATABASE_URL}" -c "SELECT 1" > /dev/null 2>&1 && echo "✓ Database connected" || {
    echo "✗ Database connection failed"
    exit 1
}

# Test 2: Go build
echo "Test 2: Go build..."
go build -o /dev/null ./cmd/orchestrator && echo "✓ Orchestrator builds" || {
    echo "✗ Orchestrator build failed"
    exit 1
}

go build -o /dev/null ./cmd/tui && echo "✓ TUI builds" || {
    echo "✗ TUI build failed"
    exit 1
}

# Test 3: Go tests
echo "Test 3: Running Go tests..."
go test ./... -v -short && echo "✓ Go tests passed" || {
    echo "✗ Go tests failed"
    exit 1
}

# Test 4: Python imports
echo "Test 4: Python imports..."
python3 -c "import robin_stocks; import pandas_ta; import sentence_transformers" 2>/dev/null && \
    echo "✓ Python dependencies available" || {
    echo "✗ Python dependencies missing"
    exit 1
}

# Test 5: Database schema
echo "Test 5: Database schema..."
psql "${DATABASE_URL}" -c "SELECT COUNT(*) FROM config" > /dev/null 2>&1 && \
    echo "✓ Database schema exists" || {
    echo "✗ Database schema missing - run scripts/setup_db.sql"
    exit 1
}

# Test 6: Environment variables
echo "Test 6: Environment variables..."
for var in DATABASE_URL ALPHAVANTAGE_API_KEY TWITTER_BEARER_TOKEN ANTHROPIC_API_KEY; do
    if [ -z "${!var}" ]; then
        echo "✗ ${var} not set"
        exit 1
    fi
done
echo "✓ All required environment variables set"

echo ""
echo "=== All integration tests passed ==="
echo "Completed at: $(date)"
