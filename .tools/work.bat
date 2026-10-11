@echo off
rem Launcher: runs the work_dot_bat binary. "work [args]" runs the work tool;
rem "work --run <subcommand> [args]" runs any other subcommand (auto_daemon, plan-status, ...).
rem Binary lookup: %WORK_DOT_BAT_BIN%, then work_dot_bat.exe on PATH, then a release build in ..\dist.
if defined WORK_DOT_BAT_BIN (
  set "BIN=%WORK_DOT_BAT_BIN%"
  goto run
)
where work_dot_bat.exe >nul 2>nul
if not errorlevel 1 (
  set "BIN=work_dot_bat.exe"
  goto run
)
set "ARCH=amd64"
if /i "%PROCESSOR_ARCHITECTURE%"=="ARM64" set "ARCH=arm64"
set "BIN=%~dp0..\dist\work_dot_bat_windows_%ARCH%.exe"
if not exist "%BIN%" (
  echo work_dot_bat binary not found. Put work_dot_bat.exe on PATH, set WORK_DOT_BAT_BIN, or run .tools\build-release.sh. 1>&2
  exit /b 1
)
:run
if /i "%~1"=="--run" goto run_subcommand
"%BIN%" work %*
exit /b %errorlevel%

:run_subcommand
set "REST="
for /f "tokens=1* delims= " %%a in ("%*") do set "REST=%%b"
"%BIN%" %REST%
exit /b %errorlevel%
