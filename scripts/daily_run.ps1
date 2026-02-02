# Market Intelligence Aggregator - Daily Run Script (Windows PowerShell)
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
#   .\scripts\daily_run.ps1           # Run all steps
#   .\scripts\daily_run.ps1 -NoTui    # Run without TUI
#
# Prerequisites:
#   - Go 1.21+
#   - Python 3.11+ with venv
#   - Environment variables set (see .envrc.example)

param(
    [switch]$NoTui
)

$ErrorActionPreference = "Stop"

# Configuration
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ProjectRoot = Split-Path -Parent $ScriptDir
$VenvPath = Join-Path $ProjectRoot "venv"
$LogDir = Join-Path $ProjectRoot "logs"
$LogFile = Join-Path $LogDir "daily_run_$(Get-Date -Format 'yyyyMMdd_HHmmss').log"

# Functions
function Write-Log {
    param([string]$Message)
    $timestamp = Get-Date -Format "yyyy-MM-dd HH:mm:ss"
    $logMessage = "[$timestamp] $Message"
    Write-Host $logMessage
    Add-Content -Path $LogFile -Value $logMessage
}

function Test-EnvVar {
    param([string]$VarName)
    $value = [Environment]::GetEnvironmentVariable($VarName)
    if ([string]::IsNullOrEmpty($value)) {
        Write-Log "ERROR: Environment variable $VarName is not set"
        exit 1
    }
}

# Main script
function Main {
    Write-Log "=== Market Intelligence Daily Run ==="
    Write-Log "Started at: $(Get-Date)"
    Write-Log "Project root: $ProjectRoot"

    # Create logs directory if needed
    if (-not (Test-Path $LogDir)) {
        New-Item -ItemType Directory -Path $LogDir | Out-Null
    }

    # Change to project directory
    Set-Location $ProjectRoot

    # Check required environment variables
    Write-Log "Checking environment variables..."
    Test-EnvVar "DATABASE_URL"
    Test-EnvVar "ALPHAVANTAGE_API_KEY"
    Test-EnvVar "TWITTER_BEARER_TOKEN"
    Test-EnvVar "ANTHROPIC_API_KEY"
    Write-Log "Environment validated"

    # Activate Python virtual environment
    $venvActivate = Join-Path $VenvPath "Scripts\Activate.ps1"
    if (Test-Path $venvActivate) {
        Write-Log "Activating Python virtual environment..."
        & $venvActivate
    } else {
        Write-Log "Warning: Virtual environment not found at $VenvPath"
        Write-Log "Python scripts may fail if dependencies are not installed globally"
    }

    # Step 1: Fetch portfolio from Robinhood
    Write-Log "Step 1/6: Fetching portfolio from Robinhood..."
    $robinhoodUser = [Environment]::GetEnvironmentVariable("ROBINHOOD_USERNAME")
    $robinhoodPass = [Environment]::GetEnvironmentVariable("ROBINHOOD_PASSWORD")
    if (-not [string]::IsNullOrEmpty($robinhoodUser) -and -not [string]::IsNullOrEmpty($robinhoodPass)) {
        try {
            python "$ProjectRoot\services\robinhood\fetch_portfolio.py"
        } catch {
            Write-Log "Warning: Portfolio fetch failed, continuing..."
        }
    } else {
        Write-Log "Skipping portfolio fetch (Robinhood credentials not set)"
    }

    # Step 2: Fetch market data
    Write-Log "Step 2/6: Fetching market data..."
    try {
        go run "$ProjectRoot\cmd\orchestrator\main.go" fetch-market
    } catch {
        Write-Log "Warning: Market data fetch failed, continuing..."
    }

    # Step 3: Fetch creator content
    Write-Log "Step 3/6: Fetching creator content..."
    try {
        go run "$ProjectRoot\cmd\orchestrator\main.go" fetch-social
    } catch {
        Write-Log "Warning: Social content fetch failed, continuing..."
    }

    # Step 4: Calculate technical indicators
    Write-Log "Step 4/6: Calculating technical indicators..."
    try {
        python "$ProjectRoot\services\analysis\indicators.py"
    } catch {
        Write-Log "Warning: Indicator calculation failed, continuing..."
    }

    # Step 5: Generate embeddings for new content
    Write-Log "Step 5/6: Generating embeddings..."
    try {
        python "$ProjectRoot\services\analysis\embeddings.py"
    } catch {
        Write-Log "Warning: Embedding generation failed, continuing..."
    }

    # Step 6: Run analysis and generate recommendations
    Write-Log "Step 6/6: Generating recommendations..."
    try {
        go run "$ProjectRoot\cmd\orchestrator\main.go" analyze
    } catch {
        Write-Log "ERROR: Analysis failed"
        exit 1
    }

    Write-Log "Daily workflow completed successfully"
    Write-Log "Ended at: $(Get-Date)"

    # Launch TUI if requested
    if (-not $NoTui) {
        Write-Log "Launching TUI..."
        go run "$ProjectRoot\cmd\tui\main.go"
    }
}

# Run main function
Main
