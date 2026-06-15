package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"nav-saas-mvp/backend/internal/auth"
	"nav-saas-mvp/backend/internal/domain"
	"nav-saas-mvp/backend/internal/gsn"
	"nav-saas-mvp/backend/internal/store"
)

type Server struct {
	store  *store.FileStore
	auth   *auth.Service
	gsn    *gsn.Service
	static http.Handler
}

type contextKey string

const claimsKey contextKey = "claims"

func NewServer(store *store.FileStore, authService *auth.Service, gsnService *gsn.Service, webDir string) *Server {
	return &Server{
		store:  store,
		auth:   authService,
		gsn:    gsnService,
		static: http.FileServer(http.Dir(webDir)),
	}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/login", s.handleLogin)
	mux.Handle("/api/me", s.withAuth(http.HandlerFunc(s.handleMe)))
	mux.Handle("/api/companies", s.withAuth(http.HandlerFunc(s.handleCompanies)))
	mux.Handle("/api/users", s.withAuth(http.HandlerFunc(s.handleUsers)))
	mux.Handle("/api/constructions", s.withAuth(http.HandlerFunc(s.handleConstructions)))
	mux.Handle("/api/objects", s.withAuth(http.HandlerFunc(s.handleObjects)))
	mux.Handle("/api/estimates", s.withAuth(http.HandlerFunc(s.handleEstimates)))
	mux.Handle("/api/estimates/", s.withAuth(http.HandlerFunc(s.handleEstimateByID)))
	mux.Handle("/api/gsn/hierarchy", s.withAuth(http.HandlerFunc(s.handleGSNHierarchy)))
	mux.Handle("/", s.static)

	return s.withCommonHeaders(mux)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	token, user, err := s.auth.Login(input.Email, input.Password)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"token": token,
		"user":  user,
	})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	claims := mustClaims(r)
	user, ok := s.store.FindUserByID(claims.UserID)
	if !ok {
		writeError(w, http.StatusUnauthorized, "user not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"user":   user,
		"claims": claims,
	})
}

func (s *Server) handleCompanies(w http.ResponseWriter, r *http.Request) {
	claims := mustClaims(r)
	switch r.Method {
	case http.MethodGet:
		companies := s.store.ListCompanies()
		if !claims.Role.CanManageCompanies() {
			filtered := companies[:0]
			for _, company := range companies {
				if company.ID == claims.CompanyID {
					filtered = append(filtered, company)
				}
			}
			companies = filtered
		}
		writeJSON(w, http.StatusOK, companies)

	case http.MethodPost:
		if !claims.Role.CanManageCompanies() {
			writeError(w, http.StatusForbidden, "only super admin can create companies")
			return
		}

		var input struct {
			Name string `json:"name"`
		}
		if err := readJSON(r, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}

		company, err := s.store.CreateCompany(input.Name)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, company)

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleUsers(w http.ResponseWriter, r *http.Request) {
	claims := mustClaims(r)
	if !claims.Role.CanManageUsers() {
		writeError(w, http.StatusForbidden, "not enough permissions")
		return
	}

	includeAll := claims.Role == domain.RoleSuperAdmin
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.store.ListUsers(claims.CompanyID, includeAll))

	case http.MethodPost:
		var input store.NewUser
		if err := readJSON(r, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}

		if !includeAll {
			input.CompanyID = claims.CompanyID
			if input.Role == domain.RoleSuperAdmin {
				writeError(w, http.StatusForbidden, "company admin cannot create super admins")
				return
			}
		}

		user, err := s.store.CreateUser(input)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, user)

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleConstructions(w http.ResponseWriter, r *http.Request) {
	claims := mustClaims(r)
	includeAll := claims.Role == domain.RoleSuperAdmin

	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.store.ListConstructions(claims.CompanyID, includeAll))

	case http.MethodPost:
		if !claims.Role.CanEditEstimates() {
			writeError(w, http.StatusForbidden, "not enough permissions")
			return
		}

		var input store.ConstructionInput
		if err := readJSON(r, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}

		construction, err := s.store.CreateConstruction(claims.CompanyID, input)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, construction)

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleObjects(w http.ResponseWriter, r *http.Request) {
	claims := mustClaims(r)
	includeAll := claims.Role == domain.RoleSuperAdmin

	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.store.ListObjects(claims.CompanyID, includeAll))

	case http.MethodPost:
		if !claims.Role.CanEditEstimates() {
			writeError(w, http.StatusForbidden, "not enough permissions")
			return
		}

		var input store.ConstructionObjectInput
		if err := readJSON(r, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}

		object, err := s.store.CreateObject(claims.CompanyID, includeAll, input)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, object)

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleEstimates(w http.ResponseWriter, r *http.Request) {
	claims := mustClaims(r)
	includeAll := claims.Role == domain.RoleSuperAdmin

	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.store.ListEstimates(claims.CompanyID, includeAll))

	case http.MethodPost:
		if !claims.Role.CanEditEstimates() {
			writeError(w, http.StatusForbidden, "not enough permissions")
			return
		}

		var input store.EstimateInput
		if err := readJSON(r, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}

		estimate, err := s.store.CreateEstimate(claims.CompanyID, input)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, estimate)

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleEstimateByID(w http.ResponseWriter, r *http.Request) {
	claims := mustClaims(r)
	id := strings.TrimPrefix(r.URL.Path, "/api/estimates/")
	if id == "" {
		writeError(w, http.StatusNotFound, "estimate not found")
		return
	}

	includeAll := claims.Role == domain.RoleSuperAdmin
	switch r.Method {
	case http.MethodPut:
		if !claims.Role.CanEditEstimates() {
			writeError(w, http.StatusForbidden, "not enough permissions")
			return
		}

		var input store.EstimateInput
		if err := readJSON(r, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}

		estimate, err := s.store.UpdateEstimate(id, claims.CompanyID, includeAll, input)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, estimate)

	case http.MethodDelete:
		if !claims.Role.CanEditEstimates() {
			writeError(w, http.StatusForbidden, "not enough permissions")
			return
		}

		if err := s.store.DeleteEstimate(id, claims.CompanyID, includeAll); err != nil {
			writeStoreError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleGSNHierarchy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	limit := 200
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid limit")
			return
		}
		limit = parsed
	}

	nodes, err := s.gsn.ListChildren(r.Context(), r.URL.Query().Get("parent"), limit)
	if err != nil {
		if errors.Is(err, gsn.ErrNotConfigured) {
			writeError(w, http.StatusServiceUnavailable, "GSN database is not configured; set APP_GSN_DATABASE_URL")
			return
		}
		slog.Error("gsn hierarchy query failed", "error", err)
		writeError(w, http.StatusBadGateway, "failed to read GSN hierarchy")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"nodes": nodes,
	})
}

func (s *Server) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		token, err := auth.BearerToken(r.Header.Get("Authorization"))
		if err != nil {
			writeError(w, http.StatusUnauthorized, "missing bearer token")
			return
		}

		claims, err := s.auth.Verify(token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid bearer token")
			return
		}

		ctx := context.WithValue(r.Context(), claimsKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) withCommonHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		next.ServeHTTP(w, r)
	})
}

func mustClaims(r *http.Request) domain.Claims {
	claims, ok := r.Context().Value(claimsKey).(domain.Claims)
	if !ok {
		slog.Warn("request reached protected handler without claims")
		return domain.Claims{}
	}
	return claims
}

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
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not found")
	case errors.Is(err, store.ErrForbidden):
		writeError(w, http.StatusForbidden, "not enough permissions")
	case errors.Is(err, store.ErrConflict):
		writeError(w, http.StatusBadRequest, "invalid or conflicting data")
	default:
		slog.Error("store operation failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}
