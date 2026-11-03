@echo off
call "%~dp0work.bat" --run monitor %*
exit /b %errorlevel%
