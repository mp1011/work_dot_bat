@echo off
rem Launcher: runs the work_dot_bat binary with the given tool and arguments.
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
"%BIN%" %*
exit /b %errorlevel%
