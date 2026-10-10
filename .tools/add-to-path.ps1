<#
.SYNOPSIS
    Adds work_dot_bat\.tools to the user's PATH environment variable.

.DESCRIPTION
    Permanently appends or prepends the repository's .tools/ directory to the
    current User's PATH in the Windows registry, and updates the current
    PowerShell session so the commands are immediately accessible.

    Commands made available globally:
      - work
      - work_dot_bat
      - auto_daemon
      - auto_status
      - create_worktree
      - prepare_for_push

.PARAMETER Scope
    Environment variable scope: 'User' (default) or 'Machine'.
    Note: 'Machine' requires running PowerShell as Administrator.

.PARAMETER SetWorkDotBatBin
    If specified, also sets the WORK_DOT_BAT_BIN environment variable
    pointing directly to the built binary in dist/.

.PARAMETER Prepend
    If specified, places bin/ at the beginning of PATH instead of the end.

.PARAMETER DryRun
    Show what would be changed without modifying any environment variables.
#>
[CmdletBinding()]
param(
    [ValidateSet('User', 'Machine')]
    [string]$Scope = 'User',

    [switch]$SetWorkDotBatBin,

    [switch]$Prepend,

    [switch]$DryRun
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

# 1. Resolve directory paths
$RepoRoot = (Resolve-Path "$PSScriptRoot\..").Path
$BinDir = Join-Path $RepoRoot ".tools"

if (-not (Test-Path -LiteralPath $BinDir)) {
    Write-Error ".tools directory not found at: $BinDir"
    return
}

Write-Host "work_dot_bat repository root: $RepoRoot"
Write-Host "Target directory to add to PATH: $BinDir"

# 2. Check current PATH in the requested scope
$targetScope = if ($Scope -eq 'Machine') { [EnvironmentVariableTarget]::Machine } else { [EnvironmentVariableTarget]::User }
$rawPath = [Environment]::GetEnvironmentVariable('Path', $targetScope)
$existingEntries = if ($rawPath) {
    $rawPath -split ';' | Where-Object { [string]::IsNullOrWhiteSpace($_) -eq $false }
} else {
    @()
}

$alreadyInScope = $false
foreach ($entry in $existingEntries) {
    if ($entry.TrimEnd('\/').Equals($BinDir.TrimEnd('\/'), [System.StringComparison]::OrdinalIgnoreCase)) {
        $alreadyInScope = $true
        break
    }
}

if ($alreadyInScope) {
    Write-Host "[OK] $BinDir is already in $Scope PATH." -ForegroundColor Green
} else {
    if ($Prepend) {
        $newEntries = @($BinDir) + $existingEntries
    } else {
        $newEntries = $existingEntries + @($BinDir)
    }
    $newPathValue = ($newEntries | Select-Object -Unique) -join ';'

    if ($DryRun) {
        Write-Host "[DRY RUN] Would update $Scope PATH to include: $BinDir" -ForegroundColor Yellow
    } else {
        [Environment]::SetEnvironmentVariable('Path', $newPathValue, $targetScope)
        Write-Host "[SUCCESS] Added $BinDir to $Scope PATH." -ForegroundColor Green
    }
}

# 3. Update current process PATH so commands work immediately in this session
$currentProcessEntries = ($env:PATH -split ';' | Where-Object { [string]::IsNullOrWhiteSpace($_) -eq $false })
$alreadyInProcess = $false
foreach ($entry in $currentProcessEntries) {
    if ($entry.TrimEnd('\/').Equals($BinDir.TrimEnd('\/'), [System.StringComparison]::OrdinalIgnoreCase)) {
        $alreadyInProcess = $true
        break
    }
}

if (-not $alreadyInProcess) {
    if ($DryRun) {
        Write-Host "[DRY RUN] Would update current session PATH." -ForegroundColor Yellow
    } else {
        if ($Prepend) {
            $env:PATH = "$BinDir;$env:PATH"
        } else {
            $env:PATH = "$env:PATH;$BinDir"
        }
        Write-Host "[OK] Updated current process session PATH." -ForegroundColor Cyan
    }
}

# 4. Optional: Set WORK_DOT_BAT_BIN
if ($SetWorkDotBatBin) {
    $arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'amd64' }
    $binaryPath = Join-Path $RepoRoot "dist\work_dot_bat_windows_$arch.exe"
    if (Test-Path -LiteralPath $binaryPath) {
        if ($DryRun) {
            Write-Host "[DRY RUN] Would set WORK_DOT_BAT_BIN to $binaryPath" -ForegroundColor Yellow
        } else {
            [Environment]::SetEnvironmentVariable('WORK_DOT_BAT_BIN', $binaryPath, $targetScope)
            $env:WORK_DOT_BAT_BIN = $binaryPath
            Write-Host "[SUCCESS] Set WORK_DOT_BAT_BIN ($Scope) to: $binaryPath" -ForegroundColor Green
        }
    } else {
        Write-Warning "Binary not found at $binaryPath; skipping WORK_DOT_BAT_BIN."
    }
}

# 5. Verification
Write-Host "`nVerifying command availability:" -ForegroundColor Cyan
$cmd = Get-Command "work_dot_bat.bat" -ErrorAction SilentlyContinue
if (-not $cmd) {
    $cmd = Get-Command "work_dot_bat" -ErrorAction SilentlyContinue
}

if ($cmd) {
    Write-Host "  work_dot_bat resolved to: $($cmd.Source)" -ForegroundColor Green
    try {
        $version = & $cmd.Source --version 2>&1
        Write-Host "  work_dot_bat output: $version" -ForegroundColor Green
    } catch {
        Write-Warning "  Could not run $($cmd.Source): $_"
    }
} else {
    Write-Host "  Note: You may need to restart your terminal for newly spawned shells to reflect PATH changes." -ForegroundColor Yellow
}
