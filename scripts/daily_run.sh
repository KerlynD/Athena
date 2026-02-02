#!/bin/bash
# Market Intelligence Aggregator - Daily Run Script (Linux/macOS)
#
# This script orchestrates the daily workflow:
# 1. Fetch portfolio from Robinhood
# 2. Fetch market data from Alpha Vantage
# 3. Fetch creator content from Twitter/X
# 4. Calculate technical indicators
# 5. Generate embeddings for new content
# 6. Run analysis and generate recommendations
# 7. Launch TUI to display results
#
# Usage:
#   ./scripts/daily_run.sh           # Run all steps
#   ./scripts/daily_run.sh --no-tui  # Run without TUI
#
# Prerequisites:
#   - Go 1.21+
#   - Python 3.11+ with venv
#   - Environment variables set (see .envrc.example)

set -e  # Exit on error

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
VENV_PATH="${PROJECT_ROOT}/venv"
LOG_FILE="${PROJECT_ROOT}/logs/daily_run_$(date +%Y%m%d_%H%M%S).log"

# Parse arguments
LAUNCH_TUI=true
for arg in "$@"; do
    case $arg in
        --no-tui)
            LAUNCH_TUI=false
            shift
            ;;
    esac
done

# Functions
log() {
    local timestamp=$(date '+%Y-%m-%d %H:%M:%S')
    echo "[${timestamp}] $1" | tee -a "${LOG_FILE}"
}

error() {
    log "ERROR: $1"
    exit 1
}

check_env() {
    local var=$1
    if [ -z "${!var}" ]; then
        error "Environment variable ${var} is not set"
    fi
}

# Main script
main() {
    log "=== Market Intelligence Daily Run ==="
    log "Started at: $(date)"
    log "Project root: ${PROJECT_ROOT}"

    # Create logs directory if needed
    mkdir -p "${PROJECT_ROOT}/logs"

    # Change to project directory
    cd "${PROJECT_ROOT}"

    # Check required environment variables
    log "Checking environment variables..."
    check_env "DATABASE_URL"
    check_env "ALPHAVANTAGE_API_KEY"
    check_env "TWITTER_BEARER_TOKEN"
    check_env "ANTHROPIC_API_KEY"
    log "Environment validated"

    # Activate Python virtual environment
    if [ -d "${VENV_PATH}" ]; then
        log "Activating Python virtual environment..."
        source "${VENV_PATH}/bin/activate"
    else
        log "Warning: Virtual environment not found at ${VENV_PATH}"
        log "Python scripts may fail if dependencies are not installed globally"
    fi

    # Step 1: Fetch portfolio from Robinhood
    log "Step 1/6: Fetching portfolio from Robinhood..."
    if [ -n "${ROBINHOOD_USERNAME}" ] && [ -n "${ROBINHOOD_PASSWORD}" ]; then
        python3 "${PROJECT_ROOT}/services/robinhood/fetch_portfolio.py" || {
            log "Warning: Portfolio fetch failed, continuing..."
        }
    else
        log "Skipping portfolio fetch (Robinhood credentials not set)"
    fi

    # Step 2: Fetch market data
    log "Step 2/6: Fetching market data..."
    go run "${PROJECT_ROOT}/cmd/orchestrator/main.go" fetch-market || {
        log "Warning: Market data fetch failed, continuing..."
    }

    # Step 3: Fetch creator content
    log "Step 3/6: Fetching creator content..."
    go run "${PROJECT_ROOT}/cmd/orchestrator/main.go" fetch-social || {
        log "Warning: Social content fetch failed, continuing..."
    }

    # Step 4: Calculate technical indicators
    log "Step 4/6: Calculating technical indicators..."
    python3 "${PROJECT_ROOT}/services/analysis/indicators.py" || {
        log "Warning: Indicator calculation failed, continuing..."
    }

    # Step 5: Generate embeddings for new content
    log "Step 5/6: Generating embeddings..."
    python3 "${PROJECT_ROOT}/services/analysis/embeddings.py" || {
        log "Warning: Embedding generation failed, continuing..."
    }

    # Step 6: Run analysis and generate recommendations
    log "Step 6/6: Generating recommendations..."
    go run "${PROJECT_ROOT}/cmd/orchestrator/main.go" analyze || {
        error "Analysis failed"
    }

    log "Daily workflow completed successfully"
    log "Ended at: $(date)"

    # Launch TUI if requested
    if [ "${LAUNCH_TUI}" = true ]; then
        log "Launching TUI..."
        go run "${PROJECT_ROOT}/cmd/tui/main.go"
    fi
}

# Run main function
main "$@"
