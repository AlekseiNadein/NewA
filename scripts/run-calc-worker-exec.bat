@echo off
cd /d %~dp0..
set APP_DATA_PATH=data\app.json
set APP_DATABASE_URL=user=postgres password=postgres dbname=postgres sslmode=disable
set APP_GSN_DATABASE_URL=user=postgres password=postgres dbname=postgres sslmode=disable
data\nav-calc-worker.exe >> data\calc-worker.log 2>&1
