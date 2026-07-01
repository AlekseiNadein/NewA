package authapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"nav-saas-mvp/backend/internal/auth"
	"nav-saas-mvp/backend/internal/authstore"
	"nav-saas-mvp/backend/internal/domain"
	"nav-saas-mvp/backend/internal/observability"
)

type contextKey string

const claimsKey contextKey = "claims"

type Server struct {
	accounts authstore.AccountStore
	auth     *auth.Service
}

func New(accounts authstore.AccountStore, authService *auth.Service) *Server {
	return &Server{
		accounts: accounts,
		auth:     authService,
	}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	s.RegisterRoutes(mux)
	return observability.WrapHTTP(withCommonHeaders(mux))
}

func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/auth/login", s.handleLogin)
	mux.HandleFunc("/api/auth/logout", s.handleLogout)
	mux.HandleFunc("/api/auth/register", s.handleRegister)
	mux.Handle("/api/me", s.withAuth(http.HandlerFunc(s.handleMe)))
	mux.Handle("/api/companies", s.withAuth(http.HandlerFunc(s.handleCompanies)))
	mux.Handle("/api/users", s.withAuth(s.withAdmin(http.HandlerFunc(s.handleUsers))))
	mux.Handle("/api/users/", s.withAuth(s.withAdmin(http.HandlerFunc(s.handleUserByID))))
	mux.Handle("/api/admin/licenses", s.withAuth(s.withAdmin(http.HandlerFunc(s.handleAdminLicenses))))
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

	auth.SetSessionCookie(w, r, token)

	writeJSON(w, http.StatusOK, map[string]any{
		"user": user,
		"access": map[string]bool{
			"app":   user.CanAccessApp(),
			"admin": user.CanAccessAdmin(),
		},
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	auth.ClearSessionCookie(w, r)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
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

	var input authstore.RegisterUser
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	if input.Password != input.PasswordConfirm {
		writeError(w, http.StatusBadRequest, "пароли не совпадают")
		return
	}

	user, err := s.accounts.RegisterUser(input)
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
	user, ok := s.accounts.FindUserByID(claims.UserID)
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
		companies := s.accounts.ListCompanies()
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

		company, err := s.accounts.CreateCompany(input.Name)
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
		writeJSON(w, http.StatusOK, s.accounts.ListUsers(claims.CompanyID, includeAll))

	case http.MethodPost:
		var input authstore.NewUser
		if err := readJSON(r, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}

		if !includeAll {
			input.CompanyID = claims.CompanyID
			input.CompanyName = ""
			input.IsSuperAdministrator = false
		}

		user, err := s.accounts.CreateUser(input)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, s.accounts.UserView(user))

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
		var input authstore.UpdateUser
		if err := readJSON(r, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}

		user, err := s.accounts.UpdateUser(id, claims.CompanyID, includeAll, input)
		if err != nil {
			writeStoreError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, s.accounts.UserView(user))

	case http.MethodDelete:
		if err := s.accounts.DeleteUser(id, claims.CompanyID, includeAll); err != nil {
			writeStoreError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleAdminLicenses(w http.ResponseWriter, r *http.Request) {
	claims := mustClaims(r)

	switch r.Method {
	case http.MethodGet:
		companyID := strings.TrimSpace(r.URL.Query().Get("companyId"))
		if claims.Role != domain.RoleSuperAdmin {
			companyID = claims.CompanyID
		} else if companyID == "" {
			companyID = claims.CompanyID
		}

		view, err := s.accounts.GetCompanyLicenses(companyID)
		if err != nil {
			if errors.Is(err, authstore.ErrNotFound) {
				writeError(w, http.StatusNotFound, "company not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "failed to read licenses")
			return
		}
		view.Editable = claims.Role.CanManageLicenses()
		writeJSON(w, http.StatusOK, view)

	case http.MethodPut:
		if !claims.Role.CanManageLicenses() {
			writeError(w, http.StatusForbidden, "редактировать лицензии может только суперадминистратор")
			return
		}

		var input struct {
			CompanyID string         `json:"companyId"`
			Items     map[string]int `json:"items"`
		}
		if err := readJSON(r, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}

		companyID := strings.TrimSpace(input.CompanyID)
		if companyID == "" {
			writeError(w, http.StatusBadRequest, "companyId is required")
			return
		}

		view, err := s.accounts.UpdateCompanyLicenses(companyID, authstore.UpdateCompanyLicensesInput{
			Items: input.Items,
		})
		if err != nil {
			switch {
			case errors.Is(err, authstore.ErrNotFound):
				writeError(w, http.StatusNotFound, "company not found")
			case errors.Is(err, authstore.ErrConflict):
				writeError(w, http.StatusBadRequest, "invalid license data")
			default:
				writeError(w, http.StatusInternalServerError, "failed to update licenses")
			}
			return
		}
		view.Editable = true
		writeJSON(w, http.StatusOK, view)

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
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

func (s *Server) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		token, err := auth.TokenFromRequest(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "missing session")
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

func withCommonHeaders(next http.Handler) http.Handler {
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
		return domain.Claims{}
	}
	return claims
}
