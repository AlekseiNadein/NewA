@echo off
cd /d %~dp0..
set APP_AUTH_ADDR=:8081
set APP_DATA_PATH=data\app.json
set APP_AUTH_DATABASE_URL=user=postgres password=postgres dbname=postgres sslmode=disable
set APP_JWT_SECRET=dev-secret-change-me
data\nav-auth-server.exe
