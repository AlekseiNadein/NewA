@echo off
cd /d %~dp0..
set APP_DATA_PATH=data\app.json
set APP_DATABASE_URL=user=postgres password=postgres dbname=postgres sslmode=disable
set APP_GSN_DATABASE_URL=user=postgres password=postgres dbname=postgres sslmode=disable
set APP_QUEUE_MODE=rabbit
set APP_RABBITMQ_URL=amqp://guest:guest@127.0.0.1:5672/
set APP_RABBITMQ_EXCHANGE=estimate.calc
set APP_RABBITMQ_PREFETCH=4
data\nav-calc-worker.exe >> data\calc-worker.log 2>&1
