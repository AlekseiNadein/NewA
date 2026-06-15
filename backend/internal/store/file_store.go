package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"nav-saas-mvp/backend/internal/auth"
	"nav-saas-mvp/backend/internal/domain"
)

var (
	ErrNotFound  = errors.New("not found")
	ErrConflict  = errors.New("conflict")
	ErrForbidden = errors.New("forbidden")
)

type FileStore struct {
	mu        sync.RWMutex
	path      string
	companies map[string]domain.Company
	users     map[string]domain.User
	estimates map[string]domain.Estimate
}

type snapshot struct {
	Companies []domain.Company  `json:"companies"`
	Users     []domain.User     `json:"users"`
	Estimates []domain.Estimate `json:"estimates"`
}

type NewUser struct {
	CompanyID string      `json:"companyId"`
	Email     string      `json:"email"`
	Name      string      `json:"name"`
	Role      domain.Role `json:"role"`
	Password  string      `json:"password"`
}

type EstimateInput struct {
	Title       string                `json:"title"`
	Description string                `json:"description"`
	Status      domain.EstimateStatus `json:"status"`
	Items       []domain.EstimateItem `json:"items"`
}

func NewFileStore(path string) (*FileStore, error) {
	store := &FileStore{
		path:      path,
		companies: map[string]domain.Company{},
		users:     map[string]domain.User{},
		estimates: map[string]domain.Estimate{},
	}

	if err := store.load(); err != nil {
		return nil, err
	}

	if len(store.users) == 0 {
		if err := store.seed(); err != nil {
			return nil, err
		}
	}

	return store, nil
}

func (s *FileStore) FindUserByEmail(email string) (domain.User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	normalized := strings.ToLower(strings.TrimSpace(email))
	for _, user := range s.users {
		if user.Email == normalized {
			return user, true
		}
	}

	return domain.User{}, false
}

func (s *FileStore) FindUserByID(id string) (domain.User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, ok := s.users[id]
	return user, ok
}

func (s *FileStore) ListCompanies() []domain.Company {
	s.mu.RLock()
	defer s.mu.RUnlock()

	companies := make([]domain.Company, 0, len(s.companies))
	for _, company := range s.companies {
		companies = append(companies, company)
	}

	sort.Slice(companies, func(i, j int) bool {
		return companies[i].Name < companies[j].Name
	})

	return companies
}

func (s *FileStore) CreateCompany(name string) (domain.Company, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	name = strings.TrimSpace(name)
	if name == "" {
		return domain.Company{}, ErrConflict
	}

	for _, company := range s.companies {
		if strings.EqualFold(company.Name, name) {
			return domain.Company{}, ErrConflict
		}
	}

	now := time.Now().UTC()
	company := domain.Company{
		ID:        newID("cmp"),
		Name:      name,
		CreatedAt: now,
		UpdatedAt: now,
	}

	s.companies[company.ID] = company
	return company, s.saveLocked()
}

func (s *FileStore) ListUsers(companyID string, includeAll bool) []domain.User {
	s.mu.RLock()
	defer s.mu.RUnlock()

	users := make([]domain.User, 0, len(s.users))
	for _, user := range s.users {
		if includeAll || user.CompanyID == companyID {
			users = append(users, user)
		}
	}

	sort.Slice(users, func(i, j int) bool {
		return users[i].Email < users[j].Email
	})

	return users
}

func (s *FileStore) CreateUser(input NewUser) (domain.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	input.Name = strings.TrimSpace(input.Name)
	input.Password = strings.TrimSpace(input.Password)

	if input.Email == "" || input.Name == "" || input.CompanyID == "" || input.Password == "" {
		return domain.User{}, ErrConflict
	}

	if _, ok := s.companies[input.CompanyID]; !ok {
		return domain.User{}, ErrNotFound
	}

	for _, user := range s.users {
		if user.Email == input.Email {
			return domain.User{}, ErrConflict
		}
	}

	if input.Role == "" {
		input.Role = domain.RoleUser
	}
	if !validRole(input.Role) {
		return domain.User{}, ErrConflict
	}

	hash, salt, err := auth.NewPassword(input.Password)
	if err != nil {
		return domain.User{}, err
	}

	now := time.Now().UTC()
	user := domain.User{
		ID:           newID("usr"),
		CompanyID:    input.CompanyID,
		Email:        input.Email,
		Name:         input.Name,
		Role:         input.Role,
		PasswordHash: hash,
		PasswordSalt: salt,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	s.users[user.ID] = user
	return user, s.saveLocked()
}

func (s *FileStore) ListEstimates(companyID string, includeAll bool) []domain.Estimate {
	s.mu.RLock()
	defer s.mu.RUnlock()

	estimates := make([]domain.Estimate, 0, len(s.estimates))
	for _, estimate := range s.estimates {
		if includeAll || estimate.CompanyID == companyID {
			estimates = append(estimates, estimate)
		}
	}

	sort.Slice(estimates, func(i, j int) bool {
		return estimates[i].UpdatedAt.After(estimates[j].UpdatedAt)
	})

	return estimates
}

func (s *FileStore) CreateEstimate(companyID string, input EstimateInput) (domain.Estimate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.companies[companyID]; !ok {
		return domain.Estimate{}, ErrNotFound
	}

	estimate, err := buildEstimate(newID("est"), companyID, input, time.Now().UTC(), time.Now().UTC())
	if err != nil {
		return domain.Estimate{}, err
	}

	s.estimates[estimate.ID] = estimate
	return estimate, s.saveLocked()
}

func (s *FileStore) UpdateEstimate(id string, companyID string, includeAll bool, input EstimateInput) (domain.Estimate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, ok := s.estimates[id]
	if !ok {
		return domain.Estimate{}, ErrNotFound
	}
	if !includeAll && current.CompanyID != companyID {
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

	for _, company := range snap.Companies {
		s.companies[company.ID] = company
	}
	for _, user := range snap.Users {
		s.users[user.ID] = user
	}
	for _, estimate := range snap.Estimates {
		s.estimates[estimate.ID] = estimate
	}

	return nil
}

func (s *FileStore) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}

	snap := snapshot{
		Companies: make([]domain.Company, 0, len(s.companies)),
		Users:     make([]domain.User, 0, len(s.users)),
		Estimates: make([]domain.Estimate, 0, len(s.estimates)),
	}

	for _, company := range s.companies {
		snap.Companies = append(snap.Companies, company)
	}
	for _, user := range s.users {
		snap.Users = append(snap.Users, user)
	}
	for _, estimate := range s.estimates {
		snap.Estimates = append(snap.Estimates, estimate)
	}

	sort.Slice(snap.Companies, func(i, j int) bool {
		return snap.Companies[i].Name < snap.Companies[j].Name
	})
	sort.Slice(snap.Users, func(i, j int) bool {
		return snap.Users[i].Email < snap.Users[j].Email
	})
	sort.Slice(snap.Estimates, func(i, j int) bool {
		return snap.Estimates[i].UpdatedAt.After(snap.Estimates[j].UpdatedAt)
	})

	content, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.path, content, 0o600)
}

func (s *FileStore) seed() error {
	now := time.Now().UTC()
	company := domain.Company{
		ID:        newID("cmp"),
		Name:      "Demo Company",
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.companies[company.ID] = company

	for _, input := range []NewUser{
		{CompanyID: company.ID, Email: "admin@example.com", Name: "Platform Admin", Role: domain.RoleSuperAdmin, Password: "admin123"},
		{CompanyID: company.ID, Email: "manager@example.com", Name: "Company Manager", Role: domain.RoleCompanyAdmin, Password: "manager123"},
		{CompanyID: company.ID, Email: "user@example.com", Name: "Estimate User", Role: domain.RoleUser, Password: "user123"},
	} {
		hash, salt, err := auth.NewPassword(input.Password)
		if err != nil {
			return err
		}

		user := domain.User{
			ID:           newID("usr"),
			CompanyID:    input.CompanyID,
			Email:        input.Email,
			Name:         input.Name,
			Role:         input.Role,
			PasswordHash: hash,
			PasswordSalt: salt,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		s.users[user.ID] = user
	}

	estimate, err := buildEstimate(newID("est"), company.ID, EstimateInput{
		Title:       "Demo estimate",
		Description: "Small estimate module for the first SaaS MVP slice.",
		Status:      domain.EstimateDraft,
		Items: []domain.EstimateItem{
			{Name: "Concrete works", Quantity: 12, Unit: "m3", UnitPrice: 1500},
			{Name: "Labor", Quantity: 24, Unit: "h", UnitPrice: 900},
		},
	}, now, now)
	if err != nil {
		return err
	}
	s.estimates[estimate.ID] = estimate

	return s.saveLocked()
}

func buildEstimate(id string, companyID string, input EstimateInput, createdAt time.Time, updatedAt time.Time) (domain.Estimate, error) {
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)
	if input.Title == "" {
		return domain.Estimate{}, ErrConflict
	}
	if input.Status == "" {
		input.Status = domain.EstimateDraft
	}
	if !validEstimateStatus(input.Status) {
		return domain.Estimate{}, ErrConflict
	}

	items := make([]domain.EstimateItem, 0, len(input.Items))
	var total float64
	for _, item := range input.Items {
		item.Name = strings.TrimSpace(item.Name)
		item.Unit = strings.TrimSpace(item.Unit)
		if item.Name == "" {
			return domain.Estimate{}, ErrConflict
		}
		if item.ID == "" {
			item.ID = newID("itm")
		}
		item.Total = item.Quantity * item.UnitPrice
		total += item.Total
		items = append(items, item)
	}

	return domain.Estimate{
		ID:          id,
		CompanyID:   companyID,
		Title:       input.Title,
		Description: input.Description,
		Status:      input.Status,
		Items:       items,
		Total:       total,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
	}, nil
}

func newID(prefix string) string {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return prefix + "_" + hex.EncodeToString([]byte(time.Now().Format("20060102150405.000000000")))
	}

	return prefix + "_" + hex.EncodeToString(bytes)
}

func validRole(role domain.Role) bool {
	switch role {
	case domain.RoleSuperAdmin, domain.RoleCompanyAdmin, domain.RoleUser:
		return true
	default:
		return false
	}
}

func validEstimateStatus(status domain.EstimateStatus) bool {
	switch status {
	case domain.EstimateDraft, domain.EstimateApproved, domain.EstimateArchived:
		return true
	default:
		return false
	}
}
