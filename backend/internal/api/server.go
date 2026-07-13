package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"nav-saas-mvp/backend/internal/auth"
	"nav-saas-mvp/backend/internal/authstore"
	"nav-saas-mvp/backend/internal/domain"
	"nav-saas-mvp/backend/internal/gsn"
	"nav-saas-mvp/backend/internal/observability"
	"nav-saas-mvp/backend/internal/presence"
	"nav-saas-mvp/backend/internal/queue"
	"nav-saas-mvp/backend/internal/store"
)

type Server struct {
	store           *store.FileStore
	authStore       *authstore.Store
	authReader      authstore.AppReader
	auth            *auth.Service
	gsn             *gsn.Service
	estimateLocks   *presence.EstimateLocks
	licenseSessions *presence.LicenseSessions
	webDir          string
	static          http.Handler
}

type contextKey string

const claimsKey contextKey = "claims"

func NewServer(store *store.FileStore, authStore *authstore.Store, authService *auth.Service, gsnService *gsn.Service, webDir string) *Server {
	return &Server{
		store:           store,
		authStore:       authStore,
		authReader:      authStore,
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
	mux.HandleFunc("/api/healthz", s.handleHealthz)
	mux.Handle("/api/admin/estimate-locks", s.withAuth(s.withAdmin(http.HandlerFunc(s.handleAdminEstimateLocks))))
	mux.Handle("/api/admin/estimate-locks/", s.withAuth(s.withAdmin(http.HandlerFunc(s.handleAdminEstimateLockByID))))
	mux.Handle("/api/admin/queue-stats", s.withAuth(s.withAdmin(http.HandlerFunc(s.handleAdminQueueStats))))
	mux.Handle("/api/admin/queue-purge", s.withAuth(s.withAdmin(http.HandlerFunc(s.handleAdminQueuePurge))))
	mux.Handle("/api/admin/monitoring", s.withAuth(s.withAdmin(http.HandlerFunc(s.handleAdminMonitoring))))
	mux.Handle("/api/constructions", s.withAuth(s.withAuthorized(http.HandlerFunc(s.handleConstructions))))
	mux.Handle("/api/constructions/", s.withAuth(s.withAuthorized(http.HandlerFunc(s.handleConstructionByID))))
	mux.Handle("/api/objects", s.withAuth(s.withAuthorized(http.HandlerFunc(s.handleObjects))))
	mux.Handle("/api/objects/", s.withAuth(s.withAuthorized(http.HandlerFunc(s.handleObjectByID))))
	mux.Handle("/api/estimates", s.withAuth(s.withAuthorized(http.HandlerFunc(s.handleEstimates))))
	mux.Handle("/api/estimates/", s.withAuth(s.withAuthorized(http.HandlerFunc(s.handleEstimateByID))))
	mux.Handle("/api/license-sessions", s.withAuth(s.withAuthorized(http.HandlerFunc(s.handleLicenseSessions))))
	mux.Handle("/api/estimate-locks", s.withAuth(s.withAuthorized(http.HandlerFunc(s.handleEstimateLocks))))
	mux.Handle("/api/settings", s.withAuth(s.withAuthorized(http.HandlerFunc(s.handleSettings))))
	mux.Handle("/api/user-positions", s.withAuth(s.withAuthorized(http.HandlerFunc(s.handleUserPositions))))
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

	return observability.WrapHTTP(s.withCommonHeaders(mux))
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, buildQueueHealth(r.Context(), s.store, s.gsn != nil && s.gsn.Configured()))
}

func (s *Server) handleAdminQueueStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, buildQueueHealth(r.Context(), s.store, s.gsn != nil && s.gsn.Configured()))
}

func (s *Server) handleAdminQueuePurge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	mode := s.store.QueueMode()
	if mode != "rabbit" && mode != "dual" {
		writeError(w, http.StatusBadRequest, "очистка очереди доступна только в режиме rabbit")
		return
	}

	var input struct {
		Target string `json:"target"`
	}
	if err := readJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	target := strings.ToLower(strings.TrimSpace(input.Target))
	if target == "" {
		target = "dlq"
	}

	var queueNames []string
	switch target {
	case "dlq":
		queueNames = []string{"estimate.calc.dlq"}
	case "main":
		queueNames = []string{"estimate.calc.main"}
	case "retry":
		queueNames = queue.CalcRetryQueueNames()
	case "all":
		queueNames = queue.CalcQueueNames()
	default:
		writeError(w, http.StatusBadRequest, "unknown purge target")
		return
	}

	rabbitURL := strings.TrimSpace(os.Getenv("APP_RABBITMQ_URL"))
	if rabbitURL == "" {
		writeError(w, http.StatusServiceUnavailable, "rabbitmq is not configured")
		return
	}

	purged, err := queue.PurgeQueues(rabbitURL, queueNames)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	total := 0
	for _, count := range purged {
		total += count
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"target": target,
		"purged": purged,
		"total":  total,
	})
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

	case http.MethodDelete:
		if !claims.Role.CanEditEstimates() {
			writeError(w, http.StatusForbidden, "not enough permissions")
			return
		}

		if err := s.store.DeleteConstruction(id, claims.CompanyID, includeAll); err != nil {
			writeStoreError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)

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

	case http.MethodDelete:
		if !claims.Role.CanEditEstimates() {
			writeError(w, http.StatusForbidden, "not enough permissions")
			return
		}

		if err := s.store.DeleteObject(id, claims.CompanyID, includeAll); err != nil {
			writeStoreError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleEstimates(w http.ResponseWriter, r *http.Request) {
	claims := mustClaims(r)
	includeAll := claims.Role == domain.RoleSuperAdmin

	switch r.Method {
	case http.MethodGet:
		summary := r.URL.Query().Get("summary") == "1" || strings.EqualFold(r.URL.Query().Get("summary"), "true")
		if summary {
			writeJSON(w, http.StatusOK, s.store.ListEstimatesSummary(claims.CompanyID, includeAll))
			return
		}
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
	if strings.HasSuffix(id, "/calc-batch") {
		s.handleEstimateCalcBatch(w, r, strings.TrimSuffix(id, "/calc-batch"))
		return
	}
	if strings.HasSuffix(id, "/calc/cancel") {
		s.handleEstimateCalcCancel(w, r, strings.TrimSuffix(id, "/calc/cancel"))
		return
	}
	if strings.HasSuffix(id, "/calc-status") {
		s.handleEstimateCalcStatus(w, r, strings.TrimSuffix(id, "/calc-status"))
		return
	}
	if strings.HasSuffix(id, "/calc") {
		s.handleEstimateCalcStart(w, r, strings.TrimSuffix(id, "/calc"))
		return
	}

	includeAll := claims.Role == domain.RoleSuperAdmin
	switch r.Method {
	case http.MethodGet:
		estimate, err := s.store.GetEstimate(id, claims.CompanyID, includeAll)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, estimate)

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

func (s *Server) handleEstimateCalcStatus(w http.ResponseWriter, r *http.Request, estimateID string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	claims := mustClaims(r)
	includeAll := claims.Role == domain.RoleSuperAdmin
	summary := r.URL.Query().Get("summary") == "1" || strings.EqualFold(r.URL.Query().Get("summary"), "true")
	if summary {
		calcSummary, err := s.store.SummarizeEstimateCalcStatus(r.Context(), claims.CompanyID, estimateID, includeAll)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, calcSummary)
		return
	}
	lite := r.URL.Query().Get("lite") == "1" || strings.EqualFold(r.URL.Query().Get("lite"), "true")
	statuses, err := s.store.ListEstimateCalcStatuses(r.Context(), claims.CompanyID, estimateID, includeAll, lite)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": statuses,
	})
}

func (s *Server) handleEstimateCalcStart(w http.ResponseWriter, r *http.Request, estimateID string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	claims := mustClaims(r)
	if !claims.Role.CanEditEstimates() {
		writeError(w, http.StatusForbidden, "not enough permissions")
		return
	}

	includeAll := claims.Role == domain.RoleSuperAdmin
	if err := s.store.StartEstimateCalc(r.Context(), estimateID, claims.CompanyID, includeAll); err != nil {
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) handleEstimateCalcBatch(w http.ResponseWriter, r *http.Request, estimateID string) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	claims := mustClaims(r)
	includeAll := claims.Role == domain.RoleSuperAdmin
	applied, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("applied")))
	generation, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("generation")), 10, 64)
	wait := r.URL.Query().Get("wait") == "1" || strings.EqualFold(r.URL.Query().Get("wait"), "true")

	batch, err := s.store.FetchEstimateCalcBatch(r.Context(), claims.CompanyID, estimateID, includeAll, applied, generation, wait, 25*time.Second)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, batch)
}

func (s *Server) handleEstimateCalcCancel(w http.ResponseWriter, r *http.Request, estimateID string) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	claims := mustClaims(r)
	if !claims.Role.CanEditEstimates() {
		writeError(w, http.StatusForbidden, "not enough permissions")
		return
	}

	includeAll := claims.Role == domain.RoleSuperAdmin
	generation, err := s.store.CancelEstimateCalc(r.Context(), claims.CompanyID, estimateID, includeAll)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"generation": generation,
	})
}

func (s *Server) handleLicenseSessions(w http.ResponseWriter, r *http.Request) {
	claims := mustClaims(r)
	userName := strings.TrimSpace(claims.Name)
	if userName == "" {
		userName = claims.Email
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
			userName,
			validated,
			func(subsectionID string) int {
				return s.authReader.LicenseAvailable(claims.CompanyID, subsectionID)
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

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	claims := mustClaims(r)

	switch r.Method {
	case http.MethodGet:
		settings, err := s.store.GetAppSettings()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read settings")
			return
		}
		settings.Editable = claims.Role.CanManageUsers()
		writeJSON(w, http.StatusOK, settings)
	case http.MethodPut:
		if !claims.Role.CanManageUsers() {
			writeError(w, http.StatusForbidden, "редактировать настройки может только администратор")
			return
		}
		var input store.UpdateAppSettingsInput
		if err := readJSON(r, &input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		settings, err := s.store.UpdateAppSettings(input)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update settings")
			return
		}
		settings.Editable = true
		writeJSON(w, http.StatusOK, settings)
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

	userName := strings.TrimSpace(claims.Name)
	if userName == "" {
		userName = claims.Email
	}

	switch r.Method {
	case http.MethodPut:
		if !claims.Role.CanEditEstimates() {
			writeError(w, http.StatusForbidden, "not enough permissions")
			return
		}

		lock, err := s.estimateLocks.Acquire(estimateID, claims.UserID, userName, estimate.CompanyID)
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
		released := s.estimateLocks.Release(estimateID, claims.UserID)
		w.WriteHeader(http.StatusNoContent)
		if released {
			s.scheduleClearEstimateCalcOnClose(estimateID, estimate.CompanyID, includeAll)
		}

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) scheduleClearEstimateCalcOnClose(estimateID, companyID string, includeAll bool) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		if err := s.store.ClearEstimateCalcResultsOnClose(ctx, companyID, estimateID, includeAll); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return
			}
			slog.Warn("clear estimate calc on close failed", "estimateId", estimateID, "error", err)
		}
	}()
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

	companyNames := make(map[string]string, len(s.authReader.ListCompanies()))
	for _, company := range s.authReader.ListCompanies() {
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

	if lock, ok := s.estimateLocks.ForceRelease(estimateID); ok {
		w.WriteHeader(http.StatusNoContent)
		s.scheduleClearEstimateCalcOnClose(estimateID, lock.CompanyID, includeAll)
		return
	}

	writeError(w, http.StatusNotFound, "estimate lock not found")
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
		if !claims.CanAccessApp() {
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
	case errors.Is(err, store.ErrNotFound), errors.Is(err, authstore.ErrNotFound):
		writeError(w, http.StatusNotFound, "not found")
	case errors.Is(err, store.ErrForbidden), errors.Is(err, authstore.ErrForbidden):
		writeError(w, http.StatusForbidden, "not enough permissions")
	case errors.Is(err, store.ErrConflict), errors.Is(err, authstore.ErrConflict):
		writeError(w, http.StatusBadRequest, "некорректные или конфликтующие данные")
	default:
		slog.Error("store operation failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}
