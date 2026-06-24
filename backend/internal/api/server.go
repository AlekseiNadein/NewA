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
	"time"

	"nav-saas-mvp/backend/internal/auth"
	"nav-saas-mvp/backend/internal/domain"
	"nav-saas-mvp/backend/internal/gsn"
	"nav-saas-mvp/backend/internal/presence"
	"nav-saas-mvp/backend/internal/store"
)

type Server struct {
	store           *store.FileStore
	auth            *auth.Service
	gsn             *gsn.Service
	estimateLocks   *presence.EstimateLocks
	licenseSessions *presence.LicenseSessions
	webDir          string
	static          http.Handler
}

type contextKey string

const claimsKey contextKey = "claims"

func NewServer(store *store.FileStore, authService *auth.Service, gsnService *gsn.Service, webDir string) *Server {
	return &Server{
		store:           store,
		auth:            authService,
		gsn:             gsnService,
		estimateLocks:   presence.NewEstimateLocks(),
		licenseSessions: presence.NewLicenseSessions(),
		webDir:          webDir,
		static:          http.FileServer(http.Dir(webDir)),
	}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/login", s.handleLogin)
	mux.HandleFunc("/api/auth/logout", s.handleLogout)
	mux.HandleFunc("/api/auth/register", s.handleRegister)
	mux.Handle("/api/me", s.withAuth(http.HandlerFunc(s.handleMe)))
	mux.Handle("/api/companies", s.withAuth(http.HandlerFunc(s.handleCompanies)))
	mux.Handle("/api/users", s.withAuth(s.withAdmin(http.HandlerFunc(s.handleUsers))))
	mux.Handle("/api/users/", s.withAuth(s.withAdmin(http.HandlerFunc(s.handleUserByID))))
	mux.Handle("/api/admin/estimate-locks", s.withAuth(s.withAdmin(http.HandlerFunc(s.handleAdminEstimateLocks))))
	mux.Handle("/api/admin/estimate-locks/", s.withAuth(s.withAdmin(http.HandlerFunc(s.handleAdminEstimateLockByID))))
	mux.Handle("/api/admin/licenses", s.withAuth(s.withAdmin(http.HandlerFunc(s.handleAdminLicenses))))
	mux.Handle("/api/constructions", s.withAuth(s.withAuthorized(http.HandlerFunc(s.handleConstructions))))
	mux.Handle("/api/constructions/", s.withAuth(s.withAuthorized(http.HandlerFunc(s.handleConstructionByID))))
	mux.Handle("/api/objects", s.withAuth(s.withAuthorized(http.HandlerFunc(s.handleObjects))))
	mux.Handle("/api/objects/", s.withAuth(s.withAuthorized(http.HandlerFunc(s.handleObjectByID))))
	mux.Handle("/api/estimates", s.withAuth(s.withAuthorized(http.HandlerFunc(s.handleEstimates))))
	mux.Handle("/api/estimates/", s.withAuth(s.withAuthorized(http.HandlerFunc(s.handleEstimateByID))))
	mux.Handle("/api/license-sessions", s.withAuth(s.withAuthorized(http.HandlerFunc(s.handleLicenseSessions))))
	mux.Handle("/api/estimate-locks", s.withAuth(s.withAuthorized(http.HandlerFunc(s.handleEstimateLocks))))
	mux.Handle("/api/gsn/supplements", s.withAuth(s.withAuthorized(http.HandlerFunc(s.handleGSNSupplements))))
	mux.Handle("/api/gsn/base-info", s.withAuth(s.withAuthorized(http.HandlerFunc(s.handleGSNBaseInfo))))
	mux.Handle("/api/gsn/hierarchy", s.withAuth(s.withAuthorized(http.HandlerFunc(s.handleGSNHierarchy))))
	mux.Handle("/api/gsn/hierarchy-search", s.withAuth(s.withAuthorized(http.HandlerFunc(s.handleGSNHierarchySearch))))
	mux.Handle("/api/gsn/document", s.withAuth(s.withAuthorized(http.HandlerFunc(s.handleGSNDocument))))
	mux.Handle("/api/gsn/record", s.withAuth(s.withAuthorized(http.HandlerFunc(s.handleGSNRecord))))
	mux.Handle("/api/gsn/hierarchy-records", s.withAuth(s.withAuthorized(http.HandlerFunc(s.handleGSNHierarchyRecords))))
	mux.Handle("/api/gsn/regions", s.withAuth(s.withAuthorized(http.HandlerFunc(s.handleGSNRegions))))
	mux.Handle("/api/gsn/fgis-sets", s.withAuth(s.withAuthorized(http.HandlerFunc(s.handleGSNFGISSets))))
	mux.Handle("/api/gsn/fgis-rows", s.withAuth(s.withAuthorized(http.HandlerFunc(s.handleGSNFGISRows))))
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

	case http.MethodDelete:
		if err := s.store.DeleteUser(id, claims.CompanyID, includeAll); err != nil {
			writeStoreError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)

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

	if strings.HasSuffix(id, "/lock") {
		s.handleEstimateLock(w, r, strings.TrimSuffix(id, "/lock"))
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

func (s *Server) handleLicenseSessions(w http.ResponseWriter, r *http.Request) {
	claims := mustClaims(r)
	user, ok := s.store.FindUserByID(claims.UserID)
	if !ok {
		writeError(w, http.StatusUnauthorized, "user not found")
		return
	}

	switch r.Method {
	case http.MethodPut:
		var input struct {
			SubsectionIDs []string `json:"subsectionIds"`
		}
		if err := readJSON(r, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}

		validated := make([]string, 0, len(input.SubsectionIDs))
		for _, id := range input.SubsectionIDs {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			if !domain.ValidBaseSubsectionID(domain.BaseSubsectionID(id)) {
				writeError(w, http.StatusBadRequest, "unknown subsection id")
				return
			}
			validated = append(validated, id)
		}

		result := s.licenseSessions.Sync(
			claims.CompanyID,
			claims.UserID,
			user.Name,
			validated,
			func(subsectionID string) int {
				return s.store.LicenseAvailable(claims.CompanyID, subsectionID)
			},
			func(subsectionID string) string {
				return domain.BaseSubsectionName(domain.BaseSubsectionID(subsectionID))
			},
		)
		if !result.Granted {
			writeJSON(w, http.StatusForbidden, result)
			return
		}
		writeJSON(w, http.StatusOK, result)

	case http.MethodDelete:
		s.licenseSessions.ReleaseAll(claims.UserID)
		w.WriteHeader(http.StatusNoContent)

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleEstimateLocks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	claims := mustClaims(r)
	writeJSON(w, http.StatusOK, s.estimateLocks.ListByCompany(claims.CompanyID))
}

func (s *Server) handleEstimateLock(w http.ResponseWriter, r *http.Request, estimateID string) {
	claims := mustClaims(r)
	includeAll := claims.Role == domain.RoleSuperAdmin

	estimate, ok := s.findEstimate(estimateID, claims.CompanyID, includeAll)
	if !ok {
		writeError(w, http.StatusNotFound, "estimate not found")
		return
	}

	user, ok := s.store.FindUserByID(claims.UserID)
	if !ok {
		writeError(w, http.StatusUnauthorized, "user not found")
		return
	}

	switch r.Method {
	case http.MethodPut:
		if !claims.Role.CanEditEstimates() {
			writeError(w, http.StatusForbidden, "not enough permissions")
			return
		}

		lock, err := s.estimateLocks.Acquire(estimateID, claims.UserID, user.Name, estimate.CompanyID)
		if err != nil {
			var conflict presence.LockConflict
			if errors.As(err, &conflict) {
				writeJSON(w, http.StatusConflict, map[string]any{
					"error": "смета редактируется другим пользователем",
					"lock":  conflict.Lock,
				})
				return
			}
			writeError(w, http.StatusInternalServerError, "failed to acquire lock")
			return
		}
		writeJSON(w, http.StatusOK, lock)

	case http.MethodDelete:
		s.estimateLocks.Release(estimateID, claims.UserID)
		w.WriteHeader(http.StatusNoContent)

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) findEstimate(id, companyID string, includeAll bool) (domain.Estimate, bool) {
	for _, estimate := range s.store.ListEstimates(companyID, includeAll) {
		if estimate.ID == id {
			return estimate, true
		}
	}
	return domain.Estimate{}, false
}

type adminEstimateLockView struct {
	EstimateID    string    `json:"estimateId"`
	EstimateCode  string    `json:"estimateCode"`
	EstimateTitle string    `json:"estimateTitle"`
	UserID        string    `json:"userId"`
	UserName      string    `json:"userName"`
	CompanyID     string    `json:"companyId"`
	CompanyName   string    `json:"companyName"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

func (s *Server) handleAdminEstimateLocks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	claims := mustClaims(r)
	includeAll := claims.Role == domain.RoleSuperAdmin
	locks := s.estimateLocks.List(claims.CompanyID, includeAll)

	companyNames := make(map[string]string, len(s.store.ListCompanies()))
	for _, company := range s.store.ListCompanies() {
		companyNames[company.ID] = company.Name
	}

	estimates := s.store.ListEstimates(claims.CompanyID, includeAll)
	estimateByID := make(map[string]domain.Estimate, len(estimates))
	for _, estimate := range estimates {
		estimateByID[estimate.ID] = estimate
	}

	items := make([]adminEstimateLockView, 0, len(locks))
	for _, lock := range locks {
		item := adminEstimateLockView{
			EstimateID:  lock.EstimateID,
			UserID:      lock.UserID,
			UserName:    lock.UserName,
			CompanyID:   lock.CompanyID,
			CompanyName: companyNames[lock.CompanyID],
			UpdatedAt:   lock.UpdatedAt,
		}
		if estimate, ok := estimateByID[lock.EstimateID]; ok {
			item.EstimateCode = estimate.Code
			item.EstimateTitle = estimate.Title
		}
		items = append(items, item)
	}

	writeJSON(w, http.StatusOK, items)
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

		view, err := s.store.GetCompanyLicenses(companyID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
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

		view, err := s.store.UpdateCompanyLicenses(companyID, store.UpdateCompanyLicensesInput{
			Items: input.Items,
		})
		if err != nil {
			switch {
			case errors.Is(err, store.ErrNotFound):
				writeError(w, http.StatusNotFound, "company not found")
			case errors.Is(err, store.ErrConflict):
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

func (s *Server) handleAdminEstimateLockByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	claims := mustClaims(r)
	includeAll := claims.Role == domain.RoleSuperAdmin
	estimateID := strings.TrimPrefix(r.URL.Path, "/api/admin/estimate-locks/")
	if estimateID == "" {
		writeError(w, http.StatusNotFound, "estimate lock not found")
		return
	}

	var targetLock *presence.EstimateLock
	for _, lock := range s.estimateLocks.List(claims.CompanyID, includeAll) {
		if lock.EstimateID == estimateID {
			targetLock = &lock
			break
		}
	}
	if targetLock == nil {
		writeError(w, http.StatusNotFound, "estimate lock not found")
		return
	}

	if _, ok := s.estimateLocks.ForceRelease(estimateID); !ok {
		writeError(w, http.StatusNotFound, "estimate lock not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
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

func (s *Server) handleGSNHierarchySearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	supplement := strings.TrimSpace(r.URL.Query().Get("supplement"))
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if supplement == "" {
		writeError(w, http.StatusBadRequest, "supplement is required")
		return
	}
	if query == "" {
		writeError(w, http.StatusBadRequest, "q is required")
		return
	}

	limit := 50
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid limit")
			return
		}
		limit = parsed
	}

	matches, err := s.gsn.SearchHierarchy(r.Context(), supplement, query, limit)
	if err != nil {
		if errors.Is(err, gsn.ErrNotConfigured) {
			writeError(w, http.StatusServiceUnavailable, "GSN database is not configured; set APP_GSN_DATABASE_URL")
			return
		}
		slog.Error("gsn hierarchy search failed", "error", err)
		writeError(w, http.StatusBadGateway, "failed to search GSN hierarchy")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"matches": matches,
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

func (s *Server) handleGSNDocument(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	supplement := strings.TrimSpace(r.URL.Query().Get("supplement"))
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if supplement == "" || code == "" {
		writeError(w, http.StatusBadRequest, "supplement and code are required")
		return
	}

	document, err := s.gsn.GetHierarchyDocument(r.Context(), supplement, code)
	if err != nil {
		if errors.Is(err, gsn.ErrNotConfigured) {
			writeError(w, http.StatusServiceUnavailable, "GSN database is not configured; set APP_GSN_DATABASE_URL")
			return
		}
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "not imported") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		slog.Error("gsn document query failed", "error", err)
		writeError(w, http.StatusBadGateway, "failed to read GSN document")
		return
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "inline; filename=\""+document.FileName+"\"")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(document.Content); err != nil {
		slog.Error("gsn document write failed", "error", err)
	}
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

func (s *Server) handleGSNRecord(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if code == "" {
		writeError(w, http.StatusBadRequest, "code is required")
		return
	}
	fgisSetID := strings.TrimSpace(r.URL.Query().Get("fgisSet"))
	district := strings.TrimSpace(r.URL.Query().Get("district"))

	record, err := s.gsn.GetRecordDetail(r.Context(), code, fgisSetID, district)
	if err != nil {
		if errors.Is(err, gsn.ErrNotConfigured) {
			writeError(w, http.StatusServiceUnavailable, "GSN database is not configured; set APP_GSN_DATABASE_URL")
			return
		}
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, "record not found")
			return
		}
		slog.Error("gsn record query failed", "error", err, "code", code)
		writeError(w, http.StatusBadGateway, "failed to read GSN record")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"record": record,
	})
}

func (s *Server) handleGSNHierarchyRecords(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	supplement := strings.TrimSpace(r.URL.Query().Get("supplement"))
	hierarchy := strings.TrimSpace(r.URL.Query().Get("hierarchy"))
	if supplement == "" || hierarchy == "" {
		writeError(w, http.StatusBadRequest, "supplement and hierarchy are required")
		return
	}

	fgisSetID := strings.TrimSpace(r.URL.Query().Get("fgisSet"))
	district := strings.TrimSpace(r.URL.Query().Get("district"))

	records, err := s.gsn.ListHierarchyRecords(r.Context(), supplement, hierarchy, fgisSetID, district)
	if err != nil {
		if errors.Is(err, gsn.ErrNotConfigured) {
			writeError(w, http.StatusServiceUnavailable, "GSN database is not configured; set APP_GSN_DATABASE_URL")
			return
		}
		slog.Error("gsn hierarchy records query failed", "error", err, "supplement", supplement, "hierarchy", hierarchy)
		writeError(w, http.StatusBadGateway, "failed to read GSN hierarchy records")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"records": records,
	})
}

func (s *Server) handleGSNRegions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	regions, err := s.gsn.ListRegions(r.Context())
	if err != nil {
		if errors.Is(err, gsn.ErrNotConfigured) {
			writeError(w, http.StatusServiceUnavailable, "GSN database is not configured; set APP_GSN_DATABASE_URL")
			return
		}
		slog.Error("gsn regions query failed", "error", err)
		writeError(w, http.StatusBadGateway, "failed to read GSN regions")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"regions": regions,
	})
}

func (s *Server) handleGSNFGISSets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	sets, err := s.gsn.ListFGISSets(r.Context())
	if err != nil {
		if errors.Is(err, gsn.ErrNotConfigured) {
			writeError(w, http.StatusServiceUnavailable, "GSN database is not configured; set APP_GSN_DATABASE_URL")
			return
		}
		slog.Error("gsn fgis sets query failed", "error", err)
		writeError(w, http.StatusBadGateway, "failed to read FGIS sets")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"sets": sets,
	})
}

func (s *Server) handleGSNFGISRows(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	setID := strings.TrimSpace(r.URL.Query().Get("set"))
	if setID == "" {
		writeError(w, http.StatusBadRequest, "set query parameter is required")
		return
	}

	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}

	offset := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("offset")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			offset = parsed
		}
	}

	result, err := s.gsn.ListFGISSetRows(r.Context(), setID, r.URL.Query().Get("q"), limit, offset)
	if err != nil {
		if errors.Is(err, gsn.ErrNotConfigured) {
			writeError(w, http.StatusServiceUnavailable, "GSN database is not configured; set APP_GSN_DATABASE_URL")
			return
		}
		slog.Error("gsn fgis rows query failed", "error", err, "set", setID)
		writeError(w, http.StatusBadGateway, "failed to read FGIS rows")
		return
	}

	writeJSON(w, http.StatusOK, result)
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
