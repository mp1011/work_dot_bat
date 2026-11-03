@echo off
call "%~dp0work.bat" --run quota %*
exit /b %errorlevel%
