@echo off
cd /d %~dp0..
set APP_AUTH_ADDR=:8081
set APP_DATA_PATH=data\app.json
set APP_AUTH_DATABASE_URL=user=postgres password=postgres dbname=postgres sslmode=disable
set APP_RABBITMQ_EXCHANGE=estimate.calc
set APP_RABBITMQ_PREFETCH=4
set APP_LOG_LEVEL=info
set APP_LOG_FORMAT=json
set APP_SERVICE_NAME=nav-auth
set APP_LOG_FILE=data\nav-auth.log
set APP_METRICS_ADDR=127.0.0.1:9091
set OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:4318
set OTEL_SERVICE_NAME=nav-auth
data\nav-auth-server.exe
