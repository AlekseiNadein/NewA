@echo off
cd /d c:\NAV\Cursor\NewA
set APP_ADDR=:8080
set APP_WEB_DIR=web
set APP_DATA_PATH=data\app.json
set APP_GSN_DATABASE_URL=user=postgres password=postgres dbname=postgres sslmode=disable
data\nav-server.exe