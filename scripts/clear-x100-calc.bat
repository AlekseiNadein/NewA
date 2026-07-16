@echo off
setlocal
cd /d "%~dp0.."
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0clear-x100-calc.ps1" %*
exit /b %ERRORLEVEL%
