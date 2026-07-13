package api

import (
	"net/http"

	"nav-saas-mvp/backend/internal/domain"
)

func (s *Server) handleUserPositions(w http.ResponseWriter, r *http.Request) {
	claims := mustClaims(r)

	switch r.Method {
	case http.MethodGet:
		items, err := s.store.ListUserPositions(r.Context(), claims.CompanyID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read user positions")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	case http.MethodPut:
		var input struct {
			Items []domain.UserPosition `json:"items"`
		}
		if err := readJSON(r, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		items, err := s.store.ReplaceUserPositions(r.Context(), claims.CompanyID, input.Items)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to save user positions")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
