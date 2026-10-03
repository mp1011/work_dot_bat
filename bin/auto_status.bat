@echo off
call "%~dp0work_dot_bat.bat" auto_status %*
exit /b %errorlevel%
