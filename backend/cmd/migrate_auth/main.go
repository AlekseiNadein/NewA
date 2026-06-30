package main

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"

	"nav-saas-mvp/backend/internal/authstore"
)

func main() {
	ctx := context.Background()
	authURL := env("APP_AUTH_DATABASE_URL", "")
	if authURL == "" {
		slog.Error("APP_AUTH_DATABASE_URL is required")
		os.Exit(1)
	}

	dataPath := env("APP_DATA_PATH", filepath.Join("data", "app.json"))

	store, err := authstore.New(ctx, authURL)
	if err != nil {
		slog.Error("failed to connect auth database", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	users, companies, err := store.ImportFromAppJSONFile(ctx, dataPath)
	if err != nil {
		slog.Error("import failed", "error", err, "path", dataPath)
		os.Exit(1)
	}

	slog.Info("auth import complete", "users", users, "companies", companies, "source", dataPath)
}

func env(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
