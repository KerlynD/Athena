# Athena Daily Task Scheduler Setup for Windows
# Run this script as Administrator to create scheduled task

$ErrorActionPreference = "Stop"

# Configuration
$TaskName = "Athena-DailyAnalysis"
$TaskDescription = "Run Athena market intelligence aggregator daily at 7:00 AM"
$ProjectPath = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
$RunTime = "07:00"

Write-Host "Athena Scheduler Setup" -ForegroundColor Cyan
Write-Host "======================" -ForegroundColor Cyan
Write-Host ""

# Check if running as admin
$isAdmin = ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) {
    Write-Host "WARNING: Not running as Administrator. Task may not be created properly." -ForegroundColor Yellow
    Write-Host "Consider running: Start-Process powershell -Verb RunAs -ArgumentList '-File', '$($MyInvocation.MyCommand.Path)'" -ForegroundColor Yellow
    Write-Host ""
}

# Create the daily run script that the scheduler will execute
$DailyScriptPath = Join-Path $ProjectPath "scripts\scheduled_run.ps1"
$DailyScriptContent = @"
# Athena Scheduled Daily Run
# This script is executed by Windows Task Scheduler

`$ErrorActionPreference = "Continue"
`$ProjectPath = "$ProjectPath"

# Log file
`$LogDir = Join-Path `$ProjectPath "logs"
if (-not (Test-Path `$LogDir)) {
    New-Item -ItemType Directory -Path `$LogDir | Out-Null
}
`$LogFile = Join-Path `$LogDir "athena_`$(Get-Date -Format 'yyyy-MM-dd').log"

function Log {
    param([string]`$Message)
    `$timestamp = Get-Date -Format "yyyy-MM-dd HH:mm:ss"
    "`$timestamp - `$Message" | Tee-Object -FilePath `$LogFile -Append
}

Log "========================================="
Log "Athena Daily Run Starting"
Log "========================================="

# Change to project directory
Set-Location `$ProjectPath

# Load environment variables
`$EnvFile = Join-Path `$ProjectPath ".envrc"
if (Test-Path `$EnvFile) {
    Log "Loading environment from .envrc"
    Get-Content `$EnvFile | ForEach-Object {
        if (`$_ -match '^export\s+([^=]+)=(.*)$') {
            `$key = `$matches[1]
            `$value = `$matches[2] -replace '^["'']|["'']$', ''
            [Environment]::SetEnvironmentVariable(`$key, `$value, "Process")
        }
    }
}

# Step 1: Fetch market data
Log "Step 1: Fetching market data..."
try {
    & go run ./cmd/orchestrator fetch-market 2>&1 | ForEach-Object { Log `$_ }
    Log "Market data fetch complete"
} catch {
    Log "ERROR: Market data fetch failed: `$_"
}

# Step 2: Run analysis
Log "Step 2: Running analysis..."
try {
    # Activate Python venv if it exists
    `$VenvActivate = Join-Path `$ProjectPath "venv\Scripts\Activate.ps1"
    if (Test-Path `$VenvActivate) {
        . `$VenvActivate
    }
    
    & go run ./cmd/orchestrator analyze 2>&1 | ForEach-Object { Log `$_ }
    Log "Analysis complete"
} catch {
    Log "ERROR: Analysis failed: `$_"
}

Log "========================================="
Log "Athena Daily Run Complete"
Log "========================================="
"@

Write-Host "Creating scheduled run script at: $DailyScriptPath" -ForegroundColor Green
Set-Content -Path $DailyScriptPath -Value $DailyScriptContent -Encoding UTF8

# Create the scheduled task
Write-Host ""
Write-Host "Creating scheduled task: $TaskName" -ForegroundColor Green

# Check if task already exists
$existingTask = Get-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
if ($existingTask) {
    Write-Host "Task already exists. Removing old task..." -ForegroundColor Yellow
    Unregister-ScheduledTask -TaskName $TaskName -Confirm:$false
}

# Create action
$Action = New-ScheduledTaskAction -Execute "powershell.exe" -Argument "-ExecutionPolicy Bypass -WindowStyle Hidden -File `"$DailyScriptPath`""

# Create trigger (daily at 7 AM)
$Trigger = New-ScheduledTaskTrigger -Daily -At $RunTime

# Create settings
$Settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -StartWhenAvailable -RunOnlyIfNetworkAvailable

# Create the task
try {
    Register-ScheduledTask -TaskName $TaskName -Description $TaskDescription -Action $Action -Trigger $Trigger -Settings $Settings -RunLevel Highest
    Write-Host ""
    Write-Host "SUCCESS: Scheduled task created!" -ForegroundColor Green
    Write-Host ""
    Write-Host "Task Details:" -ForegroundColor Cyan
    Write-Host "  Name: $TaskName"
    Write-Host "  Runs: Daily at $RunTime"
    Write-Host "  Script: $DailyScriptPath"
    Write-Host "  Logs: $ProjectPath\logs\"
    Write-Host ""
    Write-Host "To test the task now, run:" -ForegroundColor Yellow
    Write-Host "  Start-ScheduledTask -TaskName '$TaskName'"
    Write-Host ""
    Write-Host "To view task status:" -ForegroundColor Yellow
    Write-Host "  Get-ScheduledTask -TaskName '$TaskName' | Get-ScheduledTaskInfo"
    Write-Host ""
    Write-Host "To remove the task:" -ForegroundColor Yellow
    Write-Host "  Unregister-ScheduledTask -TaskName '$TaskName' -Confirm:`$false"
} catch {
    Write-Host "ERROR: Failed to create scheduled task: $_" -ForegroundColor Red
    Write-Host ""
    Write-Host "You may need to run this script as Administrator." -ForegroundColor Yellow
}
