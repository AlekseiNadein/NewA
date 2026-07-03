@echo off
cd /d %~dp0..
set APP_DATA_PATH=data\app.json
set APP_DATABASE_URL=user=postgres password=postgres dbname=postgres sslmode=disable
set APP_GSN_DATABASE_URL=user=postgres password=postgres dbname=postgres sslmode=disable
set APP_QUEUE_MODE=rabbit
set APP_RABBITMQ_URL=amqp://guest:guest@127.0.0.1:5672/
set APP_RABBITMQ_EXCHANGE=estimate.calc
set APP_RABBITMQ_PREFETCH=4
set APP_LOG_LEVEL=info
set APP_LOG_FORMAT=json
set APP_SERVICE_NAME=nav-calc-worker
set APP_LOG_FILE=data\calc-worker.log
set APP_METRICS_ADDR=127.0.0.1:9092
set OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:4318
set OTEL_SERVICE_NAME=nav-calc-worker
data\nav-calc-worker.exe
