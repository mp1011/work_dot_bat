@echo off
call "%~dp0work_dot_bat.bat" create_worktree %*
exit /b %errorlevel%
