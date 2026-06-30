@echo off
cd /d c:\NAV\Cursor\NewA

call scripts\restart-auth-server.bat
if errorlevel 1 exit /b 1

call scripts\restart-calc-worker.bat
if errorlevel 1 exit /b 1

call scripts\restart-nginx.bat
if errorlevel 1 exit /b 1

echo [server] stopping old API on :8090...
for /f "tokens=5" %%a in ('netstat -ano ^| findstr :8090 ^| findstr LISTENING') do taskkill /PID %%a /F >nul 2>&1

echo [server] building...
go build -o data\nav-server.exe .\backend\cmd\server
if errorlevel 1 exit /b 1

set APP_ADDR=:8090
set APP_WEB_DIR=web
set APP_DATA_PATH=data\app.json
set APP_DATABASE_URL=user=postgres password=postgres dbname=postgres sslmode=disable
set APP_AUTH_DATABASE_URL=user=postgres password=postgres dbname=postgres sslmode=disable
set APP_JWT_SECRET=dev-secret-change-me
set APP_GSN_DATABASE_URL=user=postgres password=postgres dbname=postgres sslmode=disable

echo [server] starting on :8090 (public entry via nginx :8080)...
data\nav-server.exe
