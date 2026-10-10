@echo off
call "%~dp0work.bat" --run auto_daemon %*
exit /b %errorlevel%
