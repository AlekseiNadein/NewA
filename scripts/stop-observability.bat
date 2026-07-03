@echo off
cd /d %~dp0..\deploy\observability
echo [observability] stopping Loki + Alloy + Prometheus + Grafana...
docker compose down
