@echo off
cd /d %~dp0..
set "PROJECT_STATUS_ROOT=C:\Codex\ProjectStatus"
set PORT=8100
set APP_DATABASE_URL=user=postgres password=postgres dbname=postgres sslmode=disable
set APP_JWT_SECRET=dev-secret-change-me
set APP_WEB_DIR=%PROJECT_STATUS_ROOT%\web
data\project-status.exe
