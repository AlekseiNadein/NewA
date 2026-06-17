package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"path/filepath"
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
	webDir string
	static http.Handler
}

type contextKey string

const claimsKey contextKey = "claims"

func NewServer(store *store.FileStore, authService *auth.Service, gsnService *gsn.Service, webDir string) *Server {
	return &Server{
		store:  store,
		auth:   authService,
		gsn:    gsnService,
		webDir: webDir,
		static: http.FileServer(http.Dir(webDir)),
	}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/login", s.handleLogin)
	mux.HandleFunc("/api/auth/register", s.handleRegister)
	mux.Handle("/api/me", s.withAuth(http.HandlerFunc(s.handleMe)))
	mux.Handle("/api/companies", s.withAuth(http.HandlerFunc(s.handleCompanies)))
	mux.Handle("/api/users", s.withAuth(s.withAdmin(http.HandlerFunc(s.handleUsers))))
	mux.Handle("/api/users/", s.withAuth(s.withAdmin(http.HandlerFunc(s.handleUserByID))))
	mux.Handle("/api/constructions", s.withAuth(s.withAuthorized(http.HandlerFunc(s.handleConstructions))))
	mux.Handle("/api/constructions/", s.withAuth(s.withAuthorized(http.HandlerFunc(s.handleConstructionByID))))
	mux.Handle("/api/objects", s.withAuth(s.withAuthorized(http.HandlerFunc(s.handleObjects))))
	mux.Handle("/api/objects/", s.withAuth(s.withAuthorized(http.HandlerFunc(s.handleObjectByID))))
	mux.Handle("/api/estimates", s.withAuth(s.withAuthorized(http.HandlerFunc(s.handleEstimates))))
	mux.Handle("/api/estimates/", s.withAuth(s.withAuthorized(http.HandlerFunc(s.handleEstimateByID))))
	mux.Handle("/api/gsn/supplements", s.withAuth(s.withAuthorized(http.HandlerFunc(s.handleGSNSupplements))))
	mux.Handle("/api/gsn/base-info", s.withAuth(s.withAuthorized(http.HandlerFunc(s.handleGSNBaseInfo))))
	mux.Handle("/api/gsn/hierarchy", s.withAuth(s.withAuthorized(http.HandlerFunc(s.handleGSNHierarchy))))
	mux.HandleFunc("/admin", s.serveAdmin)
	mux.HandleFunc("/admin/", s.serveAdmin)
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
		CompanyName string `json:"companyName"`
		Name        string `json:"name"`
		Password    string `json:"password"`
	}
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	token, user, err := s.auth.Login(input.CompanyName, input.Name, input.Password)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrNotAuthorized):
			writeError(w, http.StatusForbidden, "учётная запись ожидает подтверждения администратора")
		default:
			writeError(w, http.StatusUnauthorized, "неверная компания, ФИО или пароль")
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"token": token,
		"user":  user,
		"access": map[string]bool{
			"app":   user.CanAccessApp(),
			"admin": user.CanAccessAdmin(),
		},
	})
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var input store.RegisterUser
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	if input.Password != input.PasswordConfirm {
		writeError(w, http.StatusBadRequest, "пароли не совпадают")
		return
	}

	user, err := s.store.RegisterUser(input)
	if err != nil {
		writeStoreError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"user":    user,
		"message": "заявка на регистрацию отправлена, ожидайте подтверждения администратора",
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
		"user": user,
		"access": map[string]bool{
			"app":   user.CanAccessApp(),
			"admin": user.CanAccessAdmin(),
		},
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
			input.CompanyName = ""
			input.IsSuperAdministrator = false
		}

		user, err := s.store.CreateUser(input)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, s.store.UserView(user))

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleUserByID(w http.ResponseWriter, r *http.Request) {
	claims := mustClaims(r)
	id := strings.TrimPrefix(r.URL.Path, "/api/users/")
	if id == "" {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	includeAll := claims.Role == domain.RoleSuperAdmin
	switch r.Method {
	case http.MethodPut:
		var input store.UpdateUser
		if err := readJSON(r, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}

		user, err := s.store.UpdateUser(id, claims.CompanyID, includeAll, input)
		if err != nil {
			writeStoreError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, s.store.UserView(user))

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

func (s *Server) handleConstructionByID(w http.ResponseWriter, r *http.Request) {
	claims := mustClaims(r)
	id := strings.TrimPrefix(r.URL.Path, "/api/constructions/")
	if id == "" {
		writeError(w, http.StatusNotFound, "construction not found")
		return
	}

	includeAll := claims.Role == domain.RoleSuperAdmin
	switch r.Method {
	case http.MethodPut:
		if !claims.Role.CanEditEstimates() {
			writeError(w, http.StatusForbidden, "not enough permissions")
			return
		}

		var input store.ConstructionInput
		if err := readJSON(r, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}

		construction, err := s.store.UpdateConstruction(id, claims.CompanyID, includeAll, input)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, construction)

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

func (s *Server) handleObjectByID(w http.ResponseWriter, r *http.Request) {
	claims := mustClaims(r)
	id := strings.TrimPrefix(r.URL.Path, "/api/objects/")
	if id == "" {
		writeError(w, http.StatusNotFound, "object not found")
		return
	}

	includeAll := claims.Role == domain.RoleSuperAdmin
	switch r.Method {
	case http.MethodPut:
		if !claims.Role.CanEditEstimates() {
			writeError(w, http.StatusForbidden, "not enough permissions")
			return
		}

		var input store.ConstructionObjectInput
		if err := readJSON(r, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}

		object, err := s.store.UpdateObject(id, claims.CompanyID, includeAll, input)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, object)

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

func (s *Server) handleGSNSupplements(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	supplements, err := s.gsn.ListSupplements(r.Context())
	if err != nil {
		if errors.Is(err, gsn.ErrNotConfigured) {
			writeError(w, http.StatusServiceUnavailable, "GSN database is not configured; set APP_GSN_DATABASE_URL")
			return
		}
		slog.Error("gsn supplements query failed", "error", err)
		writeError(w, http.StatusBadGateway, "failed to read GSN supplements")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"supplements": supplements,
	})
}

func (s *Server) handleGSNHierarchy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	supplement := r.URL.Query().Get("supplement")
	if supplement == "" {
		writeError(w, http.StatusBadRequest, "supplement is required")
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

	nodes, err := s.gsn.ListChildren(r.Context(), supplement, r.URL.Query().Get("parent"), limit)
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

func (s *Server) handleGSNBaseInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	supplement := r.URL.Query().Get("supplement")
	if supplement == "" {
		writeError(w, http.StatusBadRequest, "supplement is required")
		return
	}

	info, err := s.gsn.BaseInfo(r.Context(), supplement)
	if err != nil {
		if errors.Is(err, gsn.ErrNotConfigured) {
			writeError(w, http.StatusServiceUnavailable, "GSN database is not configured; set APP_GSN_DATABASE_URL")
			return
		}
		slog.Error("gsn base info query failed", "error", err)
		writeError(w, http.StatusBadGateway, "failed to read GSN base info")
		return
	}

	writeJSON(w, http.StatusOK, info)
}

func (s *Server) serveAdmin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	http.ServeFile(w, r, filepath.Join(s.webDir, "admin.html"))
}

func (s *Server) withAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := mustClaims(r)
		if !claims.Role.CanManageUsers() {
			writeError(w, http.StatusForbidden, "not enough permissions")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) withAuthorized(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := mustClaims(r)
		user, ok := s.store.FindUserByID(claims.UserID)
		if !ok {
			writeError(w, http.StatusUnauthorized, "user not found")
			return
		}
		if !user.CanAccessApp() {
			writeError(w, http.StatusForbidden, "учётная запись не авторизована для работы в системе")
			return
		}
		next.ServeHTTP(w, r)
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
		writeError(w, http.StatusBadRequest, "некорректные или конфликтующие данные")
	default:
		slog.Error("store operation failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}
