# syntax=docker/dockerfile:1

FROM golang:1.25-bookworm AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY backend ./backend

ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/nav-api ./backend/cmd/server && \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/nav-auth ./backend/cmd/auth_server && \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/nav-calc-worker ./backend/cmd/calc_worker

FROM debian:bookworm-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates && \
    rm -rf /var/lib/apt/lists/* && \
    groupadd --gid 10001 nav && \
    useradd --uid 10001 --gid nav --no-create-home --home-dir /app nav && \
    mkdir -p /app/runtime && \
    chown -R nav:nav /app

WORKDIR /app

COPY --from=build /out/ /app/bin/
COPY web /app/web
COPY db /app/db

USER 10001:10001

ENV APP_WEB_DIR=/app/web \
    APP_DATA_PATH=/app/runtime/app.json \
    APP_LOG_FORMAT=json \
    APP_LOG_LEVEL=info

EXPOSE 8081 8090 9090 9091 9092

CMD ["/app/bin/nav-api"]
