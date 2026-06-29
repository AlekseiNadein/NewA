@echo off
cd /d c:\NAV\Cursor\NewA
go build -o data\nav-calc-worker.exe .\backend\cmd\calc_worker
if errorlevel 1 exit /b 1
set APP_DATA_PATH=data\app.json
set APP_DATABASE_URL=user=postgres password=postgres dbname=postgres sslmode=disable
set APP_GSN_DATABASE_URL=user=postgres password=postgres dbname=postgres sslmode=disable
data\nav-calc-worker.exe
