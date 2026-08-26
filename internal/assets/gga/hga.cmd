@echo off
setlocal
set "HGA_PS1=%~dp0hga.ps1"
powershell -NoProfile -ExecutionPolicy Bypass -File "%HGA_PS1%" %*
exit /b %ERRORLEVEL%
