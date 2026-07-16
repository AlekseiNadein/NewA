@echo off
cd /d %~dp0..\deploy\observability
echo [observability] starting Redis + Loki + Alloy + Tempo + OTel Collector + Prometheus + Grafana...
docker compose up -d
if errorlevel 1 exit /b 1
echo [observability] Redis:            redis://127.0.0.1:6379/0
echo [observability] Grafana:         http://localhost:3000  (admin / admin)
echo [observability] Prometheus:      http://localhost:9093
echo [observability] Loki:            http://localhost:3100
echo [observability] Tempo:           http://localhost:3200
echo [observability] OTLP HTTP/gRPC:  http://localhost:4318 / localhost:4317
echo [observability] k6 load tests:   scripts\run-k6-smoke.bat / scripts\run-k6-load.bat [VUs]
