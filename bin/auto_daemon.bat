@echo off
call "%~dp0work_dot_bat.bat" auto_daemon %*
exit /b %errorlevel%
