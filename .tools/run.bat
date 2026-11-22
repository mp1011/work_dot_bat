@echo off
call "%~dp0work.bat" --run run %*
exit /b %errorlevel%
