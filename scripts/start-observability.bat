@echo off
cd /d %~dp0..\deploy\observability
echo [observability] starting Loki + Alloy + Prometheus + Grafana...
docker compose up -d
if errorlevel 1 exit /b 1
echo [observability] Grafana:    http://localhost:3000  (admin / admin)
echo [observability] Prometheus: http://localhost:9093
echo [observability] Loki:       http://localhost:3100
