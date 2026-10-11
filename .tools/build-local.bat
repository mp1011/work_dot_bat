@echo off
rem Windows equivalent of build-local.sh (no bash needed); keeps the window open so the result can be read.
cd /d "%~dp0.."
go build -o dist\work_dot_bat_windows_amd64.exe .\src\cmd\work_dot_bat
set RC=%ERRORLEVEL%
if %RC% neq 0 (echo. & echo BUILD FAILED, exit code %RC%. If the file is in use, quit every running work session and retry.) else (echo. & echo BUILD OK: dist\work_dot_bat_windows_amd64.exe)
pause
exit /b %RC%
