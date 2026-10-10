@echo off
call "%~dp0work.bat" --run create_worktree %*
exit /b %errorlevel%
