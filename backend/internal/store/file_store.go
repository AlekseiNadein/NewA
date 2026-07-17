package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"nav-saas-mvp/backend/internal/domain"
	"nav-saas-mvp/backend/internal/estimatecalc"
	"nav-saas-mvp/backend/internal/observability"
	"nav-saas-mvp/backend/internal/requestctx"
)

var (
	ErrNotFound  = errors.New("not found")
	ErrConflict  = errors.New("conflict")
	ErrForbidden = errors.New("forbidden")
)

type FileStore struct {
	mu                 sync.RWMutex
	path               string
	treeDB             *pgxpool.Pool
	queueMode          string
	disableCalcEnqueue bool
	calcStartMu        sync.Mutex
	calcStartJobs      map[string]struct{}
	constructions      map[string]domain.Construction
	objects            map[string]domain.ConstructionObject
	estimates          map[string]domain.Estimate
	settings           domain.AppSettings
	// legacyAuth* preserves companies/users/licenses in app.json for auth ImportIfEmpty.
	legacyCompanies json.RawMessage
	legacyUsers     json.RawMessage
	legacyLicenses  json.RawMessage
}

type snapshot struct {
	Companies     json.RawMessage             `json:"companies,omitempty"`
	Users         json.RawMessage             `json:"users,omitempty"`
	Licenses      json.RawMessage             `json:"licenses,omitempty"`
	Constructions []domain.Construction       `json:"constructions"`
	Objects       []domain.ConstructionObject `json:"objects"`
	Estimates     []domain.Estimate           `json:"estimates"`
	Settings      domain.AppSettings          `json:"settings,omitempty"`
}

type EstimateInput struct {
	ObjectID    string                `json:"objectId"`
	Code        string                `json:"code"`
	Title       string                `json:"title"`
	Description string                `json:"description"`
	District    string                `json:"district"`
	FgisSetID   string                `json:"fgisSetId"`
	Status      domain.EstimateStatus `json:"status"`
	Total       *float64              `json:"total,omitempty"`
	Items       []domain.EstimateItem `json:"items"`
}

type ConstructionInput struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

type ConstructionObjectInput struct {
	ConstructionID string `json:"constructionId"`
	Code           string `json:"code"`
	Name           string `json:"name"`
}

const (
	defaultConstructionCode = "00"
	defaultConstructionName = "Новая стройка"
)

func NewFileStore(path string, treeDatabaseURL string) (*FileStore, error) {
	queueMode := normalizeQueueMode(os.Getenv("APP_QUEUE_MODE"))
	disableCalcEnqueue := envBool("APP_DISABLE_ESTIMATE_CALC_ENQUEUE")
	store := &FileStore{
		path:          path,
		queueMode:     queueMode,
		disableCalcEnqueue: disableCalcEnqueue,
		calcStartJobs: map[string]struct{}{},
		constructions: map[string]domain.Construction{},
		objects:       map[string]domain.ConstructionObject{},
		estimates:     map[string]domain.Estimate{},
		settings:      defaultAppSettings(),
	}

	if err := store.load(); err != nil {
		return nil, err
	}

	if strings.TrimSpace(treeDatabaseURL) != "" {
		pool, err := pgxpool.New(context.Background(), treeDatabaseURL)
		if err != nil {
			return nil, err
		}
		store.treeDB = pool
		if err := store.ensureTreeSchema(context.Background()); err != nil {
			return nil, err
		}
	}

	changed, err := store.ensureConstructionStructure()
	if err != nil {
		return nil, err
	}
	if changed {
		if err := store.save(); err != nil {
			return nil, err
		}
	}

	if store.treeDB != nil {
		if err := store.syncTreeToDB(context.Background()); err != nil {
			return nil, err
		}
	}

	return store, nil
}

func normalizeQueueMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "rabbit":
		return "rabbit"
	case "dual":
		return "dual"
	default:
		return "db"
	}
}

func (s *FileStore) QueueMode() string {
	return s.queueMode
}

func (s *FileStore) ListConstructions(companyID string, includeAll bool) []domain.Construction {
	if s.treeDB != nil {
		items, err := s.listConstructionsDB(context.Background(), companyID, includeAll)
		if err == nil {
			return items
		}
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	constructions := make([]domain.Construction, 0, len(s.constructions))
	for _, construction := range s.constructions {
		if includeAll || construction.CompanyID == companyID {
			constructions = append(constructions, construction)
		}
	}

	sort.Slice(constructions, func(i, j int) bool {
		return compareCodes(constructions[i].Code, constructions[j].Code)
	})

	return constructions
}

func (s *FileStore) CreateConstruction(companyID string, input ConstructionInput) (domain.Construction, error) {
	if s.treeDB != nil {
		return s.createConstructionDB(context.Background(), companyID, input)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	name := strings.TrimSpace(input.Name)
	code := strings.TrimSpace(input.Code)
	if name == "" || code == "" {
		return domain.Construction{}, ErrConflict
	}

	now := time.Now().UTC()
	construction := domain.Construction{
		ID:        newID("con"),
		CompanyID: companyID,
		Code:      code,
		Name:      name,
		CreatedAt: now,
		UpdatedAt: now,
	}

	s.constructions[construction.ID] = construction
	return construction, s.saveLocked()
}

func (s *FileStore) UpdateConstruction(id string, companyID string, includeAll bool, input ConstructionInput) (domain.Construction, error) {
	if s.treeDB != nil {
		return s.updateConstructionDB(context.Background(), id, companyID, includeAll, input)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	current, ok := s.constructions[id]
	if !ok {
		return domain.Construction{}, ErrNotFound
	}
	if !includeAll && current.CompanyID != companyID {
		return domain.Construction{}, ErrForbidden
	}

	name := strings.TrimSpace(input.Name)
	code := strings.TrimSpace(input.Code)
	if name == "" || code == "" {
		return domain.Construction{}, ErrConflict
	}

	current.Code = code
	current.Name = name
	current.UpdatedAt = time.Now().UTC()
	s.constructions[id] = current
	return current, s.saveLocked()
}

func (s *FileStore) DeleteConstruction(id string, companyID string, includeAll bool) error {
	if s.treeDB != nil {
		return s.deleteConstructionDB(context.Background(), id, companyID, includeAll)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	current, ok := s.constructions[id]
	if !ok {
		return ErrNotFound
	}
	if !includeAll && current.CompanyID != companyID {
		return ErrForbidden
	}

	for objectID, object := range s.objects {
		if object.ConstructionID != id {
			continue
		}
		for estimateID, estimate := range s.estimates {
			if estimate.ObjectID == objectID {
				delete(s.estimates, estimateID)
			}
		}
		delete(s.objects, objectID)
	}
	delete(s.constructions, id)
	return s.saveLocked()
}

func (s *FileStore) ListObjects(companyID string, includeAll bool) []domain.ConstructionObject {
	if s.treeDB != nil {
		items, err := s.listObjectsDB(context.Background(), companyID, includeAll)
		if err == nil {
			return items
		}
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	objects := make([]domain.ConstructionObject, 0, len(s.objects))
	for _, object := range s.objects {
		if includeAll || object.CompanyID == companyID {
			objects = append(objects, object)
		}
	}

	sort.Slice(objects, func(i, j int) bool {
		return compareCodes(objects[i].Code, objects[j].Code)
	})

	return objects
}

func (s *FileStore) CreateObject(companyID string, includeAll bool, input ConstructionObjectInput) (domain.ConstructionObject, error) {
	if s.treeDB != nil {
		return s.createObjectDB(context.Background(), companyID, includeAll, input)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	construction, ok := s.constructions[input.ConstructionID]
	if !ok {
		return domain.ConstructionObject{}, ErrNotFound
	}
	if !includeAll && construction.CompanyID != companyID {
		return domain.ConstructionObject{}, ErrForbidden
	}

	name := strings.TrimSpace(input.Name)
	code := strings.TrimSpace(input.Code)
	if name == "" || code == "" {
		return domain.ConstructionObject{}, ErrConflict
	}

	now := time.Now().UTC()
	object := domain.ConstructionObject{
		ID:             newID("obj"),
		CompanyID:      construction.CompanyID,
		ConstructionID: construction.ID,
		Code:           code,
		Name:           name,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	s.objects[object.ID] = object
	return object, s.saveLocked()
}

func (s *FileStore) UpdateObject(id string, companyID string, includeAll bool, input ConstructionObjectInput) (domain.ConstructionObject, error) {
	if s.treeDB != nil {
		return s.updateObjectDB(context.Background(), id, companyID, includeAll, input)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	current, ok := s.objects[id]
	if !ok {
		return domain.ConstructionObject{}, ErrNotFound
	}
	if !includeAll && current.CompanyID != companyID {
		return domain.ConstructionObject{}, ErrForbidden
	}

	name := strings.TrimSpace(input.Name)
	code := strings.TrimSpace(input.Code)
	if name == "" || code == "" {
		return domain.ConstructionObject{}, ErrConflict
	}

	current.Code = code
	current.Name = name
	current.UpdatedAt = time.Now().UTC()
	s.objects[id] = current
	return current, s.saveLocked()
}

func (s *FileStore) DeleteObject(id string, companyID string, includeAll bool) error {
	if s.treeDB != nil {
		return s.deleteObjectDB(context.Background(), id, companyID, includeAll)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	current, ok := s.objects[id]
	if !ok {
		return ErrNotFound
	}
	if !includeAll && current.CompanyID != companyID {
		return ErrForbidden
	}

	for estimateID, estimate := range s.estimates {
		if estimate.ObjectID == id {
			delete(s.estimates, estimateID)
		}
	}
	delete(s.objects, id)
	return s.saveLocked()
}

func (s *FileStore) ListEstimates(companyID string, includeAll bool) []domain.Estimate {
	if s.treeDB != nil {
		items, err := s.listEstimatesDB(context.Background(), companyID, includeAll)
		if err == nil {
			return items
		}
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.listEstimatesLocked(companyID, includeAll)
}

func (s *FileStore) ListEstimatesSummary(companyID string, includeAll bool) []domain.Estimate {
	if s.treeDB != nil {
		items, err := s.listEstimatesSummaryDB(context.Background(), companyID, includeAll)
		if err == nil {
			return items
		}
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	estimates := s.listEstimatesLocked(companyID, includeAll)
	for i := range estimates {
		estimates[i].Items = nil
	}
	return estimates
}

func (s *FileStore) GetEstimate(id, companyID string, includeAll bool) (domain.Estimate, error) {
	if s.treeDB != nil {
		return s.getEstimateDB(context.Background(), id, companyID, includeAll)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	estimate, ok := s.estimates[id]
	if !ok {
		return domain.Estimate{}, ErrNotFound
	}
	if !includeAll && estimate.CompanyID != companyID {
		return domain.Estimate{}, ErrForbidden
	}
	return estimate, nil
}

func (s *FileStore) listEstimatesLocked(companyID string, includeAll bool) []domain.Estimate {
	estimates := make([]domain.Estimate, 0, len(s.estimates))
	for _, estimate := range s.estimates {
		if includeAll || estimate.CompanyID == companyID {
			estimates = append(estimates, estimate)
		}
	}

	sort.Slice(estimates, func(i, j int) bool {
		return compareCodes(estimates[i].Code, estimates[j].Code)
	})

	return estimates
}

func (s *FileStore) CreateEstimate(companyID string, input EstimateInput) (domain.Estimate, error) {
	if s.treeDB != nil {
		return s.createEstimateDB(context.Background(), companyID, input)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	input.ObjectID = strings.TrimSpace(input.ObjectID)
	if input.ObjectID == "" {
		return domain.Estimate{}, ErrConflict
	}

	object, ok := s.objects[input.ObjectID]
	if !ok {
		return domain.Estimate{}, ErrNotFound
	}
	if object.CompanyID != companyID {
		return domain.Estimate{}, ErrForbidden
	}

	estimate, err := buildEstimate(newID("est"), companyID, input, time.Now().UTC(), time.Now().UTC())
	if err != nil {
		return domain.Estimate{}, err
	}

	s.estimates[estimate.ID] = estimate
	return estimate, s.saveLocked()
}

func (s *FileStore) UpdateEstimate(id string, companyID string, includeAll bool, input EstimateInput) (domain.Estimate, error) {
	if s.treeDB != nil {
		return s.updateEstimateDB(context.Background(), id, companyID, includeAll, input)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	current, ok := s.estimates[id]
	if !ok {
		return domain.Estimate{}, ErrNotFound
	}
	if !includeAll && current.CompanyID != companyID {
		return domain.Estimate{}, ErrForbidden
	}

	if strings.TrimSpace(input.ObjectID) == "" {
		input.ObjectID = current.ObjectID
	}
	if input.Description == "" {
		input.Description = current.Description
	}
	if input.District == "" {
		input.District = current.District
	}
	if input.Status == "" {
		input.Status = current.Status
	}
	if len(input.Items) == 0 {
		input.Items = current.Items
	}

	object, ok := s.objects[input.ObjectID]
	if !ok {
		return domain.Estimate{}, ErrNotFound
	}
	if object.CompanyID != current.CompanyID || (!includeAll && object.CompanyID != companyID) {
		return domain.Estimate{}, ErrForbidden
	}

	updated, err := buildEstimate(id, current.CompanyID, input, current.CreatedAt, time.Now().UTC())
	if err != nil {
		return domain.Estimate{}, err
	}

	s.estimates[id] = updated
	return updated, s.saveLocked()
}

func (s *FileStore) DeleteEstimate(id string, companyID string, includeAll bool) error {
	if s.treeDB != nil {
		return s.deleteEstimateDB(context.Background(), id, companyID, includeAll)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	current, ok := s.estimates[id]
	if !ok {
		return ErrNotFound
	}
	if !includeAll && current.CompanyID != companyID {
		return ErrForbidden
	}

	delete(s.estimates, id)
	return s.saveLocked()
}

func (s *FileStore) save() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.saveLocked()
}

func (s *FileStore) load() error {
	if _, err := os.Stat(s.path); errors.Is(err, os.ErrNotExist) {
		return nil
	}

	content, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}

	var snap snapshot
	if err := json.Unmarshal(content, &snap); err != nil {
		return err
	}

	for _, construction := range snap.Constructions {
		s.constructions[construction.ID] = construction
	}
	for _, object := range snap.Objects {
		s.objects[object.ID] = object
	}
	for _, estimate := range snap.Estimates {
		s.estimates[estimate.ID] = estimate
	}
	s.settings = normalizeAppSettings(snap.Settings)
	s.legacyCompanies = cloneRawJSON(snap.Companies)
	s.legacyUsers = cloneRawJSON(snap.Users)
	s.legacyLicenses = cloneRawJSON(snap.Licenses)

	return nil
}

func cloneRawJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	out := make(json.RawMessage, len(raw))
	copy(out, raw)
	return out
}

func (s *FileStore) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}

	snap := snapshot{
		Companies:     cloneRawJSON(s.legacyCompanies),
		Users:         cloneRawJSON(s.legacyUsers),
		Licenses:      cloneRawJSON(s.legacyLicenses),
		Constructions: make([]domain.Construction, 0, len(s.constructions)),
		Objects:       make([]domain.ConstructionObject, 0, len(s.objects)),
		Estimates:     make([]domain.Estimate, 0, len(s.estimates)),
		Settings:      normalizeAppSettings(s.settings),
	}

	for _, construction := range s.constructions {
		snap.Constructions = append(snap.Constructions, construction)
	}
	for _, object := range s.objects {
		snap.Objects = append(snap.Objects, object)
	}
	for _, estimate := range s.estimates {
		snap.Estimates = append(snap.Estimates, estimate)
	}

	sort.Slice(snap.Constructions, func(i, j int) bool {
		return compareCodes(snap.Constructions[i].Code, snap.Constructions[j].Code)
	})
	sort.Slice(snap.Objects, func(i, j int) bool {
		return compareCodes(snap.Objects[i].Code, snap.Objects[j].Code)
	})
	sort.Slice(snap.Estimates, func(i, j int) bool {
		return compareCodes(snap.Estimates[i].Code, snap.Estimates[j].Code)
	})

	content, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.path, content, 0o600)
}

func buildEstimate(id string, companyID string, input EstimateInput, createdAt time.Time, updatedAt time.Time) (domain.Estimate, error) {
	input.ObjectID = strings.TrimSpace(input.ObjectID)
	input.Code = strings.TrimSpace(input.Code)
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)
	input.District = strings.TrimSpace(input.District)
	input.FgisSetID = strings.TrimSpace(input.FgisSetID)
	if input.ObjectID == "" || input.Code == "" || input.Title == "" {
		return domain.Estimate{}, ErrConflict
	}
	if input.Status == "" {
		input.Status = domain.EstimateDraft
	}
	if !validEstimateStatus(input.Status) {
		return domain.Estimate{}, ErrConflict
	}

	items := make([]domain.EstimateItem, 0, len(input.Items))
	for _, item := range input.Items {
		normalized, _, err := normalizeEstimateItem(item)
		if err != nil {
			return domain.Estimate{}, err
		}
		items = append(items, normalized)
	}

	total := 0.0

	return domain.Estimate{
		ID:          id,
		CompanyID:   companyID,
		ObjectID:    input.ObjectID,
		Code:        input.Code,
		Title:       input.Title,
		Description: input.Description,
		District:    input.District,
		FgisSetID:   input.FgisSetID,
		Status:      input.Status,
		Items:       items,
		Total:       total,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
	}, nil
}

func compareCodes(left, right string) bool {
	return left < right
}

func (s *FileStore) ensureConstructionStructure() (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	changed := false
	companyIDs := make(map[string]struct{})
	for _, construction := range s.constructions {
		if construction.CompanyID != "" {
			companyIDs[construction.CompanyID] = struct{}{}
		}
	}
	for _, estimate := range s.estimates {
		if estimate.CompanyID != "" {
			companyIDs[estimate.CompanyID] = struct{}{}
		}
	}
	for companyID := range companyIDs {
		beforeConstructions := len(s.constructions)
		s.ensureDefaultConstructionLocked(companyID)
		if len(s.constructions) > beforeConstructions {
			changed = true
		}
		if s.migrateLegacyCodesLocked(companyID) {
			changed = true
		}
	}

	if !changed {
		return false, nil
	}
	return true, s.saveLocked()
}

func (s *FileStore) ensureDefaultConstructionLocked(companyID string) {
	if s.findConstructionByCodeLocked(companyID, defaultConstructionCode).ID != "" {
		return
	}

	now := time.Now().UTC()
	construction := domain.Construction{
		ID:        newID("con"),
		CompanyID: companyID,
		Code:      defaultConstructionCode,
		Name:      defaultConstructionName,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.constructions[construction.ID] = construction
}

func (s *FileStore) findConstructionByCodeLocked(companyID, code string) domain.Construction {
	for _, construction := range s.constructions {
		if construction.CompanyID == companyID && construction.Code == code {
			return construction
		}
	}
	return domain.Construction{}
}

func (s *FileStore) migrateLegacyCodesLocked(companyID string) bool {
	changed := false

	for id, construction := range s.constructions {
		if construction.CompanyID != companyID || construction.Code != "" {
			continue
		}
		construction.Code = legacyCode(id, "con_")
		construction.UpdatedAt = time.Now().UTC()
		s.constructions[id] = construction
		changed = true
	}

	for id, object := range s.objects {
		if object.CompanyID != companyID || object.Code != "" {
			continue
		}
		object.Code = legacyCode(id, "obj_")
		object.UpdatedAt = time.Now().UTC()
		s.objects[id] = object
		changed = true
	}

	for id, estimate := range s.estimates {
		if estimate.CompanyID != companyID || estimate.Code != "" {
			continue
		}
		estimate.Code = legacyCode(id, "est_")
		estimate.UpdatedAt = time.Now().UTC()
		s.estimates[id] = estimate
		changed = true
	}

	return changed
}

func legacyCode(id, prefix string) string {
	return strings.TrimPrefix(id, prefix)
}

func (s *FileStore) ensureTreeSchema(ctx context.Context) error {
	if s.treeDB == nil {
		return nil
	}

	sql := `
CREATE TABLE IF NOT EXISTS app_constructions (
    id TEXT PRIMARY KEY,
    company_id TEXT NOT NULL,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_app_constructions_company_id ON app_constructions(company_id);

CREATE TABLE IF NOT EXISTS app_construction_objects (
    id TEXT PRIMARY KEY,
    company_id TEXT NOT NULL,
    construction_id TEXT NOT NULL REFERENCES app_constructions(id) ON DELETE CASCADE,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_app_construction_objects_company_id ON app_construction_objects(company_id);
CREATE INDEX IF NOT EXISTS idx_app_construction_objects_construction_id ON app_construction_objects(construction_id);

CREATE TABLE IF NOT EXISTS app_estimates (
    id TEXT PRIMARY KEY,
    company_id TEXT NOT NULL,
    object_id TEXT NOT NULL REFERENCES app_construction_objects(id) ON DELETE CASCADE,
    code TEXT NOT NULL,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    district TEXT NOT NULL DEFAULT '',
    fgis_set_id TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL CHECK (status IN ('draft', 'approved', 'archived')),
    total NUMERIC(14, 2) NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_app_estimates_company_id ON app_estimates(company_id);
CREATE INDEX IF NOT EXISTS idx_app_estimates_object_id ON app_estimates(object_id);

ALTER TABLE app_estimates ADD COLUMN IF NOT EXISTS district TEXT NOT NULL DEFAULT '';
ALTER TABLE app_estimates ADD COLUMN IF NOT EXISTS fgis_set_id TEXT NOT NULL DEFAULT '';
ALTER TABLE app_estimates ADD COLUMN IF NOT EXISTS calc_generation BIGINT NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS app_estimate_lines (
    id TEXT NOT NULL,
    estimate_id TEXT NOT NULL REFERENCES app_estimates(id) ON DELETE CASCADE,
    line_type TEXT NOT NULL CHECK (line_type IN ('section', 'subsection', 'position')),
    source TEXT NOT NULL DEFAULT '',
    code TEXT NOT NULL DEFAULT '',
    original_code TEXT NOT NULL DEFAULT '',
    name TEXT NOT NULL,
    quantity NUMERIC(14, 3) NOT NULL DEFAULT 0,
    unit TEXT NOT NULL DEFAULT '',
    unit_price NUMERIC(14, 2) NOT NULL DEFAULT 0,
    total NUMERIC(14, 2) NOT NULL DEFAULT 0,
    sort_order INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (estimate_id, id)
);

ALTER TABLE app_estimate_lines ADD COLUMN IF NOT EXISTS code TEXT NOT NULL DEFAULT '';
ALTER TABLE app_estimate_lines ADD COLUMN IF NOT EXISTS original_code TEXT NOT NULL DEFAULT '';
ALTER TABLE app_estimate_lines ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT '';
ALTER TABLE app_estimate_lines ADD COLUMN IF NOT EXISTS raw_text TEXT NOT NULL DEFAULT '';
ALTER TABLE app_estimate_lines ADD COLUMN IF NOT EXISTS parsed_json JSONB;
ALTER TABLE app_estimate_lines ADD COLUMN IF NOT EXISTS calc_json JSONB;
ALTER TABLE app_estimate_lines ADD COLUMN IF NOT EXISTS calc_status TEXT NOT NULL DEFAULT '';
ALTER TABLE app_estimate_lines ADD COLUMN IF NOT EXISTS calc_error TEXT NOT NULL DEFAULT '';
ALTER TABLE app_estimate_lines ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 0;
ALTER TABLE app_estimate_lines ADD COLUMN IF NOT EXISTS calculated_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_app_estimate_lines_estimate_id ON app_estimate_lines(estimate_id, sort_order, id);

CREATE TABLE IF NOT EXISTS app_settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS app_user_positions (
    id TEXT PRIMARY KEY,
    company_id TEXT NOT NULL,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    unit TEXT NOT NULL DEFAULT '',
    cost NUMERIC(14, 2) NOT NULL DEFAULT 0,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_app_user_positions_company_code
    ON app_user_positions(company_id, code);

CREATE TABLE IF NOT EXISTS estimate_calc_jobs (
    id TEXT PRIMARY KEY,
    company_id TEXT NOT NULL,
    estimate_id TEXT NOT NULL REFERENCES app_estimates(id) ON DELETE CASCADE,
    line_id TEXT NOT NULL,
    revision BIGINT NOT NULL,
    job_type TEXT NOT NULL DEFAULT 'estimate_line',
    status TEXT NOT NULL CHECK (status IN ('queued', 'leased', 'done', 'failed', 'dead')),
    priority INTEGER NOT NULL DEFAULT 0,
    attempts INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 5,
    run_after TIMESTAMPTZ NOT NULL DEFAULT now(),
    leased_until TIMESTAMPTZ,
    locked_by TEXT NOT NULL DEFAULT '',
    last_error TEXT NOT NULL DEFAULT '',
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (estimate_id, line_id, revision)
);
CREATE INDEX IF NOT EXISTS idx_estimate_calc_jobs_ready
    ON estimate_calc_jobs(priority DESC, run_after, created_at)
    WHERE status = 'queued';
CREATE INDEX IF NOT EXISTS idx_estimate_calc_jobs_estimate
    ON estimate_calc_jobs(estimate_id, line_id, revision);

CREATE TABLE IF NOT EXISTS outbox_events (
    id TEXT PRIMARY KEY,
    topic TEXT NOT NULL,
    routing_key TEXT NOT NULL,
    payload JSONB NOT NULL,
    headers JSONB NOT NULL DEFAULT '{}'::jsonb,
    published_at TIMESTAMPTZ,
    attempts INTEGER NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_outbox_events_unpublished
    ON outbox_events(created_at)
    WHERE published_at IS NULL;

CREATE TABLE IF NOT EXISTS calc_message_receipts (
    id TEXT PRIMARY KEY,
    estimate_id TEXT NOT NULL,
    line_id TEXT NOT NULL,
    revision BIGINT NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (estimate_id, line_id, revision)
);
CREATE INDEX IF NOT EXISTS idx_calc_message_receipts_estimate
    ON calc_message_receipts(estimate_id, line_id, revision);
` + estimateCalcTablesSQL

	_, err := s.treeDB.Exec(ctx, sql)
	if err != nil {
		return err
	}
	if err := s.migrateEstimateLinesCompositePK(ctx); err != nil {
		return err
	}
	return s.migrateEstimateCalcStateStartingStatus(ctx)
}

func (s *FileStore) migrateEstimateCalcStateStartingStatus(ctx context.Context) error {
	if s.treeDB == nil {
		return nil
	}
	var allowsStarting bool
	err := s.treeDB.QueryRow(ctx, `
SELECT EXISTS (
    SELECT 1
    FROM pg_constraint c
    JOIN pg_class t ON t.oid = c.conrelid
    JOIN pg_namespace n ON n.oid = t.relnamespace
    WHERE n.nspname = current_schema()
      AND t.relname = 'estimate_calc_state'
      AND c.conname = 'estimate_calc_state_status_check'
      AND pg_get_constraintdef(c.oid) LIKE '%starting%'
)`).Scan(&allowsStarting)
	if err != nil {
		return err
	}
	if allowsStarting {
		return nil
	}
	var tableExists bool
	if err := s.treeDB.QueryRow(ctx, `
SELECT EXISTS (
    SELECT 1 FROM information_schema.tables
    WHERE table_schema = current_schema() AND table_name = 'estimate_calc_state'
)`).Scan(&tableExists); err != nil {
		return err
	}
	if !tableExists {
		return nil
	}
	_, err = s.treeDB.Exec(ctx, `
ALTER TABLE estimate_calc_state DROP CONSTRAINT IF EXISTS estimate_calc_state_status_check;
ALTER TABLE estimate_calc_state ADD CONSTRAINT estimate_calc_state_status_check
    CHECK (status IN ('', 'starting', 'running', 'done', 'failed'));
`)
	return err
}

func (s *FileStore) migrateEstimateLinesCompositePK(ctx context.Context) error {
	if s.treeDB == nil {
		return nil
	}
	var hasCompositePK bool
	err := s.treeDB.QueryRow(ctx, `
SELECT EXISTS (
    SELECT 1
    FROM information_schema.table_constraints tc
    JOIN information_schema.key_column_usage kcu
      ON tc.constraint_name = kcu.constraint_name
      AND tc.table_schema = kcu.table_schema
    WHERE tc.table_schema = current_schema()
      AND tc.table_name = 'app_estimate_lines'
      AND tc.constraint_type = 'PRIMARY KEY'
      AND kcu.column_name = 'estimate_id'
)`).Scan(&hasCompositePK)
	if err != nil {
		return err
	}
	if hasCompositePK {
		return nil
	}
	var tableExists bool
	if err := s.treeDB.QueryRow(ctx, `
SELECT EXISTS (
    SELECT 1 FROM information_schema.tables
    WHERE table_schema = current_schema() AND table_name = 'app_estimate_lines'
)`).Scan(&tableExists); err != nil {
		return err
	}
	if !tableExists {
		return nil
	}
	_, err = s.treeDB.Exec(ctx, `
ALTER TABLE app_estimate_lines DROP CONSTRAINT IF EXISTS app_estimate_lines_pkey;
ALTER TABLE app_estimate_lines ADD PRIMARY KEY (estimate_id, id);
`)
	return err
}

func (s *FileStore) syncTreeToDB(ctx context.Context) error {
	if s.treeDB == nil {
		return nil
	}

	var constructionCount, objectCount, estimateCount int64
	if err := s.treeDB.QueryRow(ctx, `
SELECT
	(SELECT COUNT(*) FROM app_constructions),
	(SELECT COUNT(*) FROM app_construction_objects),
	(SELECT COUNT(*) FROM app_estimates)`).Scan(&constructionCount, &objectCount, &estimateCount); err != nil {
		return err
	}
	// PG is source of truth once migrated. Re-inserting from legacy app.json
	// resurrected deleted empty drafts (e.g. 4000/1-1) on every restart.
	if constructionCount+objectCount+estimateCount > 0 {
		return s.dropLegacyTreeSnapshot()
	}

	s.mu.RLock()
	constructions := make([]domain.Construction, 0, len(s.constructions))
	for _, item := range s.constructions {
		constructions = append(constructions, item)
	}
	objects := make([]domain.ConstructionObject, 0, len(s.objects))
	for _, item := range s.objects {
		objects = append(objects, item)
	}
	estimates := make([]domain.Estimate, 0, len(s.estimates))
	for _, item := range s.estimates {
		estimates = append(estimates, item)
	}
	s.mu.RUnlock()

	tx, err := s.treeDB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for _, item := range constructions {
		_, err = tx.Exec(ctx, `INSERT INTO app_constructions (id, company_id, code, name, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT (id) DO NOTHING`, item.ID, item.CompanyID, item.Code, item.Name, item.CreatedAt, item.UpdatedAt)
		if err != nil {
			return err
		}
	}
	for _, item := range objects {
		_, err = tx.Exec(ctx, `INSERT INTO app_construction_objects (id, company_id, construction_id, code, name, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7) ON CONFLICT (id) DO NOTHING`, item.ID, item.CompanyID, item.ConstructionID, item.Code, item.Name, item.CreatedAt, item.UpdatedAt)
		if err != nil {
			return err
		}
	}
	for _, item := range estimates {
		_, err = tx.Exec(ctx, `INSERT INTO app_estimates (id, company_id, object_id, code, title, description, district, fgis_set_id, status, total, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) ON CONFLICT (id) DO NOTHING`,
			item.ID, item.CompanyID, item.ObjectID, item.Code, item.Title, item.Description, item.District, item.FgisSetID, item.Status, item.Total, item.CreatedAt, item.UpdatedAt)
		if err != nil {
			return err
		}
		lines := ensureUniqueEstimateLineIDs(item.Items)
		for order, line := range lines {
			line.Revision = estimateLineRevision(item, line)
			_, err = tx.Exec(ctx, `INSERT INTO app_estimate_lines (id, estimate_id, line_type, source, code, original_code, name, quantity, unit, unit_price, total, raw_text, parsed_json, calc_json, calc_status, calc_error, revision, calculated_at, sort_order)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
ON CONFLICT (estimate_id, id) DO UPDATE SET
    line_type = EXCLUDED.line_type,
    source = EXCLUDED.source,
    code = EXCLUDED.code,
    original_code = EXCLUDED.original_code,
    name = EXCLUDED.name,
    quantity = EXCLUDED.quantity,
    unit = EXCLUDED.unit,
    unit_price = EXCLUDED.unit_price,
    total = EXCLUDED.total,
    raw_text = EXCLUDED.raw_text,
    parsed_json = EXCLUDED.parsed_json,
    calc_json = EXCLUDED.calc_json,
    calc_status = EXCLUDED.calc_status,
    calc_error = EXCLUDED.calc_error,
    revision = EXCLUDED.revision,
    calculated_at = EXCLUDED.calculated_at,
    sort_order = EXCLUDED.sort_order`,
				line.ID, item.ID, estimateLineType(line.Type), estimateItemSource(line.Source), line.Code, line.OriginalCode, line.Name, line.Quantity, line.Unit, line.UnitPrice, line.Total, line.RawText, nullableJSON(line.ParsedJSON), nullableJSON(line.CalcJSON), line.CalcStatus, line.CalcError, line.Revision, line.CalculatedAt, order)
			if err != nil {
				return err
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return s.dropLegacyTreeSnapshot()
}

// dropLegacyTreeSnapshot clears constructions/objects/estimates from app.json after
// PG migration so deleted drafts cannot be resurrected on the next restart.
func (s *FileStore) dropLegacyTreeSnapshot() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.constructions) == 0 && len(s.objects) == 0 && len(s.estimates) == 0 {
		return nil
	}
	s.constructions = map[string]domain.Construction{}
	s.objects = map[string]domain.ConstructionObject{}
	s.estimates = map[string]domain.Estimate{}
	return s.saveLocked()
}

func (s *FileStore) removeEstimateFromLegacySnapshot(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.estimates[id]; !ok {
		return nil
	}
	delete(s.estimates, id)
	return s.saveLocked()
}

func (s *FileStore) removeObjectFromLegacySnapshot(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := false
	if _, ok := s.objects[id]; ok {
		delete(s.objects, id)
		changed = true
	}
	for estimateID, estimate := range s.estimates {
		if estimate.ObjectID == id {
			delete(s.estimates, estimateID)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return s.saveLocked()
}

func (s *FileStore) removeConstructionFromLegacySnapshot(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := false
	if _, ok := s.constructions[id]; ok {
		delete(s.constructions, id)
		changed = true
	}
	for objectID, object := range s.objects {
		if object.ConstructionID != id {
			continue
		}
		for estimateID, estimate := range s.estimates {
			if estimate.ObjectID == objectID {
				delete(s.estimates, estimateID)
				changed = true
			}
		}
		delete(s.objects, objectID)
		changed = true
	}
	if !changed {
		return nil
	}
	return s.saveLocked()
}

func (s *FileStore) listConstructionsDB(ctx context.Context, companyID string, includeAll bool) ([]domain.Construction, error) {
	query := `SELECT id, company_id, code, name, created_at, updated_at FROM app_constructions`
	args := []any{}
	if !includeAll {
		query += ` WHERE company_id = $1`
		args = append(args, companyID)
	}
	query += ` ORDER BY code, id`

	rows, err := s.treeDB.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []domain.Construction{}
	for rows.Next() {
		var item domain.Construction
		if err := rows.Scan(&item.ID, &item.CompanyID, &item.Code, &item.Name, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *FileStore) createConstructionDB(ctx context.Context, companyID string, input ConstructionInput) (domain.Construction, error) {
	name := strings.TrimSpace(input.Name)
	code := strings.TrimSpace(input.Code)
	if name == "" || code == "" {
		return domain.Construction{}, ErrConflict
	}

	item := domain.Construction{
		ID:        newID("con"),
		CompanyID: companyID,
		Code:      code,
		Name:      name,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	_, err := s.treeDB.Exec(ctx, `INSERT INTO app_constructions (id, company_id, code, name, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6)`, item.ID, item.CompanyID, item.Code, item.Name, item.CreatedAt, item.UpdatedAt)
	if err != nil {
		return domain.Construction{}, err
	}
	return item, nil
}

func (s *FileStore) updateConstructionDB(ctx context.Context, id, companyID string, includeAll bool, input ConstructionInput) (domain.Construction, error) {
	name := strings.TrimSpace(input.Name)
	code := strings.TrimSpace(input.Code)
	if name == "" || code == "" {
		return domain.Construction{}, ErrConflict
	}

	where := `id = $1`
	args := []any{id}
	if !includeAll {
		where += ` AND company_id = $2`
		args = append(args, companyID)
	}

	query := `UPDATE app_constructions SET code = $` + strconv.Itoa(len(args)+1) + `, name = $` + strconv.Itoa(len(args)+2) + `, updated_at = now() WHERE ` + where + ` RETURNING id, company_id, code, name, created_at, updated_at`
	args = append(args, code, name)
	var item domain.Construction
	if err := s.treeDB.QueryRow(ctx, query, args...).Scan(&item.ID, &item.CompanyID, &item.Code, &item.Name, &item.CreatedAt, &item.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Construction{}, ErrNotFound
		}
		return domain.Construction{}, err
	}
	return item, nil
}

func (s *FileStore) deleteConstructionDB(ctx context.Context, id, companyID string, includeAll bool) error {
	query := `DELETE FROM app_constructions WHERE id = $1`
	args := []any{id}
	if !includeAll {
		query += ` AND company_id = $2`
		args = append(args, companyID)
	}
	tag, err := s.treeDB.Exec(ctx, query, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	if err := s.removeConstructionFromLegacySnapshot(id); err != nil {
		slog.Warn("failed to remove construction from legacy app.json snapshot", "id", id, "error", err)
	}
	return nil
}

func (s *FileStore) listObjectsDB(ctx context.Context, companyID string, includeAll bool) ([]domain.ConstructionObject, error) {
	query := `SELECT id, company_id, construction_id, code, name, created_at, updated_at FROM app_construction_objects`
	args := []any{}
	if !includeAll {
		query += ` WHERE company_id = $1`
		args = append(args, companyID)
	}
	query += ` ORDER BY code, id`

	rows, err := s.treeDB.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []domain.ConstructionObject{}
	for rows.Next() {
		var item domain.ConstructionObject
		if err := rows.Scan(&item.ID, &item.CompanyID, &item.ConstructionID, &item.Code, &item.Name, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *FileStore) createObjectDB(ctx context.Context, companyID string, includeAll bool, input ConstructionObjectInput) (domain.ConstructionObject, error) {
	name := strings.TrimSpace(input.Name)
	code := strings.TrimSpace(input.Code)
	if name == "" || code == "" || strings.TrimSpace(input.ConstructionID) == "" {
		return domain.ConstructionObject{}, ErrConflict
	}

	var parentCompanyID string
	if err := s.treeDB.QueryRow(ctx, `SELECT company_id FROM app_constructions WHERE id = $1`, input.ConstructionID).Scan(&parentCompanyID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ConstructionObject{}, ErrNotFound
		}
		return domain.ConstructionObject{}, err
	}
	if !includeAll && parentCompanyID != companyID {
		return domain.ConstructionObject{}, ErrForbidden
	}

	item := domain.ConstructionObject{
		ID:             newID("obj"),
		CompanyID:      parentCompanyID,
		ConstructionID: input.ConstructionID,
		Code:           code,
		Name:           name,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}
	_, err := s.treeDB.Exec(ctx, `INSERT INTO app_construction_objects (id, company_id, construction_id, code, name, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)`, item.ID, item.CompanyID, item.ConstructionID, item.Code, item.Name, item.CreatedAt, item.UpdatedAt)
	if err != nil {
		return domain.ConstructionObject{}, err
	}
	return item, nil
}

func (s *FileStore) updateObjectDB(ctx context.Context, id, companyID string, includeAll bool, input ConstructionObjectInput) (domain.ConstructionObject, error) {
	name := strings.TrimSpace(input.Name)
	code := strings.TrimSpace(input.Code)
	if name == "" || code == "" {
		return domain.ConstructionObject{}, ErrConflict
	}

	where := `id = $1`
	args := []any{id}
	if !includeAll {
		where += ` AND company_id = $2`
		args = append(args, companyID)
	}
	query := `UPDATE app_construction_objects SET code = $` + strconv.Itoa(len(args)+1) + `, name = $` + strconv.Itoa(len(args)+2) + `, updated_at = now() WHERE ` + where + ` RETURNING id, company_id, construction_id, code, name, created_at, updated_at`
	args = append(args, code, name)
	var item domain.ConstructionObject
	if err := s.treeDB.QueryRow(ctx, query, args...).Scan(&item.ID, &item.CompanyID, &item.ConstructionID, &item.Code, &item.Name, &item.CreatedAt, &item.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ConstructionObject{}, ErrNotFound
		}
		return domain.ConstructionObject{}, err
	}
	return item, nil
}

func (s *FileStore) deleteObjectDB(ctx context.Context, id, companyID string, includeAll bool) error {
	query := `DELETE FROM app_construction_objects WHERE id = $1`
	args := []any{id}
	if !includeAll {
		query += ` AND company_id = $2`
		args = append(args, companyID)
	}
	tag, err := s.treeDB.Exec(ctx, query, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	if err := s.removeObjectFromLegacySnapshot(id); err != nil {
		slog.Warn("failed to remove object from legacy app.json snapshot", "id", id, "error", err)
	}
	return nil
}

func (s *FileStore) listEstimatesDB(ctx context.Context, companyID string, includeAll bool) ([]domain.Estimate, error) {
	items, err := s.listEstimateHeadersDB(ctx, companyID, includeAll)
	if err != nil {
		return nil, err
	}
	if err := s.attachEstimateLinesDB(ctx, items); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *FileStore) listEstimatesSummaryDB(ctx context.Context, companyID string, includeAll bool) ([]domain.Estimate, error) {
	return s.listEstimateHeadersDB(ctx, companyID, includeAll)
}

func (s *FileStore) listEstimateHeadersDB(ctx context.Context, companyID string, includeAll bool) ([]domain.Estimate, error) {
	query := `
SELECT e.id, e.company_id, e.object_id, e.code, e.title, e.description, e.district, e.fgis_set_id, e.status,
    CASE
        WHEN cs.estimate_id IS NOT NULL AND cs.lines_total > 0 THEN cs.grand_total
        ELSE COALESCE((
            SELECT SUM(l.total)
            FROM app_estimate_lines l
            WHERE l.estimate_id = e.id
              AND l.calc_status = 'done'
        ), e.total)
    END,
    COALESCE((
        SELECT COUNT(*)::int
        FROM app_estimate_lines l
        WHERE l.estimate_id = e.id
          AND l.line_type = 'position'
          AND LOWER(COALESCE(l.source, '')) = 'gsn'
          AND COALESCE(NULLIF(TRIM(l.code), ''), NULLIF(TRIM(l.original_code), '')) IS NOT NULL
    ), 0),
    COALESCE((
        SELECT COUNT(*)::int
        FROM app_estimate_lines l
        WHERE l.estimate_id = e.id
          AND l.line_type = 'position'
          AND LOWER(COALESCE(l.source, '')) = 'gsn'
          AND l.calc_status IN ('done', 'failed', 'dead')
          AND COALESCE(NULLIF(TRIM(l.code), ''), NULLIF(TRIM(l.original_code), '')) IS NOT NULL
    ), 0),
    e.created_at, e.updated_at
FROM app_estimates e
LEFT JOIN estimate_calc_state cs ON cs.estimate_id = e.id`
	args := []any{}
	if !includeAll {
		query += ` WHERE e.company_id = $1`
		args = append(args, companyID)
	}
	query += ` ORDER BY e.code, e.id`
	rows, err := s.treeDB.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []domain.Estimate{}
	for rows.Next() {
		var item domain.Estimate
		if err := rows.Scan(&item.ID, &item.CompanyID, &item.ObjectID, &item.Code, &item.Title, &item.Description, &item.District, &item.FgisSetID, &item.Status, &item.Total, &item.CalcLinesTotal, &item.CalcLinesDone, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *FileStore) attachEstimateLinesDB(ctx context.Context, items []domain.Estimate) error {
	if len(items) == 0 {
		return nil
	}

	ids := make([]string, len(items))
	for i := range items {
		ids[i] = items[i].ID
	}

	lineRows, err := s.treeDB.Query(ctx, `SELECT id, estimate_id, line_type, source, code, original_code, name, quantity, unit, unit_price, total, raw_text, parsed_json, calc_json, calc_status, calc_error, revision, calculated_at FROM app_estimate_lines WHERE estimate_id = ANY($1) ORDER BY estimate_id, sort_order, id`, ids)
	if err != nil {
		return err
	}
	defer lineRows.Close()

	lineMap := map[string][]domain.EstimateItem{}
	for lineRows.Next() {
		var line domain.EstimateItem
		var estimateID string
		if err := lineRows.Scan(&line.ID, &estimateID, &line.Type, &line.Source, &line.Code, &line.OriginalCode, &line.Name, &line.Quantity, &line.Unit, &line.UnitPrice, &line.Total, &line.RawText, &line.ParsedJSON, &line.CalcJSON, &line.CalcStatus, &line.CalcError, &line.Revision, &line.CalculatedAt); err != nil {
			return err
		}
		lineMap[estimateID] = append(lineMap[estimateID], line)
	}
	if err := lineRows.Err(); err != nil {
		return err
	}
	for i := range items {
		items[i].Items = lineMap[items[i].ID]
	}
	return nil
}

func (s *FileStore) loadEstimateLinesDB(ctx context.Context, estimateID string) ([]domain.EstimateItem, error) {
	lineRows, err := s.treeDB.Query(ctx, `SELECT id, line_type, source, code, original_code, name, quantity, unit, unit_price, total, raw_text, parsed_json, calc_json, calc_status, calc_error, revision, calculated_at FROM app_estimate_lines WHERE estimate_id = $1 ORDER BY sort_order, id`, estimateID)
	if err != nil {
		return nil, err
	}
	defer lineRows.Close()

	lines := []domain.EstimateItem{}
	for lineRows.Next() {
		var line domain.EstimateItem
		if err := lineRows.Scan(&line.ID, &line.Type, &line.Source, &line.Code, &line.OriginalCode, &line.Name, &line.Quantity, &line.Unit, &line.UnitPrice, &line.Total, &line.RawText, &line.ParsedJSON, &line.CalcJSON, &line.CalcStatus, &line.CalcError, &line.Revision, &line.CalculatedAt); err != nil {
			return nil, err
		}
		lines = append(lines, line)
	}
	return lines, lineRows.Err()
}

func (s *FileStore) getEstimateDB(ctx context.Context, id, companyID string, includeAll bool) (domain.Estimate, error) {
	var item domain.Estimate
	err := s.treeDB.QueryRow(ctx, `SELECT id, company_id, object_id, code, title, description, district, fgis_set_id, status, total, created_at, updated_at FROM app_estimates WHERE id = $1`, id).
		Scan(&item.ID, &item.CompanyID, &item.ObjectID, &item.Code, &item.Title, &item.Description, &item.District, &item.FgisSetID, &item.Status, &item.Total, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Estimate{}, ErrNotFound
		}
		return domain.Estimate{}, err
	}
	if !includeAll && item.CompanyID != companyID {
		return domain.Estimate{}, ErrForbidden
	}

	lines, err := s.loadEstimateLinesDB(ctx, id)
	if err != nil {
		return domain.Estimate{}, err
	}
	item.Items = lines
	return item, nil
}

func (s *FileStore) createEstimateDB(ctx context.Context, companyID string, input EstimateInput) (domain.Estimate, error) {
	input.ObjectID = strings.TrimSpace(input.ObjectID)
	if input.ObjectID == "" {
		return domain.Estimate{}, ErrConflict
	}

	var objectCompanyID string
	if err := s.treeDB.QueryRow(ctx, `SELECT company_id FROM app_construction_objects WHERE id = $1`, input.ObjectID).Scan(&objectCompanyID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Estimate{}, ErrNotFound
		}
		return domain.Estimate{}, err
	}
	if objectCompanyID != companyID {
		return domain.Estimate{}, ErrForbidden
	}

	item, err := buildEstimate(newID("est"), companyID, input, time.Now().UTC(), time.Now().UTC())
	if err != nil {
		return domain.Estimate{}, err
	}
	if err := s.upsertEstimateDB(ctx, item); err != nil {
		return domain.Estimate{}, err
	}
	return item, nil
}

func (s *FileStore) updateEstimateDB(ctx context.Context, id, companyID string, includeAll bool, input EstimateInput) (domain.Estimate, error) {
	current, err := s.getEstimateDB(ctx, id, companyID, includeAll)
	if err != nil {
		return domain.Estimate{}, err
	}

	if strings.TrimSpace(input.ObjectID) == "" {
		input.ObjectID = current.ObjectID
	}
	if strings.TrimSpace(input.Code) == "" {
		input.Code = current.Code
	}
	if strings.TrimSpace(input.Title) == "" {
		input.Title = current.Title
	}
	if input.Description == "" {
		input.Description = current.Description
	}
	if input.District == "" {
		input.District = current.District
	}
	if input.Status == "" {
		input.Status = current.Status
	}
	if len(input.Items) == 0 {
		input.Items = current.Items
	}

	var objectCompanyID string
	if err := s.treeDB.QueryRow(ctx, `SELECT company_id FROM app_construction_objects WHERE id = $1`, input.ObjectID).Scan(&objectCompanyID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Estimate{}, ErrNotFound
		}
		return domain.Estimate{}, err
	}
	if !includeAll && objectCompanyID != companyID {
		return domain.Estimate{}, ErrForbidden
	}

	updated, err := buildEstimate(id, current.CompanyID, input, current.CreatedAt, time.Now().UTC())
	if err != nil {
		return domain.Estimate{}, err
	}
	if err := s.upsertEstimateDB(ctx, updated); err != nil {
		return domain.Estimate{}, err
	}
	return updated, nil
}

func (s *FileStore) upsertEstimateDB(ctx context.Context, item domain.Estimate) error {
	tx, err := s.treeDB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `INSERT INTO app_estimates (id, company_id, object_id, code, title, description, district, fgis_set_id, status, total, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 0, $10, $11)
ON CONFLICT (id) DO UPDATE SET object_id = EXCLUDED.object_id, code = EXCLUDED.code, title = EXCLUDED.title, description = EXCLUDED.description, district = EXCLUDED.district, fgis_set_id = EXCLUDED.fgis_set_id, status = EXCLUDED.status, total = 0, updated_at = EXCLUDED.updated_at`,
		item.ID, item.CompanyID, item.ObjectID, item.Code, item.Title, item.Description, item.District, item.FgisSetID, item.Status, item.CreatedAt, item.UpdatedAt)
	if err != nil {
		return err
	}

	existingLines, err := loadStoredEstimateLines(ctx, tx, item.ID)
	if err != nil {
		return err
	}
	var calcGeneration int64
	if err := tx.QueryRow(ctx, `SELECT calc_generation FROM app_estimates WHERE id = $1`, item.ID).Scan(&calcGeneration); err != nil {
		return err
	}
	item.CalcGeneration = calcGeneration

	newIDs := make(map[string]struct{}, len(item.Items))
	for _, line := range item.Items {
		id := strings.TrimSpace(line.ID)
		if id != "" {
			newIDs[id] = struct{}{}
		}
	}
	for existingID := range existingLines {
		if _, keep := newIDs[existingID]; keep {
			continue
		}
		if err := removeLineCalcFromGenerationTx(ctx, tx, item.ID, calcGeneration, existingID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
DELETE FROM calc_message_receipts WHERE estimate_id = $1 AND line_id = $2
`, item.ID, existingID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
UPDATE estimate_calc_jobs
SET status = 'dead', last_error = 'line removed', leased_until = NULL, locked_by = '', updated_at = now()
WHERE estimate_id = $1 AND line_id = $2 AND status IN ('queued', 'leased')
`, item.ID, existingID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
DELETE FROM outbox_events
WHERE published_at IS NULL
  AND payload->>'estimateId' = $1
  AND payload->>'lineId' = $2
`, item.ID, existingID); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(ctx, `DELETE FROM app_estimate_lines WHERE estimate_id = $1`, item.ID); err != nil {
		return err
	}

	lines := ensureUniqueEstimateLineIDs(item.Items)
	enqueueLines := make([]domain.EstimateItem, 0)
	for i, line := range lines {
		existing, hasExisting := existingLines[line.ID]
		line = prepareEstimateLineForStorage(item, line, existing, hasExisting)
		if line.Revision == 0 {
			line.Revision = estimateLineRevision(item, line)
		}
		_, err = tx.Exec(ctx, `INSERT INTO app_estimate_lines (id, estimate_id, line_type, source, code, original_code, name, quantity, unit, unit_price, total, raw_text, parsed_json, calc_json, calc_status, calc_error, revision, calculated_at, sort_order)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)`,
			line.ID, item.ID, estimateLineType(line.Type), estimateItemSource(line.Source), line.Code, line.OriginalCode, line.Name, line.Quantity, line.Unit, line.UnitPrice, line.Total, line.RawText, nullableJSON(line.ParsedJSON), nullableJSON(line.CalcJSON), line.CalcStatus, line.CalcError, line.Revision, line.CalculatedAt, i)
		if err != nil {
			return err
		}

		if shouldEnqueueEstimateLineCalc(line) {
			isNew := !hasExisting
			needsRecalc := isNew || !isCalcTerminalStatus(line.CalcStatus) || line.CalcStatus == ""
			if hasExisting && existing.Revision != line.Revision {
				needsRecalc = true
			}
			if needsRecalc {
				enqueueLines = append(enqueueLines, line)
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	if len(enqueueLines) > 0 && !s.disableCalcEnqueue {
		go s.enqueueEstimateLinesAfterSave(observability.DetachCorrelation(ctx), item, enqueueLines)
	}
	return nil
}

func (s *FileStore) enqueueEstimateLinesAfterSave(ctx context.Context, estimate domain.Estimate, lines []domain.EstimateItem) {
	hasCalc, err := s.HasEstimateCalcLines(ctx, estimate.ID)
	if err != nil || !hasCalc {
		// No prior calculation state: wait for explicit StartEstimateCalc / table view.
		return
	}
	tx, err := s.treeDB.Begin(ctx)
	if err != nil {
		slog.Warn("incremental enqueue begin failed", "estimate", estimate.ID, "error", err)
		return
	}
	defer tx.Rollback(ctx)
	if err := lockEstimateCalcState(ctx, tx, estimate.ID); err != nil {
		slog.Warn("incremental enqueue lock failed", "estimate", estimate.ID, "error", err)
		return
	}
	if _, err := tx.Exec(ctx, `
UPDATE estimate_calc_state
SET lines_total = lines_total + $2,
    status = 'running',
    updated_at = now()
WHERE estimate_id = $1
`, estimate.ID, len(lines)); err != nil {
		slog.Warn("incremental enqueue state update failed", "estimate", estimate.ID, "error", err)
		return
	}
	for _, line := range lines {
		line.Revision = estimateLineRevision(estimate, line)
		if _, err := tx.Exec(ctx, `
UPDATE app_estimate_lines
SET calc_status = 'queued',
    calc_error = '',
    calc_json = NULL,
    unit_price = 0,
    total = 0,
    calculated_at = NULL,
    revision = $3
WHERE estimate_id = $1 AND id = $2
`, estimate.ID, line.ID, line.Revision); err != nil {
			slog.Warn("incremental enqueue line update failed", "estimate", estimate.ID, "line", line.ID, "error", err)
			return
		}
		if err := enqueueEstimateLineCalcJobTx(ctx, tx, estimate, line, s.queueMode); err != nil {
			slog.Warn("incremental enqueue job failed", "estimate", estimate.ID, "line", line.ID, "error", err)
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		slog.Warn("incremental enqueue commit failed", "estimate", estimate.ID, "error", err)
	}
}

func (s *FileStore) deleteEstimateDB(ctx context.Context, id, companyID string, includeAll bool) error {
	query := `DELETE FROM app_estimates WHERE id = $1`
	args := []any{id}
	if !includeAll {
		query += ` AND company_id = $2`
		args = append(args, companyID)
	}
	tag, err := s.treeDB.Exec(ctx, query, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	if err := s.removeEstimateFromLegacySnapshot(id); err != nil {
		slog.Warn("failed to remove estimate from legacy app.json snapshot", "id", id, "error", err)
	}
	return nil
}

type UpdateAppSettingsInput struct {
	CalcWorkerCount     int `json:"calcWorkerCount"`
	CalcStartBatchSize  int `json:"calcStartBatchSize"`
	CalcClientBatchSize int `json:"calcClientBatchSize"`
}

func defaultAppSettings() domain.AppSettings {
	return domain.AppSettings{
		CalcWorkerCount:     2,
		CalcStartBatchSize:  50,
		CalcClientBatchSize: 50,
	}
}

func NormalizeCalcWorkerCount(value int) int {
	if value < 1 {
		return 1
	}
	if value > 8 {
		return 8
	}
	return value
}

func NormalizeCalcStartBatchSize(value int) int {
	if value < 1 {
		return 1
	}
	if value > 500 {
		return 500
	}
	return value
}

func NormalizeCalcClientBatchSize(value int) int {
	if value < 1 {
		return 1
	}
	if value > 500 {
		return 500
	}
	return value
}

func normalizeAppSettings(settings domain.AppSettings) domain.AppSettings {
	if settings.CalcWorkerCount == 0 {
		settings.CalcWorkerCount = defaultAppSettings().CalcWorkerCount
	}
	if settings.CalcStartBatchSize == 0 {
		settings.CalcStartBatchSize = defaultAppSettings().CalcStartBatchSize
	}
	if settings.CalcClientBatchSize == 0 {
		settings.CalcClientBatchSize = defaultAppSettings().CalcClientBatchSize
	}
	settings.CalcWorkerCount = NormalizeCalcWorkerCount(settings.CalcWorkerCount)
	settings.CalcStartBatchSize = NormalizeCalcStartBatchSize(settings.CalcStartBatchSize)
	settings.CalcClientBatchSize = NormalizeCalcClientBatchSize(settings.CalcClientBatchSize)
	return settings
}

func (s *FileStore) GetAppSettings() (domain.AppSettings, error) {
	if s.treeDB != nil {
		return s.getAppSettingsDB(context.Background())
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	return normalizeAppSettings(s.settings), nil
}

func (s *FileStore) UpdateAppSettings(input UpdateAppSettingsInput) (domain.AppSettings, error) {
	settings := domain.AppSettings{
		CalcWorkerCount:     NormalizeCalcWorkerCount(input.CalcWorkerCount),
		CalcStartBatchSize:  NormalizeCalcStartBatchSize(input.CalcStartBatchSize),
		CalcClientBatchSize: NormalizeCalcClientBatchSize(input.CalcClientBatchSize),
	}
	if s.treeDB != nil {
		return s.updateAppSettingsDB(context.Background(), settings)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings = settings
	return settings, s.saveLocked()
}

func (s *FileStore) getAppSettingsDB(ctx context.Context) (domain.AppSettings, error) {
	settings := defaultAppSettings()
	rows, err := s.treeDB.Query(ctx, `SELECT key, value FROM app_settings WHERE key IN ('calc_worker_count', 'calc_start_batch_size', 'calc_client_batch_size')`)
	if err != nil {
		return domain.AppSettings{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return domain.AppSettings{}, err
		}
		if key == "calc_worker_count" {
			if parsed, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
				settings.CalcWorkerCount = parsed
			}
		}
		if key == "calc_start_batch_size" {
			if parsed, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
				settings.CalcStartBatchSize = parsed
			}
		}
		if key == "calc_client_batch_size" {
			if parsed, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
				settings.CalcClientBatchSize = parsed
			}
		}
	}
	if err := rows.Err(); err != nil {
		return domain.AppSettings{}, err
	}
	return normalizeAppSettings(settings), nil
}

func (s *FileStore) updateAppSettingsDB(ctx context.Context, settings domain.AppSettings) (domain.AppSettings, error) {
	settings = normalizeAppSettings(settings)
	_, err := s.treeDB.Exec(ctx, `
INSERT INTO app_settings (key, value, updated_at)
VALUES ('calc_worker_count', $1, now())
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()
`, strconv.Itoa(settings.CalcWorkerCount))
	if err != nil {
		return domain.AppSettings{}, err
	}
	_, err = s.treeDB.Exec(ctx, `
INSERT INTO app_settings (key, value, updated_at)
VALUES ('calc_start_batch_size', $1, now())
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()
`, strconv.Itoa(settings.CalcStartBatchSize))
	if err != nil {
		return domain.AppSettings{}, err
	}
	_, err = s.treeDB.Exec(ctx, `
INSERT INTO app_settings (key, value, updated_at)
VALUES ('calc_client_batch_size', $1, now())
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()
`, strconv.Itoa(settings.CalcClientBatchSize))
	if err != nil {
		return domain.AppSettings{}, err
	}
	return settings, nil
}

type EstimateCalcJob struct {
	ID          string
	CompanyID   string
	EstimateID  string
	LineID      string
	Revision    int64
	Generation  int64
	Code        string
	FgisSetID   string
	District    string
	Quantity    float64
	RawText     string
	Attempts    int
	MaxAttempts int
	RequestID   string
}

type EstimateLineCalcResult struct {
	Code         string
	OriginalCode string
	Name         string
	Unit         string
	Quantity     float64
	UnitPrice    float64
	Total        float64
	CalcJSON     json.RawMessage
	Snapshot     *estimatecalc.LineCalcSnapshot
}

type EstimateCalcStatus struct {
	LineID       string          `json:"lineId"`
	Revision     int64           `json:"revision"`
	Status       string          `json:"status"`
	Error        string          `json:"error,omitempty"`
	Code         string          `json:"code,omitempty"`
	OriginalCode string          `json:"originalCode,omitempty"`
	Name         string          `json:"name,omitempty"`
	Unit         string          `json:"unit,omitempty"`
	Quantity     float64         `json:"quantity,omitempty"`
	UnitPrice    float64         `json:"unitPrice,omitempty"`
	Total        float64         `json:"total,omitempty"`
	CalcJSON     json.RawMessage `json:"calcJson,omitempty"`
	CalculatedAt *time.Time      `json:"calculatedAt,omitempty"`
}

// EstimateCalcSummary is a compact aggregate for progress polling on large estimates.
type EstimateCalcSummary struct {
	Total      int     `json:"total"`
	Processed  int     `json:"processed"`
	Errors     int     `json:"errors"`
	GrandTotal float64 `json:"grandTotal"`
	Status     string  `json:"status,omitempty"`
}

func (s *FileStore) EstimateLineQuantityContext(ctx context.Context, estimateID, lineID string) (quantity float64, rawText string, err error) {
	if s.treeDB == nil {
		return 0, "", nil
	}
	err = s.treeDB.QueryRow(ctx, `
SELECT quantity, COALESCE(raw_text, '')
FROM app_estimate_lines
WHERE estimate_id = $1 AND id = $2
`, estimateID, lineID).Scan(&quantity, &rawText)
	return quantity, rawText, err
}

func (s *FileStore) ClaimEstimateCalcJob(ctx context.Context, workerID string, lease time.Duration) (EstimateCalcJob, bool, error) {
	if s.treeDB == nil {
		return EstimateCalcJob{}, false, nil
	}
	if lease <= 0 {
		lease = 30 * time.Second
	}

	var job EstimateCalcJob
	err := s.treeDB.QueryRow(ctx, `
UPDATE estimate_calc_jobs
SET status = 'leased',
    attempts = attempts + 1,
    leased_until = $1,
    locked_by = $2,
    updated_at = now()
WHERE id = (
    SELECT id
    FROM estimate_calc_jobs
    WHERE (status = 'queued' AND run_after <= now())
        OR (status = 'leased' AND leased_until IS NOT NULL AND leased_until < now())
    ORDER BY priority DESC, run_after, created_at
    FOR UPDATE SKIP LOCKED
    LIMIT 1
)
RETURNING id, company_id, estimate_id, line_id, revision, COALESCE((payload->>'generation')::bigint, 0), payload->>'code', payload->>'fgisSetId', payload->>'district', COALESCE((payload->>'quantity')::double precision, 0), COALESCE(payload->>'rawText', ''), attempts, max_attempts
`, time.Now().UTC().Add(lease), workerID).Scan(
		&job.ID,
		&job.CompanyID,
		&job.EstimateID,
		&job.LineID,
		&job.Revision,
		&job.Generation,
		&job.Code,
		&job.FgisSetID,
		&job.District,
		&job.Quantity,
		&job.RawText,
		&job.Attempts,
		&job.MaxAttempts,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return EstimateCalcJob{}, false, nil
		}
		return EstimateCalcJob{}, false, err
	}
	_, _ = s.treeDB.Exec(ctx, `
UPDATE app_estimate_lines
SET calc_status = 'leased',
    calc_error = ''
WHERE estimate_id = $1 AND id = $2 AND revision = $3
`, job.EstimateID, job.LineID, job.Revision)
	return job, true, nil
}

func (s *FileStore) ReleaseStuckCalcJobLeases(ctx context.Context) (int64, error) {
	if s.treeDB == nil {
		return 0, nil
	}
	tag, err := s.treeDB.Exec(ctx, `
UPDATE estimate_calc_jobs
SET status = 'queued',
    leased_until = NULL,
    locked_by = '',
    updated_at = now()
WHERE status = 'leased'
    AND leased_until IS NOT NULL
    AND leased_until < now()
`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *FileStore) CompleteEstimateCalcJob(ctx context.Context, job EstimateCalcJob, result EstimateLineCalcResult) error {
	if s.treeDB == nil {
		return nil
	}
	tx, err := s.treeDB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx, `
UPDATE app_estimate_lines
SET code = CASE WHEN $4 <> '' THEN $4 ELSE code END,
    original_code = CASE WHEN $5 <> '' THEN $5 ELSE original_code END,
    name = CASE WHEN $6 <> '' THEN $6 ELSE name END,
    unit = CASE WHEN $7 <> '' THEN $7 ELSE unit END,
    quantity = $8,
    unit_price = $9,
    total = $10,
    calc_json = $11,
    calc_status = 'done',
    calc_error = '',
    calculated_at = now()
WHERE estimate_id = $1 AND id = $2 AND revision = $3
`, job.EstimateID, job.LineID, job.Revision, result.Code, result.OriginalCode, result.Name, result.Unit, result.Quantity, result.UnitPrice, result.Total, nullableJSON(result.CalcJSON))
	if err != nil {
		return err
	}

	status := "done"
	lastError := ""
	if tag.RowsAffected() == 0 {
		status = "failed"
		lastError = "stale line revision"
		if _, err := tx.Exec(ctx, `
UPDATE app_estimate_lines
SET calc_status = 'failed',
    calc_error = $3
WHERE estimate_id = $1 AND id = $2
`, job.EstimateID, job.LineID, lastError); err != nil {
			return err
		}
	}
	if status == "done" {
		generation := job.Generation
		if generation == 0 {
			generation, err = loadEstimateGeneration(ctx, tx, job.EstimateID)
			if err != nil {
				return err
			}
		}
		positionNo, err := loadLineSortOrder(ctx, tx, job.EstimateID, job.LineID)
		if err != nil {
			return err
		}
		snap := result.Snapshot
		if snap == nil {
			snap = &estimatecalc.LineCalcSnapshot{
				Code:         result.Code,
				OriginalCode: result.OriginalCode,
				Name:         result.Name,
				Unit:         result.Unit,
				Quantity:     result.Quantity,
				UnitPrice:    result.UnitPrice,
				Total:        result.Total,
			}
		}
		if err := applyDoneLineCalcTx(ctx, tx, lineCalcPersistInput{
			EstimateID:   job.EstimateID,
			Generation:   generation,
			LineID:       job.LineID,
			LineRevision: job.Revision,
			PositionNo:   positionNo,
			Snapshot:     *snap,
		}); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO calc_message_receipts (id, estimate_id, line_id, revision)
VALUES ($1, $2, $3, $4)
ON CONFLICT (estimate_id, line_id, revision) DO NOTHING
`, job.ID, job.EstimateID, job.LineID, job.Revision); err != nil {
			return err
		}
	}
	if s.queueMode != "rabbit" {
		if _, err := tx.Exec(ctx, `
UPDATE estimate_calc_jobs
SET status = $2,
    last_error = $3,
    leased_until = NULL,
    locked_by = '',
    updated_at = now()
WHERE id = $1
`, job.ID, status, lastError); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *FileStore) FailEstimateCalcJob(ctx context.Context, job EstimateCalcJob, cause error) error {
	if s.treeDB == nil {
		return nil
	}
	message := ""
	if cause != nil {
		message = cause.Error()
	}
	nextStatus := "queued"
	if job.Attempts >= job.MaxAttempts {
		nextStatus = "dead"
	}
	backoff := time.Duration(job.Attempts*job.Attempts) * time.Second
	if backoff < 2*time.Second {
		backoff = 2 * time.Second
	}
	if backoff > time.Minute {
		backoff = time.Minute
	}

	tx, err := s.treeDB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if s.queueMode != "rabbit" {
		if _, err := tx.Exec(ctx, `
UPDATE estimate_calc_jobs
SET status = $2,
    run_after = $3,
    leased_until = NULL,
    locked_by = '',
    last_error = $4,
    updated_at = now()
WHERE id = $1
`, job.ID, nextStatus, time.Now().UTC().Add(backoff), message); err != nil {
			return err
		}
	}
	lineStatus := "failed"
	if nextStatus == "dead" {
		lineStatus = "dead"
	}
	tag, err := tx.Exec(ctx, `
UPDATE app_estimate_lines
SET calc_status = $4,
    calc_error = $5
WHERE estimate_id = $1 AND id = $2 AND revision = $3
`, job.EstimateID, job.LineID, job.Revision, lineStatus, message)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		if _, err := tx.Exec(ctx, `
UPDATE app_estimate_lines
SET calc_status = $3,
    calc_error = $4
WHERE estimate_id = $1 AND id = $2
`, job.EstimateID, job.LineID, lineStatus, message); err != nil {
			return err
		}
	}
	if lineStatus == "dead" {
		if err := reconcileEstimateCalcStateTx(ctx, tx, job.EstimateID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *FileStore) ListEstimateCalcStatuses(ctx context.Context, companyID, estimateID string, includeAll bool, lite bool) ([]EstimateCalcStatus, error) {
	if s.treeDB == nil {
		return []EstimateCalcStatus{}, nil
	}
	calcJSONExpr := "l.calc_json"
	if lite {
		calcJSONExpr = "NULL::jsonb"
	}
	query := fmt.Sprintf(`
SELECT l.id, l.revision, l.calc_status, l.calc_error, l.code, l.original_code, l.name, l.unit, l.quantity, l.unit_price, l.total, %s, l.calculated_at
FROM app_estimate_lines l
JOIN app_estimates e ON e.id = l.estimate_id
WHERE l.estimate_id = $1`, calcJSONExpr)
	args := []any{estimateID}
	if !includeAll {
		query += ` AND e.company_id = $2`
		args = append(args, companyID)
	}
	query += ` ORDER BY l.sort_order, l.id`

	rows, err := s.treeDB.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []EstimateCalcStatus{}
	for rows.Next() {
		var item EstimateCalcStatus
		if err := rows.Scan(&item.LineID, &item.Revision, &item.Status, &item.Error, &item.Code, &item.OriginalCode, &item.Name, &item.Unit, &item.Quantity, &item.UnitPrice, &item.Total, &item.CalcJSON, &item.CalculatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *FileStore) SummarizeEstimateCalcStatus(ctx context.Context, companyID, estimateID string, includeAll bool) (EstimateCalcSummary, error) {
	if s.treeDB == nil {
		return EstimateCalcSummary{}, nil
	}
	if _, err := s.loadEstimateCalcMeta(ctx, companyID, estimateID, includeAll); err != nil {
		return EstimateCalcSummary{}, err
	}

	var summary EstimateCalcSummary
	err := s.treeDB.QueryRow(ctx, `
SELECT lines_total, lines_done, lines_errors, grand_total, COALESCE(status, '')
FROM estimate_calc_state
WHERE estimate_id = $1
`, estimateID).Scan(&summary.Total, &summary.Processed, &summary.Errors, &summary.GrandTotal, &summary.Status)
	if err == nil {
		status := strings.TrimSpace(summary.Status)
		if status == "starting" || status == "running" {
			if recErr := s.reconcileEstimateCalcStateIfIdle(ctx, estimateID); recErr == nil {
				err = s.treeDB.QueryRow(ctx, `
SELECT lines_total, lines_done, lines_errors, grand_total, COALESCE(status, '')
FROM estimate_calc_state
WHERE estimate_id = $1
`, estimateID).Scan(&summary.Total, &summary.Processed, &summary.Errors, &summary.GrandTotal, &summary.Status)
			}
		}
		if err == nil {
			status = strings.TrimSpace(summary.Status)
			if summary.Total > 0 || status == "starting" || status == "running" {
				return summary, nil
			}
		}
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return summary, err
	}

	query := `
SELECT
    COUNT(*)::int,
    COUNT(*) FILTER (WHERE l.calc_status IN ('done', 'failed', 'dead'))::int,
    COALESCE(SUM(l.total) FILTER (WHERE l.calc_status = 'done'), 0)
FROM app_estimate_lines l
JOIN app_estimates e ON e.id = l.estimate_id
WHERE l.estimate_id = $1
    AND l.line_type = $2
    AND l.source = 'gsn'
    AND COALESCE(NULLIF(TRIM(l.code), ''), NULLIF(TRIM(l.original_code), '')) IS NOT NULL`
	args := []any{estimateID, string(domain.EstimateLinePosition)}
	if !includeAll {
		query += ` AND e.company_id = $3`
		args = append(args, companyID)
	}
	err = s.treeDB.QueryRow(ctx, query, args...).Scan(
		&summary.Total,
		&summary.Processed,
		&summary.GrandTotal,
	)
	if err != nil {
		return summary, err
	}
	summary.Errors, err = s.countEstimateCalcErrors(ctx, companyID, estimateID, includeAll)
	return summary, err
}

func (s *FileStore) countEstimateCalcErrors(ctx context.Context, companyID, estimateID string, includeAll bool) (int, error) {
	query := `
SELECT l.code, l.original_code, l.raw_text, l.calc_status
FROM app_estimate_lines l
JOIN app_estimates e ON e.id = l.estimate_id
WHERE l.estimate_id = $1
    AND l.line_type = $2
    AND l.source = 'gsn'
    AND l.calc_status IN ('failed', 'dead')`
	args := []any{estimateID, string(domain.EstimateLinePosition)}
	if !includeAll {
		query += ` AND e.company_id = $3`
		args = append(args, companyID)
	}
	rows, err := s.treeDB.Query(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	errorsCount := 0
	for rows.Next() {
		var code, originalCode, rawText, status string
		if err := rows.Scan(&code, &originalCode, &rawText, &status); err != nil {
			return 0, err
		}
		lookupCode := strings.TrimSpace(code)
		if lookupCode == "" {
			lookupCode = strings.TrimSpace(originalCode)
		}
		if IsUserCatalogCipherCode(lookupCode) {
			fields, parseErr := ParseSourceDataPositionFields(rawText)
			if parseErr == nil && !UserCatalogPositionNeedsLookup(fields) {
				continue
			}
		}
		errorsCount++
	}
	return errorsCount, rows.Err()
}

func (s *FileStore) StartEstimateCalc(ctx context.Context, id, companyID string, includeAll bool, force bool) error {
	if s.treeDB == nil {
		return nil
	}
	if s.disableCalcEnqueue {
		return nil
	}

	var estimateCompanyID, district, fgisSetID string
	var calcGeneration int64
	err := s.treeDB.QueryRow(ctx, `
SELECT company_id, district, fgis_set_id, calc_generation
FROM app_estimates
WHERE id = $1
`, id).Scan(&estimateCompanyID, &district, &fgisSetID, &calcGeneration)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if !includeAll && estimateCompanyID != companyID {
		return ErrForbidden
	}

	hasCalc, err := s.HasEstimateCalcLines(ctx, id)
	if err != nil {
		return err
	}
	var stateDistrict, stateFgis string
	var stateGen int64
	var hasState bool
	err = s.treeDB.QueryRow(ctx, `
SELECT generation, district, fgis_set_id
FROM estimate_calc_state
WHERE estimate_id = $1
`, id).Scan(&stateGen, &stateDistrict, &stateFgis)
	if err == nil {
		hasState = true
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	fullRebuild := force || !hasCalc
	if hasState && (stateDistrict != district || stateFgis != fgisSetID) {
		fullRebuild = true
	}

	if force || (hasState && (stateDistrict != district || stateFgis != fgisSetID)) {
		if _, err := s.CancelEstimateCalc(ctx, companyID, id, includeAll); err != nil {
			return err
		}
		err = s.treeDB.QueryRow(ctx, `
SELECT company_id, district, fgis_set_id, calc_generation
FROM app_estimates WHERE id = $1
`, id).Scan(&estimateCompanyID, &district, &fgisSetID, &calcGeneration)
		if err != nil {
			return err
		}
		fullRebuild = true
	}

	estimate := domain.Estimate{
		ID:             id,
		CompanyID:      estimateCompanyID,
		District:       district,
		FgisSetID:      fgisSetID,
		CalcGeneration: calcGeneration,
	}
	settings, err := s.GetAppSettings()
	if err != nil {
		return err
	}
	for attempt := 0; attempt < 120; attempt++ {
		if s.markEstimateCalcStartJob(id) {
			if _, err := s.treeDB.Exec(ctx, `
INSERT INTO estimate_calc_state (
    estimate_id, generation, district, fgis_set_id, status,
    grand_total, lines_total, lines_done, lines_errors, updated_at
) VALUES ($1, $2, $3, $4, 'starting', 0, 0, 0, 0, now())
ON CONFLICT (estimate_id) DO UPDATE SET
    generation = EXCLUDED.generation,
    district = EXCLUDED.district,
    fgis_set_id = EXCLUDED.fgis_set_id,
    status = 'starting',
    updated_at = now()
`, estimate.ID, estimate.CalcGeneration, estimate.District, estimate.FgisSetID); err != nil {
				s.unmarkEstimateCalcStartJob(id)
				return err
			}
			go s.runEstimateCalcStart(observability.DetachCorrelation(ctx), estimate, settings.CalcStartBatchSize, fullRebuild)
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	return fmt.Errorf("estimate calc start already in progress")
}

func (s *FileStore) runEstimateCalcStart(ctx context.Context, estimate domain.Estimate, batchSize int, fullRebuild bool) {
	defer s.unmarkEstimateCalcStartJob(estimate.ID)
	ctx, endSpan := observability.StartEstimateEnqueueSpan(ctx, estimate.ID)
	defer endSpan()
	if err := s.enqueueEstimateCalcBatches(ctx, estimate, batchSize, fullRebuild); err != nil {
		slog.Warn("estimate calc enqueue failed", "estimate", estimate.ID, "error", err)
	}
}

func (s *FileStore) markEstimateCalcStartJob(estimateID string) bool {
	s.calcStartMu.Lock()
	defer s.calcStartMu.Unlock()
	if _, exists := s.calcStartJobs[estimateID]; exists {
		return false
	}
	s.calcStartJobs[estimateID] = struct{}{}
	return true
}

func (s *FileStore) unmarkEstimateCalcStartJob(estimateID string) {
	s.calcStartMu.Lock()
	defer s.calcStartMu.Unlock()
	delete(s.calcStartJobs, estimateID)
}

func (s *FileStore) enqueueEstimateCalcBatches(ctx context.Context, estimate domain.Estimate, batchSize int, fullRebuild bool) error {
	if s.disableCalcEnqueue {
		return nil
	}
	if batchSize <= 0 {
		batchSize = defaultAppSettings().CalcStartBatchSize
	}
	rows, err := s.treeDB.Query(ctx, `
SELECT id, line_type, source, code, original_code, name, quantity, unit, unit_price, total, raw_text, calc_status, revision
FROM app_estimate_lines
WHERE estimate_id = $1
ORDER BY sort_order, id
`, estimate.ID)
	if err != nil {
		return err
	}
	defer rows.Close()

	type startLine struct {
		line       domain.EstimateItem
		calcStatus string
		revision   int64
	}
	lines := make([]startLine, 0)
	for rows.Next() {
		var line domain.EstimateItem
		var calcStatus string
		var revision int64
		if err := rows.Scan(
			&line.ID, &line.Type, &line.Source, &line.Code, &line.OriginalCode, &line.Name,
			&line.Quantity, &line.Unit, &line.UnitPrice, &line.Total, &line.RawText,
			&calcStatus, &revision,
		); err != nil {
			return err
		}
		lines = append(lines, startLine{line: line, calcStatus: calcStatus, revision: revision})
	}
	if err := rows.Err(); err != nil {
		return err
	}

	enqueueCandidates := make([]domain.EstimateItem, 0, len(lines))
	for _, row := range lines {
		line := prepareEstimateLineForStorage(estimate, row.line, storedEstimateLineCalc{}, false)
		if !shouldEnqueueEstimateLineCalc(line) {
			continue
		}
		if !fullRebuild {
			newRevision := estimateLineRevision(estimate, line)
			if strings.TrimSpace(row.calcStatus) == "done" && row.revision == newRevision {
				continue
			}
		}
		enqueueCandidates = append(enqueueCandidates, line)
	}

	if fullRebuild {
		tx, err := s.treeDB.Begin(ctx)
		if err != nil {
			return err
		}
		if err := clearEstimateCalcGenerationTx(ctx, tx, estimate.ID, estimate.CalcGeneration, estimate.District, estimate.FgisSetID, len(enqueueCandidates)); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		if len(enqueueCandidates) == 0 {
			if _, err := tx.Exec(ctx, `
UPDATE estimate_calc_state
SET status = 'done', updated_at = now()
WHERE estimate_id = $1
`, estimate.ID); err != nil {
				_ = tx.Rollback(ctx)
				return err
			}
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		if len(enqueueCandidates) == 0 {
			return nil
		}
	} else if len(enqueueCandidates) > 0 {
		tx, err := s.treeDB.Begin(ctx)
		if err != nil {
			return err
		}
		if err := lockEstimateCalcState(ctx, tx, estimate.ID); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		doneCount := 0
		for _, row := range lines {
			if strings.TrimSpace(row.calcStatus) == "done" {
				doneCount++
			}
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO estimate_calc_state (
    estimate_id, generation, district, fgis_set_id, status,
    grand_total, lines_total, lines_done, lines_errors, updated_at
) VALUES ($1, $2, $3, $4, 'running', 0, $5, $6, 0, now())
ON CONFLICT (estimate_id) DO UPDATE SET
    generation = EXCLUDED.generation,
    district = EXCLUDED.district,
    fgis_set_id = EXCLUDED.fgis_set_id,
    status = 'running',
    lines_total = EXCLUDED.lines_total,
    updated_at = now()
`, estimate.ID, estimate.CalcGeneration, estimate.District, estimate.FgisSetID,
			doneCount+len(enqueueCandidates), doneCount); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
	} else {
		if _, err := s.treeDB.Exec(ctx, `
UPDATE estimate_calc_state
SET status = 'done', updated_at = now()
WHERE estimate_id = $1 AND status = 'starting'
`, estimate.ID); err != nil {
			return err
		}
		return nil
	}

	for batchStart := 0; batchStart < len(enqueueCandidates); batchStart += batchSize {
		var currentGeneration int64
		if err := s.treeDB.QueryRow(ctx, `SELECT calc_generation FROM app_estimates WHERE id = $1`, estimate.ID).Scan(&currentGeneration); err != nil {
			return err
		}
		if currentGeneration != estimate.CalcGeneration {
			return nil
		}

		batchEnd := batchStart + batchSize
		if batchEnd > len(enqueueCandidates) {
			batchEnd = len(enqueueCandidates)
		}
		tx, err := s.treeDB.Begin(ctx)
		if err != nil {
			return err
		}
		if batchStart == 0 && fullRebuild {
			if _, err := tx.Exec(ctx, `DELETE FROM calc_message_receipts WHERE estimate_id = $1`, estimate.ID); err != nil {
				_ = tx.Rollback(ctx)
				return err
			}
		}
		for _, line := range enqueueCandidates[batchStart:batchEnd] {
			line.Revision = estimateLineRevision(estimate, line)
			if _, err := tx.Exec(ctx, `
UPDATE app_estimate_lines
SET calc_status = 'queued',
    calc_error = '',
    calc_json = NULL,
    unit_price = 0,
    total = 0,
    calculated_at = NULL,
    revision = $3
WHERE estimate_id = $1 AND id = $2
`, estimate.ID, line.ID, line.Revision); err != nil {
				_ = tx.Rollback(ctx)
				return err
			}
			if err := enqueueEstimateLineCalcJobTx(ctx, tx, estimate, line, s.queueMode); err != nil {
				_ = tx.Rollback(ctx)
				return err
			}
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}

func envBool(key string) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return false
	}
	switch strings.ToLower(v) {
	case "1", "true", "t", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func prepareEstimateLineForStorage(estimate domain.Estimate, line domain.EstimateItem, existing storedEstimateLineCalc, hasExisting bool) domain.EstimateItem {
	normalized, _, err := normalizeEstimateItem(line)
	if err != nil {
		return line
	}
	line = normalized
	if !shouldEnqueueEstimateLineCalc(line) {
		return line
	}
	newRevision := estimateLineRevision(estimate, line)
	line.Revision = newRevision
	if hasExisting && existing.Revision == newRevision && isCalcTerminalStatus(existing.CalcStatus) {
		line.CalcStatus = existing.CalcStatus
		line.CalcError = existing.CalcError
		line.CalcJSON = existing.CalcJSON
		line.CalculatedAt = existing.CalculatedAt
		line.UnitPrice = existing.UnitPrice
		line.Total = existing.Total
		return line
	}
	line.CalcJSON = nil
	line.CalcStatus = ""
	line.CalcError = ""
	line.CalculatedAt = nil
	line.UnitPrice = 0
	line.Total = 0
	_ = estimate
	return line
}

type storedEstimateLineCalc struct {
	Revision     int64
	CalcStatus   string
	CalcError    string
	CalcJSON     json.RawMessage
	UnitPrice    float64
	Total        float64
	CalculatedAt *time.Time
}

func loadStoredEstimateLines(ctx context.Context, tx pgx.Tx, estimateID string) (map[string]storedEstimateLineCalc, error) {
	rows, err := tx.Query(ctx, `
SELECT id, revision, calc_status, calc_error, calc_json, unit_price, total, calculated_at
FROM app_estimate_lines
WHERE estimate_id = $1`, estimateID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	lines := make(map[string]storedEstimateLineCalc)
	for rows.Next() {
		var line storedEstimateLineCalc
		var id string
		if err := rows.Scan(&id, &line.Revision, &line.CalcStatus, &line.CalcError, &line.CalcJSON, &line.UnitPrice, &line.Total, &line.CalculatedAt); err != nil {
			return nil, err
		}
		lines[id] = line
	}
	return lines, rows.Err()
}

func enqueueEstimateLineCalcJobTx(ctx context.Context, tx pgx.Tx, estimate domain.Estimate, line domain.EstimateItem, queueMode string) error {
	code := estimateRecordCode(line)
	if _, err := tx.Exec(ctx, `
DELETE FROM calc_message_receipts
WHERE estimate_id = $1 AND line_id = $2
`, estimate.ID, line.ID); err != nil {
		return err
	}
	jobID := newID("calcjob")
	payload, err := json.Marshal(map[string]any{
		"code":       code,
		"fgisSetId":  estimate.FgisSetID,
		"district":   estimate.District,
		"quantity":   line.Quantity,
		"rawText":    line.RawText,
		"generation": estimate.CalcGeneration,
	})
	if err != nil {
		return err
	}
	if queueMode != "rabbit" {
		_, err = tx.Exec(ctx, `
INSERT INTO estimate_calc_jobs (id, company_id, estimate_id, line_id, revision, status, priority, payload)
VALUES ($1, $2, $3, $4, $5, 'queued', 10, $6)
ON CONFLICT (estimate_id, line_id, revision) DO UPDATE
SET status = CASE
        WHEN estimate_calc_jobs.status = 'leased' THEN estimate_calc_jobs.status
        ELSE 'queued'
    END,
    payload = EXCLUDED.payload,
    run_after = now(),
    last_error = '',
    updated_at = now()
`, jobID, estimate.CompanyID, estimate.ID, line.ID, line.Revision, payload)
		if err != nil {
			return err
		}
	}
	if queueMode != "dual" && queueMode != "rabbit" {
		return nil
	}
	requestID := requestctx.RequestID(ctx)
	traceparent := requestctx.TraceParent(ctx)
	eventPayload, err := json.Marshal(map[string]any{
		"messageVersion": 1,
		"jobId":          jobID,
		"companyId":      estimate.CompanyID,
		"estimateId":     estimate.ID,
		"lineId":         line.ID,
		"revision":       line.Revision,
		"generation":     estimate.CalcGeneration,
		"code":           code,
		"fgisSetId":      estimate.FgisSetID,
		"district":       estimate.District,
		"quantity":       line.Quantity,
		"rawText":        line.RawText,
		"requestId":      requestID,
		"traceparent":    traceparent,
		"attempt":        1,
		"maxAttempts":    5,
		"createdAt":      time.Now().UTC(),
	})
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
INSERT INTO outbox_events (id, topic, routing_key, payload)
VALUES ($1, $2, $3, $4)
`, newID("outbox"), "estimate.calc", "estimate.calc", eventPayload)
	return err
}

type OutboxEvent struct {
	ID         string
	RoutingKey string
	Payload    []byte
}

func (s *FileStore) FetchPendingOutboxEvents(ctx context.Context, limit int) ([]OutboxEvent, error) {
	if s.treeDB == nil {
		return []OutboxEvent{}, nil
	}
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.treeDB.Query(ctx, `
SELECT id, routing_key, payload::text
FROM outbox_events
WHERE published_at IS NULL
ORDER BY created_at
LIMIT $1
`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]OutboxEvent, 0, limit)
	for rows.Next() {
		var item OutboxEvent
		var payload string
		if err := rows.Scan(&item.ID, &item.RoutingKey, &payload); err != nil {
			return nil, err
		}
		item.Payload = []byte(payload)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *FileStore) ShouldSkipCalcDelivery(ctx context.Context, job EstimateCalcJob) (bool, string) {
	if s.treeDB == nil {
		return false, ""
	}
	var receiptCount int
	if err := s.treeDB.QueryRow(ctx, `
SELECT count(*)
FROM calc_message_receipts
WHERE estimate_id = $1 AND line_id = $2 AND revision = $3
`, job.EstimateID, job.LineID, job.Revision).Scan(&receiptCount); err != nil {
		return false, ""
	}
	var lineRevision int64
	var lineStatus string
	err := s.treeDB.QueryRow(ctx, `
SELECT revision, calc_status
FROM app_estimate_lines
WHERE estimate_id = $1 AND id = $2
`, job.EstimateID, job.LineID).Scan(&lineRevision, &lineStatus)
	if err != nil {
		return calcDeliverySkipReason(receiptCount, 0, "", job.Revision)
	}
	return calcDeliverySkipReason(receiptCount, lineRevision, lineStatus, job.Revision)
}

func calcDeliverySkipReason(receiptCount int, lineRevision int64, lineStatus string, jobRevision int64) (bool, string) {
	if lineRevision != jobRevision {
		return true, "stale line revision"
	}
	if strings.TrimSpace(lineStatus) == "done" {
		return true, "line already done"
	}
	return false, ""
}

func (s *FileStore) CountPendingOutboxEvents(ctx context.Context) (int64, error) {
	if s.treeDB == nil {
		return 0, nil
	}
	var count int64
	err := s.treeDB.QueryRow(ctx, `
SELECT count(*)
FROM outbox_events
WHERE published_at IS NULL
`).Scan(&count)
	return count, err
}

type QueueStatsHistoryPoint struct {
	At            time.Time `json:"at"`
	DLQ           int       `json:"dlq"`
	Main          int       `json:"main"`
	OutboxPending int64     `json:"outboxPending"`
}

const (
	queueStatsHistoryKey   = "queue_stats_history"
	maxQueueStatsHistory   = 120
	queueStatsMinInterval  = time.Minute
)

func (s *FileStore) RecordQueueStatsSample(ctx context.Context, dlq, main int, outboxPending int64) ([]QueueStatsHistoryPoint, error) {
	if s.treeDB == nil {
		return []QueueStatsHistoryPoint{}, nil
	}
	history, err := s.loadQueueStatsHistory(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if len(history) > 0 {
		last := history[len(history)-1]
		if now.Sub(last.At) < queueStatsMinInterval {
			return history, nil
		}
	}
	history = append(history, QueueStatsHistoryPoint{
		At:            now,
		DLQ:           dlq,
		Main:          main,
		OutboxPending: outboxPending,
	})
	if len(history) > maxQueueStatsHistory {
		history = history[len(history)-maxQueueStatsHistory:]
	}
	if err := s.saveQueueStatsHistory(ctx, history); err != nil {
		return history, err
	}
	return history, nil
}

func (s *FileStore) QueueDLQDelta(ctx context.Context, window time.Duration, currentDLQ int) (int, bool) {
	if s.treeDB == nil || window <= 0 {
		return 0, false
	}
	history, err := s.loadQueueStatsHistory(ctx)
	if err != nil || len(history) == 0 {
		return 0, false
	}
	cutoff := time.Now().UTC().Add(-window)
	var baseline *QueueStatsHistoryPoint
	for i := range history {
		point := history[i]
		if point.At.After(cutoff) {
			break
		}
		baseline = &history[i]
	}
	if baseline == nil {
		baseline = &history[0]
	}
	return currentDLQ - baseline.DLQ, true
}

func (s *FileStore) loadQueueStatsHistory(ctx context.Context) ([]QueueStatsHistoryPoint, error) {
	var raw string
	err := s.treeDB.QueryRow(ctx, `SELECT value FROM app_settings WHERE key = $1`, queueStatsHistoryKey).Scan(&raw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return []QueueStatsHistoryPoint{}, nil
		}
		return nil, err
	}
	var history []QueueStatsHistoryPoint
	if err := json.Unmarshal([]byte(raw), &history); err != nil {
		return []QueueStatsHistoryPoint{}, nil
	}
	return history, nil
}

func (s *FileStore) saveQueueStatsHistory(ctx context.Context, history []QueueStatsHistoryPoint) error {
	payload, err := json.Marshal(history)
	if err != nil {
		return err
	}
	_, err = s.treeDB.Exec(ctx, `
INSERT INTO app_settings (key, value, updated_at)
VALUES ($1, $2, now())
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()
`, queueStatsHistoryKey, string(payload))
	return err
}

const queueConsumerStatsKey = "rabbit_consumer_stats"

type QueueConsumerStats struct {
	Processed  int64     `json:"processed"`
	Retried    int64     `json:"retried"`
	Dead       int64     `json:"dead"`
	Failed     int64     `json:"failed"`
	Duplicates int64     `json:"duplicates"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

func (s *FileStore) TouchQueueConsumerHeartbeat(ctx context.Context) error {
	if s.treeDB == nil {
		return nil
	}
	_, err := s.treeDB.Exec(ctx, `
INSERT INTO app_settings (key, value, updated_at)
VALUES ('rabbit_consumer_heartbeat', $1, now())
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()
`, strconv.FormatInt(time.Now().UTC().Unix(), 10))
	return err
}

func (s *FileStore) SaveQueueConsumerStats(ctx context.Context, stats QueueConsumerStats) error {
	if s.treeDB == nil {
		return nil
	}
	stats.UpdatedAt = time.Now().UTC()
	payload, err := json.Marshal(stats)
	if err != nil {
		return err
	}
	_, err = s.treeDB.Exec(ctx, `
INSERT INTO app_settings (key, value, updated_at)
VALUES ($1, $2, now())
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()
`, queueConsumerStatsKey, string(payload))
	return err
}

func (s *FileStore) LoadQueueConsumerStats(ctx context.Context) (QueueConsumerStats, bool) {
	if s.treeDB == nil {
		return QueueConsumerStats{}, false
	}
	var raw string
	err := s.treeDB.QueryRow(ctx, `
SELECT value FROM app_settings WHERE key = $1
`, queueConsumerStatsKey).Scan(&raw)
	if err != nil {
		return QueueConsumerStats{}, false
	}
	var stats QueueConsumerStats
	if err := json.Unmarshal([]byte(raw), &stats); err != nil {
		return QueueConsumerStats{}, false
	}
	return stats, true
}

// ReconcileCalcLineFromReceipt marks a line done when a calc receipt already exists
// but calc_status was reset (for example after re-saving the estimate).
func (s *FileStore) ReconcileCalcLineFromReceipt(ctx context.Context, job EstimateCalcJob) error {
	if s.treeDB == nil {
		return nil
	}
	_, err := s.treeDB.Exec(ctx, `
UPDATE app_estimate_lines l
SET calc_status = 'done',
    calc_error = '',
    calculated_at = COALESCE(l.calculated_at, now())
FROM calc_message_receipts r
WHERE l.estimate_id = $1
  AND l.id = $2
  AND l.revision = $3
  AND r.estimate_id = l.estimate_id
  AND r.line_id = l.id
  AND r.revision = l.revision
  AND l.calc_status <> 'done'
  AND l.calc_json IS NOT NULL
  AND l.total > 0
`, job.EstimateID, job.LineID, job.Revision)
	return err
}

func (s *FileStore) QueueConsumerHeartbeatAge(ctx context.Context) (time.Duration, bool) {
	if s.treeDB == nil {
		return 0, false
	}
	var value string
	err := s.treeDB.QueryRow(ctx, `
SELECT value FROM app_settings WHERE key = 'rabbit_consumer_heartbeat'
`).Scan(&value)
	if err != nil {
		return 0, false
	}
	ts, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || ts <= 0 {
		return 0, false
	}
	beat := time.Unix(ts, 0).UTC()
	return time.Since(beat), true
}

func (s *FileStore) MarkOutboxEventPublished(ctx context.Context, id string) error {
	if s.treeDB == nil {
		return nil
	}
	_, err := s.treeDB.Exec(ctx, `
UPDATE outbox_events
SET published_at = now(),
    last_error = ''
WHERE id = $1
`, id)
	return err
}

func (s *FileStore) MarkOutboxEventFailed(ctx context.Context, id string, cause error) error {
	if s.treeDB == nil {
		return nil
	}
	message := ""
	if cause != nil {
		message = cause.Error()
	}
	_, err := s.treeDB.Exec(ctx, `
UPDATE outbox_events
SET attempts = attempts + 1,
    last_error = $2
WHERE id = $1
`, id, message)
	return err
}

func shouldEnqueueEstimateLineCalc(line domain.EstimateItem) bool {
	return estimateLineType(line.Type) == string(domain.EstimateLinePosition) &&
		estimateItemSource(line.Source) == "gsn" &&
		estimateRecordCode(line) != ""
}

func estimateRecordCode(line domain.EstimateItem) string {
	if strings.TrimSpace(line.Code) != "" {
		return extractSourceDataPositionCipher(line.Code)
	}
	return extractSourceDataPositionCipher(line.OriginalCode)
}

func extractSourceDataPositionCipher(firstField string) string {
	raw := strings.TrimSpace(firstField)
	if raw == "" {
		return ""
	}
	cutAt := len(raw)
	for _, sep := range []string{"(", " ", "#"} {
		if i := strings.Index(raw, sep); i >= 0 && i < cutAt {
			cutAt = i
		}
	}
	return strings.TrimSpace(raw[:cutAt])
}

func ensureUniqueEstimateLineIDs(items []domain.EstimateItem) []domain.EstimateItem {
	if len(items) == 0 {
		return items
	}
	seen := map[string]struct{}{}
	out := make([]domain.EstimateItem, len(items))
	for i, item := range items {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			id = newID("itm")
		}
		if _, exists := seen[id]; exists {
			id = newID("itm")
		}
		seen[id] = struct{}{}
		item.ID = id
		out[i] = item
	}
	return out
}

func estimateLineRevision(estimate domain.Estimate, line domain.EstimateItem) int64 {
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(estimate.ID))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(line.ID))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(estimateRecordCode(line)))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(strconv.FormatFloat(line.Quantity, 'f', -1, 64)))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(estimate.FgisSetID))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(estimate.District))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(strconv.FormatInt(estimate.CalcGeneration, 10)))
	value := int64(hash.Sum64() & 0x7fffffffffffffff)
	if value == 0 {
		return 1
	}
	return value
}

func nullableJSON(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	return raw
}

func estimateLineType(v string) string {
	value := strings.TrimSpace(strings.ToLower(v))
	if value == "" {
		return string(domain.EstimateLinePosition)
	}
	return value
}

func estimateItemSource(v string) string {
	return strings.TrimSpace(strings.ToLower(v))
}

func isGSNEstimateSource(source string) bool {
	return estimateItemSource(source) == "gsn"
}

func normalizeEstimateItem(item domain.EstimateItem) (domain.EstimateItem, float64, error) {
	item.Type = estimateLineType(item.Type)
	item.Source = estimateItemSource(item.Source)
	item.Code = strings.TrimSpace(item.Code)
	item.OriginalCode = strings.TrimSpace(item.OriginalCode)
	item.Name = strings.TrimSpace(item.Name)
	item.Unit = strings.TrimSpace(item.Unit)
	if !validEstimateLineType(item.Type) {
		return domain.EstimateItem{}, 0, ErrConflict
	}
	if item.ID == "" {
		item.ID = newID("itm")
	}

	if isGSNEstimateSource(item.Source) {
		item.Code = extractSourceDataPositionCipher(item.Code)
		if item.Code == "" {
			return domain.EstimateItem{}, 0, ErrConflict
		}
		if quantityRaw := quantityRawFromSourceDataLine(item.RawText); quantityRaw != "" {
			quantity, err := ParseSourceDataQuantity(quantityRaw)
			if err != nil {
				return domain.EstimateItem{}, 0, err
			}
			item.Quantity = quantity
		}
		item.OriginalCode = ""
		item.Name = nameFromSourceDataLine(item.RawText)
		item.Unit = unitFromSourceDataLine(item.RawText)
		item.UnitPrice = 0
		item.Total = 0
		item.CalcJSON = nil
		item.CalcStatus = ""
		item.CalcError = ""
		item.CalculatedAt = nil
		return item, 0, nil
	}

	if strings.TrimSpace(item.RawText) != "" && item.Type == string(domain.EstimateLinePosition) {
		if quantityRaw := quantityRawFromSourceDataLine(item.RawText); quantityRaw != "" {
			quantity, err := ParseSourceDataQuantity(quantityRaw)
			if err != nil {
				return domain.EstimateItem{}, 0, err
			}
			item.Quantity = quantity
		}
	}

	if item.Name == "" {
		return domain.EstimateItem{}, 0, ErrConflict
	}
	item.Total = item.Quantity * item.UnitPrice
	return item, item.Total, nil
}

func newID(prefix string) string {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return prefix + "_" + hex.EncodeToString([]byte(time.Now().Format("20060102150405.000000000")))
	}

	return prefix + "_" + hex.EncodeToString(bytes)
}

func validEstimateStatus(status domain.EstimateStatus) bool {
	switch status {
	case domain.EstimateDraft, domain.EstimateApproved, domain.EstimateArchived:
		return true
	default:
		return false
	}
}

func validEstimateLineType(lineType string) bool {
	switch lineType {
	case string(domain.EstimateLineSection), string(domain.EstimateLineSubsection), string(domain.EstimateLinePosition):
		return true
	default:
		return false
	}
}

