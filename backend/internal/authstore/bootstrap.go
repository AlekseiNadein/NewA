package authstore

import (
	"context"
	"log/slog"
)

// Bootstrap opens the auth PG contour and imports seed data when empty.
func Bootstrap(ctx context.Context, databaseURL, appJSONPath string) (*Store, error) {
	store, err := New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}

	imported, err := store.ImportIfEmpty(ctx, appJSONPath)
	if err != nil {
		store.Close()
		return nil, err
	}
	if imported {
		slog.Info("imported users and companies from app.json into auth database", "path", appJSONPath)
	}
	if err := store.SeedDefault(ctx); err != nil {
		store.Close()
		return nil, err
	}
	return store, nil
}
