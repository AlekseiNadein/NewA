@echo off
cd /d c:\NAV\Cursor\NewA
go build -o data\nav-server.exe .\backend\cmd\server
if errorlevel 1 exit /b 1
set APP_ADDR=:8080
set APP_WEB_DIR=web
set APP_DATA_PATH=data\app.json
set APP_DATABASE_URL=user=postgres password=postgres dbname=postgres sslmode=disable
set APP_GSN_DATABASE_URL=user=postgres password=postgres dbname=postgres sslmode=disable
data\nav-server.exe