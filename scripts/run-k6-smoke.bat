@echo off
setlocal

cd /d %~dp0..
call scripts\run-k6.bat scenarios\smoke.js
exit /b %ERRORLEVEL%
