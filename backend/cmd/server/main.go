package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"nav-saas-mvp/backend/internal/api"
	"nav-saas-mvp/backend/internal/auth"
	"nav-saas-mvp/backend/internal/authstore"
	"nav-saas-mvp/backend/internal/gsn"
	"nav-saas-mvp/backend/internal/outbox"
	"nav-saas-mvp/backend/internal/observability"
	"nav-saas-mvp/backend/internal/store"
)

func main() {
	ctx := context.Background()
	observability.Init(observability.ConfigFromEnv("nav-api"))

	addr := env("APP_ADDR", ":8090")
	dataPath := env("APP_DATA_PATH", filepath.Join("data", "app.json"))
	webDir := env("APP_WEB_DIR", "web")
	jwtSecret := env("APP_JWT_SECRET", "dev-secret-change-me")
	treeDatabaseURL := env("APP_DATABASE_URL", "")
	authDatabaseURL := env("APP_AUTH_DATABASE_URL", treeDatabaseURL)
	gsnDatabaseURL := env("APP_GSN_DATABASE_URL", treeDatabaseURL)
	queueMode := strings.ToLower(strings.TrimSpace(env("APP_QUEUE_MODE", "db")))
	rabbitURL := env("APP_RABBITMQ_URL", "")
	rabbitExchange := env("APP_RABBITMQ_EXCHANGE", "estimate.calc")
	outboxBatch := envInt("APP_OUTBOX_PUBLISH_BATCH", 100)
	outboxInterval := envDuration("APP_OUTBOX_PUBLISH_INTERVAL", time.Second)

	if strings.TrimSpace(authDatabaseURL) == "" {
		slog.Error("APP_AUTH_DATABASE_URL is required")
		os.Exit(1)
	}
	if jwtSecret == "dev-secret-change-me" {
		slog.Warn("using development JWT secret; set APP_JWT_SECRET for shared environments")
	}

	fileStore, err := store.NewFileStore(dataPath, treeDatabaseURL)
	if err != nil {
		slog.Error("failed to initialize store", "error", err)
		os.Exit(1)
	}

	authStore, err := authstore.Bootstrap(ctx, authDatabaseURL, dataPath)
	if err != nil {
		slog.Error("failed to initialize auth database", "error", err)
		os.Exit(1)
	}
	defer authStore.Close()
	slog.Info("auth contour: PostgreSQL", "url", observability.RedactDatabaseURL(authDatabaseURL))

	authService := auth.NewVerifier(jwtSecret)
	slog.Info("auth API served by separate auth service (routed via nginx)")

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
	if !gsnService.Configured() {
		slog.Warn("GSN database is not configured; set APP_GSN_DATABASE_URL to enable Base -> GSN-2022")
	}

	server := api.NewServer(fileStore, authStore, authService, gsnService, webDir)
	if strings.TrimSpace(rabbitURL) != "" {
		go observability.RunQueueCollector(ctx, observability.QueueCollectorConfig{
			RabbitURL: rabbitURL,
			Interval:  30 * time.Second,
			Source:    fileStore,
		})
	}
	if (queueMode == "dual" || queueMode == "rabbit") && strings.TrimSpace(rabbitURL) != "" {
		go func() {
			slog.Info("starting outbox publisher", "exchange", rabbitExchange, "batch", outboxBatch, "interval", outboxInterval.String())
			if err := outbox.RunPublisher(ctx, fileStore, outbox.PublisherConfig{
				RabbitURL: rabbitURL,
				Exchange:  rabbitExchange,
				BatchSize: outboxBatch,
				Interval:  outboxInterval,
			}); err != nil {
				slog.Error("outbox publisher stopped", "error", err)
			}
		}()
	}

	slog.Info("starting SaaS MVP server", "addr", addr, "webDir", webDir, "dataPath", dataPath, "queueMode", queueMode)
	if err := http.ListenAndServe(addr, server.Routes()); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
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

func envDuration(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	return value
}
