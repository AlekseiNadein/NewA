@echo off
cd /d %~dp0..
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0report-x100-totals.ps1" %*
exit /b %ERRORLEVEL%
