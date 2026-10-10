@echo off
rem Automatically adds work_dot_bat\.tools to the User's PATH.
setlocal
set "SCRIPT_DIR=%~dp0"

where pwsh >nul 2>nul
if not errorlevel 1 (
  pwsh -NoProfile -ExecutionPolicy Bypass -File "%SCRIPT_DIR%add-to-path.ps1" %*
  exit /b %errorlevel%
)

where powershell >nul 2>nul
if not errorlevel 1 (
  powershell -NoProfile -ExecutionPolicy Bypass -File "%SCRIPT_DIR%add-to-path.ps1" %*
  exit /b %errorlevel%
)

echo Neither pwsh nor powershell was found on PATH. 1>&2
exit /b 1
