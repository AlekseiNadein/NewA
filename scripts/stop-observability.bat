@echo off
cd /d %~dp0..\deploy\observability
echo [observability] stopping Prometheus + Grafana...
docker compose down
