package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"nav-saas-mvp/backend/internal/auth"
	"nav-saas-mvp/backend/internal/authapi"
	"nav-saas-mvp/backend/internal/authstore"
)

func main() {
	ctx := context.Background()
	addr := env("APP_AUTH_ADDR", ":8081")
	dataPath := env("APP_DATA_PATH", filepath.Join("data", "app.json"))
	jwtSecret := env("APP_JWT_SECRET", "dev-secret-change-me")
	authDatabaseURL := env("APP_AUTH_DATABASE_URL", "")

	if strings.TrimSpace(authDatabaseURL) == "" {
		slog.Error("APP_AUTH_DATABASE_URL is required")
		os.Exit(1)
	}
	if jwtSecret == "dev-secret-change-me" {
		slog.Warn("using development JWT secret; set APP_JWT_SECRET for shared environments")
	}

	authStore, err := authstore.Bootstrap(ctx, authDatabaseURL, dataPath)
	if err != nil {
		slog.Error("failed to initialize auth database", "error", err)
		os.Exit(1)
	}
	defer authStore.Close()

	authService := auth.NewService(authStore, jwtSecret)
	server := authapi.New(authStore, authService)

	slog.Info("starting auth service", "addr", addr, "database", redactDatabaseURL(authDatabaseURL))
	if err := http.ListenAndServe(addr, server.Routes()); err != nil {
		slog.Error("auth service stopped", "error", err)
		os.Exit(1)
	}
}

func env(key, fallback string) string {
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
