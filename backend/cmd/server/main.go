package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"nav-saas-mvp/backend/internal/api"
	"nav-saas-mvp/backend/internal/auth"
	"nav-saas-mvp/backend/internal/authstore"
	"nav-saas-mvp/backend/internal/gsn"
	"nav-saas-mvp/backend/internal/store"
)

func main() {
	ctx := context.Background()
	addr := env("APP_ADDR", ":8090")
	dataPath := env("APP_DATA_PATH", filepath.Join("data", "app.json"))
	webDir := env("APP_WEB_DIR", "web")
	jwtSecret := env("APP_JWT_SECRET", "dev-secret-change-me")
	treeDatabaseURL := env("APP_DATABASE_URL", "")
	authDatabaseURL := env("APP_AUTH_DATABASE_URL", treeDatabaseURL)
	gsnDatabaseURL := env("APP_GSN_DATABASE_URL", treeDatabaseURL)

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
	slog.Info("auth contour: PostgreSQL", "url", redactDatabaseURL(authDatabaseURL))

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

	slog.Info("starting SaaS MVP server", "addr", addr, "webDir", webDir, "dataPath", dataPath)
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

func redactDatabaseURL(url string) string {
	parts := strings.Fields(url)
	for i, part := range parts {
		if strings.HasPrefix(part, "password=") {
			parts[i] = "password=***"
		}
	}
	return strings.Join(parts, " ")
}
