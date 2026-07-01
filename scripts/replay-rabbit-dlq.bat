@echo off
setlocal
cd /d %~dp0..

set APP_RABBITMQ_URL=amqp://guest:guest@127.0.0.1:5672/
set APP_RABBITMQ_EXCHANGE=estimate.calc

set LIMIT=100
if not "%~1"=="" set LIMIT=%~1

echo [rabbit] replaying up to %LIMIT% message(s) from DLQ...
go run .\backend\cmd\replay_dlq -limit %LIMIT%
exit /b %ERRORLEVEL%
