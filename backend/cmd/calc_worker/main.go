package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"nav-saas-mvp/backend/internal/calcworker"
	"nav-saas-mvp/backend/internal/gsn"
	"nav-saas-mvp/backend/internal/observability"
	"nav-saas-mvp/backend/internal/store"
)

func main() {
	observability.Init(observability.ConfigFromEnv("nav-calc-worker"))

	dataPath := env("APP_DATA_PATH", filepath.Join("data", "app.json"))
	treeDatabaseURL := env("APP_DATABASE_URL", "")
	gsnDatabaseURL := env("APP_GSN_DATABASE_URL", treeDatabaseURL)
	queueMode := strings.ToLower(strings.TrimSpace(env("APP_QUEUE_MODE", "db")))
	rabbitURL := env("APP_RABBITMQ_URL", "")
	rabbitExchange := env("APP_RABBITMQ_EXCHANGE", "estimate.calc")
	rabbitPrefetch := calcworker.NormalizeRabbitPrefetch(envInt("APP_RABBITMQ_PREFETCH", 4))

	fileStore, err := store.NewFileStore(dataPath, treeDatabaseURL)
	if err != nil {
		slog.Error("failed to initialize store", "error", err)
		os.Exit(1)
	}

	gsnService, err := gsn.NewService(gsnDatabaseURL)
	if err != nil {
		slog.Error("failed to initialize GSN database", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := gsnService.Close(); err != nil {
			slog.Warn("failed to close GSN database", "error", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if strings.TrimSpace(rabbitURL) != "" {
		go observability.RunQueueCollector(ctx, observability.QueueCollectorConfig{
			RabbitURL: rabbitURL,
			Interval:  30 * time.Second,
		})
	}

	slog.Info("starting estimate calc worker service", "dataPath", dataPath, "queueMode", queueMode, "rabbitPrefetch", rabbitPrefetch)
	if queueMode == "rabbit" {
		worker := calcworker.New(fileStore, gsnService)
		if strings.TrimSpace(rabbitURL) == "" {
			slog.Error("APP_RABBITMQ_URL is required in rabbit mode")
			os.Exit(1)
		}
		gsnService.SetMaxOpenConns(rabbitPrefetch + 4)
		if err := worker.RunRabbit(ctx, calcworker.RabbitConfig{
			URL:        rabbitURL,
			Exchange:   rabbitExchange,
			Prefetch:   rabbitPrefetch,
		}); err != nil {
			slog.Error("rabbit calc worker stopped", "error", err)
			os.Exit(1)
		}
		return
	}
	if queueMode == "db" {
		slog.Warn("calc worker uses legacy DB queue; set APP_QUEUE_MODE=rabbit for production")
	}
	calcworker.NewManager(fileStore, gsnService).Run(ctx)
}

func env(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func envInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}
