@echo off
call "%~dp0work.bat" --run prepare_for_push %*
exit /b %errorlevel%
