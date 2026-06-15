package main

import (
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"nav-saas-mvp/backend/internal/api"
	"nav-saas-mvp/backend/internal/auth"
	"nav-saas-mvp/backend/internal/gsn"
	"nav-saas-mvp/backend/internal/store"
)

func main() {
	addr := env("APP_ADDR", ":8080")
	dataPath := env("APP_DATA_PATH", filepath.Join("data", "app.json"))
	webDir := env("APP_WEB_DIR", "web")
	jwtSecret := env("APP_JWT_SECRET", "dev-secret-change-me")
	gsnDatabaseURL := env("APP_GSN_DATABASE_URL", "")

	if jwtSecret == "dev-secret-change-me" {
		slog.Warn("using development JWT secret; set APP_JWT_SECRET for shared environments")
	}

	fileStore, err := store.NewFileStore(dataPath)
	if err != nil {
		slog.Error("failed to initialize store", "error", err)
		os.Exit(1)
	}

	authService := auth.NewService(fileStore, jwtSecret)
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

	server := api.NewServer(fileStore, authService, gsnService, webDir)

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
