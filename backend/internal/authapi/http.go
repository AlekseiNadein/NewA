package authapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"nav-saas-mvp/backend/internal/authstore"
	"nav-saas-mvp/backend/internal/store"
)

func readJSON(r *http.Request, target any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(target)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		slog.Warn("failed to write json response", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound), errors.Is(err, authstore.ErrNotFound):
		writeError(w, http.StatusNotFound, "not found")
	case errors.Is(err, store.ErrForbidden), errors.Is(err, authstore.ErrForbidden):
		writeError(w, http.StatusForbidden, "not enough permissions")
	case errors.Is(err, store.ErrConflict), errors.Is(err, authstore.ErrConflict):
		writeError(w, http.StatusBadRequest, "некорректные или конфликтующие данные")
	default:
		slog.Error("auth store operation failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}
