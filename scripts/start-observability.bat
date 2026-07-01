@echo off
cd /d %~dp0..\deploy\observability
echo [observability] starting Prometheus + Grafana...
docker compose up -d
if errorlevel 1 exit /b 1
echo [observability] Grafana:  http://localhost:3000  (admin / admin)
echo [observability] Prometheus: http://localhost:9093
