# Load environment variables from .envrc for Windows PowerShell
# Usage: . .\scripts\load_env.ps1

$envFile = Join-Path $PSScriptRoot "..\.envrc"

if (-not (Test-Path $envFile)) {
    Write-Error ".envrc file not found at $envFile"
    exit 1
}

Write-Host "Loading environment variables from .envrc..." -ForegroundColor Cyan

Get-Content $envFile | ForEach-Object {
    # Skip comments and empty lines
    if ($_ -match '^\s*#' -or $_ -match '^\s*$') {
        return
    }
    
    # Parse export VAR="value" format
    if ($_ -match '^export\s+([A-Z_][A-Z0-9_]*)=["'']?([^"'']*?)["'']?\s*$') {
        $name = $matches[1]
        $value = $matches[2]
        
        # Set environment variable for current session
        [Environment]::SetEnvironmentVariable($name, $value, "Process")
        Write-Host "  Set $name" -ForegroundColor Green
    }
}

Write-Host "`nEnvironment variables loaded!" -ForegroundColor Cyan
Write-Host "You can now run: go run ./cmd/orchestrator status" -ForegroundColor Yellow
