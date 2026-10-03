@echo off
call "%~dp0work_dot_bat.bat" prepare_for_push %*
exit /b %errorlevel%
