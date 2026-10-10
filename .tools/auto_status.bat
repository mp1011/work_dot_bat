@echo off
call "%~dp0work.bat" --run auto_status %*
exit /b %errorlevel%
