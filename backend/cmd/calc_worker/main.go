package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"nav-saas-mvp/backend/internal/calcworker"
	"nav-saas-mvp/backend/internal/gsn"
	"nav-saas-mvp/backend/internal/store"
)

func main() {
	dataPath := env("APP_DATA_PATH", filepath.Join("data", "app.json"))
	treeDatabaseURL := env("APP_DATABASE_URL", "")
	gsnDatabaseURL := env("APP_GSN_DATABASE_URL", treeDatabaseURL)

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

	slog.Info("starting estimate calc worker service", "dataPath", dataPath)
	calcworker.NewManager(fileStore, gsnService).Run(ctx)
}

func env(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
