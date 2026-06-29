package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash/fnv"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"nav-saas-mvp/backend/internal/auth"
	"nav-saas-mvp/backend/internal/domain"
)

var (
	ErrNotFound  = errors.New("not found")
	ErrConflict  = errors.New("conflict")
	ErrForbidden = errors.New("forbidden")
)

type FileStore struct {
	mu            sync.RWMutex
	path          string
	treeDB        *pgxpool.Pool
	companies     map[string]domain.Company
	users         map[string]domain.User
	constructions map[string]domain.Construction
	objects       map[string]domain.ConstructionObject
	estimates     map[string]domain.Estimate
	licenses      map[string]map[string]int
	settings      domain.AppSettings
}

type snapshot struct {
	Companies     []domain.Company            `json:"companies"`
	Users         []storedUser                `json:"users"`
	Constructions []domain.Construction       `json:"constructions"`
	Objects       []domain.ConstructionObject `json:"objects"`
	Estimates     []domain.Estimate           `json:"estimates"`
	Licenses      []storedCompanyLicenses     `json:"licenses,omitempty"`
	Settings      domain.AppSettings          `json:"settings,omitempty"`
}

type storedCompanyLicenses struct {
	CompanyID string         `json:"companyId"`
	Items     map[string]int `json:"items"`
}

type storedUser struct {
	ID                   string    `json:"id"`
	CompanyID            string    `json:"companyId"`
	Email                string    `json:"email"`
	Name                 string    `json:"name"`
	Role                 string    `json:"role,omitempty"`
	Authorized           bool      `json:"authorized"`
	IsAdministrator      bool      `json:"isAdministrator"`
	IsSuperAdministrator bool      `json:"isSuperAdministrator"`
	PasswordHash         string    `json:"passwordHash"`
	PasswordSalt         string    `json:"passwordSalt"`
	CreatedAt            time.Time `json:"createdAt"`
	UpdatedAt            time.Time `json:"updatedAt"`
}

type NewUser struct {
	CompanyID            string `json:"companyId"`
	CompanyName          string `json:"companyName"`
	Email                string `json:"email"`
	Name                 string `json:"name"`
	Password             string `json:"password"`
	Authorized           bool   `json:"authorized"`
	IsAdministrator      bool   `json:"isAdministrator"`
	IsSuperAdministrator bool   `json:"isSuperAdministrator"`
}

type RegisterUser struct {
	CompanyName     string `json:"companyName"`
	Name            string `json:"name"`
	Email           string `json:"email"`
	Password        string `json:"password"`
	PasswordConfirm string `json:"passwordConfirm"`
}

type UpdateUser struct {
	CompanyID            string `json:"companyId"`
	Email                string `json:"email"`
	Name                 string `json:"name"`
	Password             string `json:"password"`
	Authorized           bool   `json:"authorized"`
	IsAdministrator      bool   `json:"isAdministrator"`
	IsSuperAdministrator bool   `json:"isSuperAdministrator"`
}

type UserView struct {
	domain.User
	CompanyName string `json:"companyName"`
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
	store := &FileStore{
		path:          path,
		companies:     map[string]domain.Company{},
		users:         map[string]domain.User{},
		constructions: map[string]domain.Construction{},
		objects:       map[string]domain.ConstructionObject{},
		estimates:     map[string]domain.Estimate{},
		licenses:      map[string]map[string]int{},
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

	if len(store.users) == 0 {
		if err := store.seed(); err != nil {
			return nil, err
		}
	} else {
		changed, err := store.ensureDemoCredentials()
		if err != nil {
			return nil, err
		}
		flagsChanged, err := store.ensureUserFlags()
		if err != nil {
			return nil, err
		}
		if changed || flagsChanged {
			if err := store.save(); err != nil {
				return nil, err
			}
		} else if len(store.constructions) == 0 {
			if err := store.ensureDemoStructure(); err != nil {
				return nil, err
			}
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

func (s *FileStore) FindUserForLogin(companyName, login string) (domain.User, bool) {
	login = strings.TrimSpace(login)
	if login == "" {
		return domain.User{}, false
	}

	normalizedEmail := strings.ToLower(login)
	if strings.Contains(normalizedEmail, "@") {
		if user, ok := s.FindUserByEmail(normalizedEmail); ok {
			return user, true
		}
	}

	return s.FindUserByCompanyAndName(companyName, login)
}

func (s *FileStore) FindUserByCompanyAndName(companyName, userName string) (domain.User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	company, ok := s.findCompanyByNameLocked(companyName)
	if !ok {
		return domain.User{}, false
	}

	normalizedName := normalizePersonName(userName)
	normalizedEmail := strings.ToLower(strings.TrimSpace(userName))
	loginByEmail := strings.Contains(normalizedEmail, "@")
	for _, user := range s.users {
		if user.CompanyID != company.ID {
			continue
		}
		if normalizePersonName(user.Name) == normalizedName {
			return user, true
		}
		if loginByEmail && user.Email == normalizedEmail {
			return user, true
		}
	}

	return domain.User{}, false
}

func (s *FileStore) FindOrCreateCompanyByName(name string) (domain.Company, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	name = strings.TrimSpace(name)
	if name == "" {
		return domain.Company{}, ErrConflict
	}

	if company, ok := s.findCompanyByNameLocked(name); ok {
		return company, nil
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

func (s *FileStore) FindCompanyByName(name string) (domain.Company, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.findCompanyByNameLocked(name)
}

func migrateStoredUser(user storedUser) storedUser {
	if user.Role != "" && !user.Authorized && !user.IsAdministrator && !user.IsSuperAdministrator {
		switch domain.Role(user.Role) {
		case domain.RoleSuperAdmin:
			user.IsSuperAdministrator = true
			user.IsAdministrator = true
			user.Authorized = true
		case domain.RoleCompanyAdmin:
			user.IsAdministrator = true
			user.Authorized = true
		case domain.RoleUser:
			user.Authorized = true
		}
	}

	switch user.Email {
	case "admin@example.com":
		user.Name = "Суперадминистратор"
		user.IsSuperAdministrator = true
		user.IsAdministrator = true
		user.Authorized = true
	case "manager@example.com":
		if user.Name == "Company Manager" {
			user.Name = "Администратор компании"
		}
		user.IsAdministrator = true
		user.Authorized = true
	case "user@example.com":
		if user.Name == "Estimate User" {
			user.Name = "Пользователь"
		}
		user.Authorized = true
	}

	return user
}

func (s *FileStore) findCompanyByNameLocked(name string) (domain.Company, bool) {
	normalized := normalizeCompanyName(name)
	for _, company := range s.companies {
		if normalizeCompanyName(company.Name) == normalized {
			return company, true
		}
	}
	return domain.Company{}, false
}

func (s *FileStore) FindUserByID(id string) (domain.User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, ok := s.users[id]
	return user, ok
}

func (u storedUser) toDomain() domain.User {
	user := domain.User{
		ID:                   u.ID,
		CompanyID:            u.CompanyID,
		Email:                u.Email,
		Name:                 u.Name,
		Authorized:           u.Authorized,
		IsAdministrator:      u.IsAdministrator,
		IsSuperAdministrator: u.IsSuperAdministrator,
		PasswordHash:         u.PasswordHash,
		PasswordSalt:         u.PasswordSalt,
		CreatedAt:            u.CreatedAt,
		UpdatedAt:            u.UpdatedAt,
	}
	if u.Role != "" && !user.IsSuperAdministrator && !user.IsAdministrator && !user.Authorized {
		switch domain.Role(u.Role) {
		case domain.RoleSuperAdmin:
			user.IsSuperAdministrator = true
			user.IsAdministrator = true
			user.Authorized = true
		case domain.RoleCompanyAdmin:
			user.IsAdministrator = true
			user.Authorized = true
		case domain.RoleUser:
			user.Authorized = true
		}
	}
	return user
}

func migrateStoredCompany(company domain.Company) domain.Company {
	if company.Name == "Demo Company" {
		company.Name = "Система"
	}
	return company
}

func userToStored(user domain.User) storedUser {
	return storedUser{
		ID:                   user.ID,
		CompanyID:            user.CompanyID,
		Email:                user.Email,
		Name:                 user.Name,
		Authorized:           user.Authorized,
		IsAdministrator:      user.IsAdministrator,
		IsSuperAdministrator: user.IsSuperAdministrator,
		PasswordHash:         user.PasswordHash,
		PasswordSalt:         user.PasswordSalt,
		CreatedAt:            user.CreatedAt,
		UpdatedAt:            user.UpdatedAt,
	}
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

func (s *FileStore) UserView(user domain.User) UserView {
	s.mu.RLock()
	defer s.mu.RUnlock()

	company := s.companies[user.CompanyID]
	return UserView{
		User:        user,
		CompanyName: company.Name,
	}
}

func (s *FileStore) ListUsers(companyID string, includeAll bool) []UserView {
	s.mu.RLock()
	defer s.mu.RUnlock()

	users := make([]UserView, 0, len(s.users))
	for _, user := range s.users {
		if includeAll || user.CompanyID == companyID {
			company := s.companies[user.CompanyID]
			users = append(users, UserView{
				User:        user,
				CompanyName: company.Name,
			})
		}
	}

	sort.Slice(users, func(i, j int) bool {
		if users[i].CompanyName != users[j].CompanyName {
			return users[i].CompanyName < users[j].CompanyName
		}
		return users[i].Name < users[j].Name
	})

	return users
}

func (s *FileStore) CreateUser(input NewUser) (domain.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	input.Name = strings.TrimSpace(input.Name)
	input.Password = strings.TrimSpace(input.Password)
	input.CompanyName = strings.TrimSpace(input.CompanyName)

	if input.Email == "" || input.Name == "" || input.Password == "" {
		return domain.User{}, ErrConflict
	}

	if input.CompanyID == "" && input.CompanyName != "" {
		company, ok := s.findCompanyByNameLocked(input.CompanyName)
		if !ok {
			now := time.Now().UTC()
			company = domain.Company{
				ID:        newID("cmp"),
				Name:      input.CompanyName,
				CreatedAt: now,
				UpdatedAt: now,
			}
			s.companies[company.ID] = company
		}
		input.CompanyID = company.ID
	}

	if input.CompanyID == "" {
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

	hash, salt, err := auth.NewPassword(input.Password)
	if err != nil {
		return domain.User{}, err
	}

	now := time.Now().UTC()
	user := domain.User{
		ID:                   newID("usr"),
		CompanyID:            input.CompanyID,
		Email:                input.Email,
		Name:                 input.Name,
		Authorized:           input.Authorized,
		IsAdministrator:      input.IsAdministrator,
		IsSuperAdministrator: input.IsSuperAdministrator,
		PasswordHash:         hash,
		PasswordSalt:         salt,
		CreatedAt:            now,
		UpdatedAt:            now,
	}

	s.users[user.ID] = user
	return user, s.saveLocked()
}

func (s *FileStore) RegisterUser(input RegisterUser) (domain.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	input.CompanyName = strings.TrimSpace(input.CompanyName)
	input.Name = strings.TrimSpace(input.Name)
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	input.Password = strings.TrimSpace(input.Password)
	input.PasswordConfirm = strings.TrimSpace(input.PasswordConfirm)

	if input.CompanyName == "" || input.Name == "" || input.Email == "" || input.Password == "" {
		return domain.User{}, ErrConflict
	}
	if input.Password != input.PasswordConfirm {
		return domain.User{}, ErrConflict
	}

	for _, user := range s.users {
		if user.Email == input.Email {
			return domain.User{}, ErrConflict
		}
	}

	company, ok := s.findCompanyByNameLocked(input.CompanyName)
	if !ok {
		now := time.Now().UTC()
		company = domain.Company{
			ID:        newID("cmp"),
			Name:      input.CompanyName,
			CreatedAt: now,
			UpdatedAt: now,
		}
		s.companies[company.ID] = company
	}

	hash, salt, err := auth.NewPassword(input.Password)
	if err != nil {
		return domain.User{}, err
	}

	now := time.Now().UTC()
	user := domain.User{
		ID:                   newID("usr"),
		CompanyID:            company.ID,
		Email:                input.Email,
		Name:                 input.Name,
		Authorized:           false,
		IsAdministrator:      false,
		IsSuperAdministrator: false,
		PasswordHash:         hash,
		PasswordSalt:         salt,
		CreatedAt:            now,
		UpdatedAt:            now,
	}

	s.users[user.ID] = user
	return user, s.saveLocked()
}

func (s *FileStore) UpdateUser(id string, actorCompanyID string, includeAll bool, input UpdateUser) (domain.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, ok := s.users[id]
	if !ok {
		return domain.User{}, ErrNotFound
	}
	if !includeAll && current.CompanyID != actorCompanyID {
		return domain.User{}, ErrForbidden
	}

	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	input.Name = strings.TrimSpace(input.Name)
	input.Password = strings.TrimSpace(input.Password)

	if input.Email == "" || input.Name == "" {
		return domain.User{}, ErrConflict
	}

	for _, user := range s.users {
		if user.ID != id && user.Email == input.Email {
			return domain.User{}, ErrConflict
		}
	}

	companyID := current.CompanyID
	if includeAll && strings.TrimSpace(input.CompanyID) != "" {
		if _, ok := s.companies[input.CompanyID]; !ok {
			return domain.User{}, ErrNotFound
		}
		companyID = input.CompanyID
	}

	authorized := input.Authorized
	isAdministrator := input.IsAdministrator
	isSuperAdministrator := input.IsSuperAdministrator
	if !includeAll {
		isSuperAdministrator = false
		companyID = current.CompanyID
	}

	updated := current
	updated.CompanyID = companyID
	updated.Email = input.Email
	updated.Name = input.Name
	updated.Authorized = authorized
	updated.IsAdministrator = isAdministrator
	updated.IsSuperAdministrator = isSuperAdministrator
	updated.UpdatedAt = time.Now().UTC()

	if input.Password != "" {
		hash, salt, err := auth.NewPassword(input.Password)
		if err != nil {
			return domain.User{}, err
		}
		updated.PasswordHash = hash
		updated.PasswordSalt = salt
	}

	s.users[id] = updated
	return updated, s.saveLocked()
}

func (s *FileStore) DeleteUser(id string, actorCompanyID string, includeAll bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, ok := s.users[id]
	if !ok {
		return ErrNotFound
	}
	if current.IsSuperAdministrator {
		return ErrForbidden
	}
	if !includeAll && current.CompanyID != actorCompanyID {
		return ErrForbidden
	}

	delete(s.users, id)
	return s.saveLocked()
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

	if _, ok := s.companies[companyID]; !ok {
		return domain.Construction{}, ErrNotFound
	}

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

	if _, ok := s.companies[companyID]; !ok {
		return domain.Estimate{}, ErrNotFound
	}

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

	for _, company := range snap.Companies {
		company = migrateStoredCompany(company)
		s.companies[company.ID] = company
	}
	for _, user := range snap.Users {
		user = migrateStoredUser(user)
		s.users[user.ID] = user.toDomain()
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
	for _, entry := range snap.Licenses {
		if entry.CompanyID == "" || entry.Items == nil {
			continue
		}
		s.licenses[entry.CompanyID] = normalizeLicenseItems(entry.Items)
	}
	s.settings = normalizeAppSettings(snap.Settings)

	return nil
}

func (s *FileStore) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}

	snap := snapshot{
		Companies:     make([]domain.Company, 0, len(s.companies)),
		Users:         make([]storedUser, 0, len(s.users)),
		Constructions: make([]domain.Construction, 0, len(s.constructions)),
		Objects:       make([]domain.ConstructionObject, 0, len(s.objects)),
		Estimates:     make([]domain.Estimate, 0, len(s.estimates)),
		Licenses:      make([]storedCompanyLicenses, 0, len(s.licenses)),
		Settings:      normalizeAppSettings(s.settings),
	}

	for _, company := range s.companies {
		snap.Companies = append(snap.Companies, company)
	}
	for _, user := range s.users {
		snap.Users = append(snap.Users, userToStored(user))
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
	for companyID, items := range s.licenses {
		if len(items) == 0 {
			continue
		}
		snap.Licenses = append(snap.Licenses, storedCompanyLicenses{
			CompanyID: companyID,
			Items:     items,
		})
	}

	sort.Slice(snap.Companies, func(i, j int) bool {
		return snap.Companies[i].Name < snap.Companies[j].Name
	})
	sort.Slice(snap.Users, func(i, j int) bool {
		return snap.Users[i].Email < snap.Users[j].Email
	})
	sort.Slice(snap.Constructions, func(i, j int) bool {
		return compareCodes(snap.Constructions[i].Code, snap.Constructions[j].Code)
	})
	sort.Slice(snap.Objects, func(i, j int) bool {
		return compareCodes(snap.Objects[i].Code, snap.Objects[j].Code)
	})
	sort.Slice(snap.Estimates, func(i, j int) bool {
		return compareCodes(snap.Estimates[i].Code, snap.Estimates[j].Code)
	})
	sort.Slice(snap.Licenses, func(i, j int) bool {
		return snap.Licenses[i].CompanyID < snap.Licenses[j].CompanyID
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
		Name:      "Система",
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.companies[company.ID] = company

	hash, salt, err := auth.NewPassword("admin123")
	if err != nil {
		return err
	}

	user := domain.User{
		ID:                   newID("usr"),
		CompanyID:            company.ID,
		Email:                "admin@example.com",
		Name:                 "Суперадминистратор",
		Authorized:           true,
		IsAdministrator:      true,
		IsSuperAdministrator: true,
		PasswordHash:         hash,
		PasswordSalt:         salt,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	s.users[user.ID] = user

	return s.saveLocked()
}

func (s *FileStore) ensureDemoStructure() error {
	now := time.Now().UTC()
	for _, company := range s.companies {
		construction := domain.Construction{
			ID:        newID("con"),
			CompanyID: company.ID,
			Code:      "01",
			Name:      "Демо-стройка",
			CreatedAt: now,
			UpdatedAt: now,
		}
		s.constructions[construction.ID] = construction

		object := domain.ConstructionObject{
			ID:             newID("obj"),
			CompanyID:      company.ID,
			ConstructionID: construction.ID,
			Code:           "01-01",
			Name:           "Объект 1",
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		s.objects[object.ID] = object

		for id, estimate := range s.estimates {
			if estimate.CompanyID == company.ID && estimate.ObjectID == "" {
				estimate.ObjectID = object.ID
				estimate.UpdatedAt = now
				s.estimates[id] = estimate
			}
		}
	}

	return s.saveLocked()
}

func (s *FileStore) ensureDemoCredentials() (bool, error) {
	demoUsers := map[string]string{
		"admin@example.com": "admin123",
	}

	changed := false
	for id, user := range s.users {
		if user.PasswordHash != "" && user.PasswordSalt != "" {
			continue
		}

		password, ok := demoUsers[user.Email]
		if !ok {
			password = "change-me"
		}

		hash, salt, err := auth.NewPassword(password)
		if err != nil {
			return false, err
		}

		user.PasswordHash = hash
		user.PasswordSalt = salt
		user.UpdatedAt = time.Now().UTC()
		s.users[id] = user
		changed = true
	}

	return changed, nil
}

func normalizeCompanyName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	replacements := []struct{ from, to string }{
		{"«", "\""}, {"»", "\""},
		{"“", "\""}, {"”", "\""},
		{"„", "\""}, {"‟", "\""},
	}
	for _, item := range replacements {
		name = strings.ReplaceAll(name, item.from, item.to)
	}
	return name
}

func normalizePersonName(name string) string {
	parts := strings.Fields(strings.ToLower(strings.TrimSpace(name)))
	if len(parts) == 0 {
		return ""
	}
	if len(parts) == 1 {
		return parts[0]
	}

	initStart := len(parts)
	for i := 1; i < len(parts); i++ {
		if looksLikeInitial(parts[i]) {
			initStart = i
			break
		}
	}
	if initStart == len(parts) {
		return strings.Join(parts, " ")
	}

	surname := strings.Join(parts[:initStart], " ")
	initials := compactInitials(parts[initStart:])
	if initials == "" {
		return surname
	}
	return strings.TrimSpace(surname + " " + initials)
}

func looksLikeInitial(part string) bool {
	part = strings.TrimSuffix(part, ".")
	runes := []rune(part)
	return len(runes) > 0 && len(runes) <= 2
}

func compactInitials(parts []string) string {
	letters := make([]rune, 0, len(parts))
	for _, part := range parts {
		for _, r := range part {
			if r == '.' || r == ' ' {
				continue
			}
			letters = append(letters, r)
		}
	}
	if len(letters) == 0 {
		return ""
	}

	var b strings.Builder
	for i, r := range letters {
		if i > 0 {
			b.WriteRune('.')
		}
		b.WriteRune(r)
	}
	b.WriteRune('.')
	return b.String()
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
	var computedTotal float64
	for _, item := range input.Items {
		normalized, itemTotal, err := normalizeEstimateItem(item)
		if err != nil {
			return domain.Estimate{}, err
		}
		computedTotal += itemTotal
		items = append(items, normalized)
	}

	total := computedTotal
	if input.Total != nil {
		total = *input.Total
	}

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
	for companyID := range s.companies {
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

CREATE TABLE IF NOT EXISTS app_estimate_lines (
    id TEXT PRIMARY KEY,
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
    sort_order INTEGER NOT NULL DEFAULT 0
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
`

	_, err := s.treeDB.Exec(ctx, sql)
	return err
}

func (s *FileStore) syncTreeToDB(ctx context.Context) error {
	if s.treeDB == nil {
		return nil
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
ON CONFLICT (id) DO UPDATE SET
    estimate_id = EXCLUDED.estimate_id,
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

	return tx.Commit(ctx)
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
	return nil
}

func (s *FileStore) listEstimatesDB(ctx context.Context, companyID string, includeAll bool) ([]domain.Estimate, error) {
	query := `SELECT id, company_id, object_id, code, title, description, district, fgis_set_id, status, total, created_at, updated_at FROM app_estimates`
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

	items := []domain.Estimate{}
	ids := []string{}
	for rows.Next() {
		var item domain.Estimate
		if err := rows.Scan(&item.ID, &item.CompanyID, &item.ObjectID, &item.Code, &item.Title, &item.Description, &item.District, &item.FgisSetID, &item.Status, &item.Total, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
		ids = append(ids, item.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return items, nil
	}

	lineRows, err := s.treeDB.Query(ctx, `SELECT id, estimate_id, line_type, source, code, original_code, name, quantity, unit, unit_price, total, raw_text, parsed_json, calc_json, calc_status, calc_error, revision, calculated_at FROM app_estimate_lines ORDER BY estimate_id, sort_order, id`)
	if err != nil {
		return nil, err
	}
	defer lineRows.Close()
	lineMap := map[string][]domain.EstimateItem{}
	for lineRows.Next() {
		var line domain.EstimateItem
		var estimateID string
		if err := lineRows.Scan(&line.ID, &estimateID, &line.Type, &line.Source, &line.Code, &line.OriginalCode, &line.Name, &line.Quantity, &line.Unit, &line.UnitPrice, &line.Total, &line.RawText, &line.ParsedJSON, &line.CalcJSON, &line.CalcStatus, &line.CalcError, &line.Revision, &line.CalculatedAt); err != nil {
			return nil, err
		}
		lineMap[estimateID] = append(lineMap[estimateID], line)
	}
	for i := range items {
		items[i].Items = lineMap[items[i].ID]
	}
	return items, nil
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
	currentItems, err := s.listEstimatesDB(ctx, companyID, includeAll)
	if err != nil {
		return domain.Estimate{}, err
	}
	var current *domain.Estimate
	for i := range currentItems {
		if currentItems[i].ID == id {
			current = &currentItems[i]
			break
		}
	}
	if current == nil {
		return domain.Estimate{}, ErrNotFound
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
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
ON CONFLICT (id) DO UPDATE SET object_id = EXCLUDED.object_id, code = EXCLUDED.code, title = EXCLUDED.title, description = EXCLUDED.description, district = EXCLUDED.district, fgis_set_id = EXCLUDED.fgis_set_id, status = EXCLUDED.status, total = EXCLUDED.total, updated_at = EXCLUDED.updated_at`,
		item.ID, item.CompanyID, item.ObjectID, item.Code, item.Title, item.Description, item.District, item.FgisSetID, item.Status, item.Total, item.CreatedAt, item.UpdatedAt)
	if err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `DELETE FROM app_estimate_lines WHERE estimate_id = $1`, item.ID); err != nil {
		return err
	}
	lines := ensureUniqueEstimateLineIDs(item.Items)
	for i, line := range lines {
		line.Revision = estimateLineRevision(item, line)
		if line.CalcStatus == "" && shouldEnqueueEstimateLineCalc(line) {
			line.CalcStatus = "queued"
		}
		_, err = tx.Exec(ctx, `INSERT INTO app_estimate_lines (id, estimate_id, line_type, source, code, original_code, name, quantity, unit, unit_price, total, raw_text, parsed_json, calc_json, calc_status, calc_error, revision, calculated_at, sort_order)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)`,
			line.ID, item.ID, estimateLineType(line.Type), estimateItemSource(line.Source), line.Code, line.OriginalCode, line.Name, line.Quantity, line.Unit, line.UnitPrice, line.Total, line.RawText, nullableJSON(line.ParsedJSON), nullableJSON(line.CalcJSON), line.CalcStatus, line.CalcError, line.Revision, line.CalculatedAt, i)
		if err != nil {
			return err
		}
		if shouldEnqueueEstimateLineCalc(line) {
			if err := enqueueEstimateLineCalcJobTx(ctx, tx, item, line); err != nil {
				return err
			}
		}
	}

	return tx.Commit(ctx)
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
	return nil
}

type UpdateAppSettingsInput struct {
	CalcWorkerCount int `json:"calcWorkerCount"`
}

func defaultAppSettings() domain.AppSettings {
	return domain.AppSettings{
		CalcWorkerCount: 2,
	}
}

func NormalizeCalcWorkerCount(value int) int {
	if value < 1 {
		return 1
	}
	if value > 16 {
		return 16
	}
	return value
}

func normalizeAppSettings(settings domain.AppSettings) domain.AppSettings {
	if settings.CalcWorkerCount == 0 {
		settings.CalcWorkerCount = defaultAppSettings().CalcWorkerCount
	}
	settings.CalcWorkerCount = NormalizeCalcWorkerCount(settings.CalcWorkerCount)
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
		CalcWorkerCount: NormalizeCalcWorkerCount(input.CalcWorkerCount),
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
	rows, err := s.treeDB.Query(ctx, `SELECT key, value FROM app_settings WHERE key = 'calc_worker_count'`)
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
	return settings, nil
}

type EstimateCalcJob struct {
	ID          string
	CompanyID   string
	EstimateID  string
	LineID      string
	Revision    int64
	Code        string
	FgisSetID   string
	District    string
	Attempts    int
	MaxAttempts int
}

type EstimateLineCalcResult struct {
	Code         string
	OriginalCode string
	Name         string
	Unit         string
	CalcJSON     json.RawMessage
}

type EstimateCalcStatus struct {
	LineID       string          `json:"lineId"`
	Revision     int64           `json:"revision"`
	Status       string          `json:"status"`
	Error        string          `json:"error,omitempty"`
	CalcJSON     json.RawMessage `json:"calcJson,omitempty"`
	CalculatedAt *time.Time      `json:"calculatedAt,omitempty"`
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
RETURNING id, company_id, estimate_id, line_id, revision, payload->>'code', payload->>'fgisSetId', payload->>'district', attempts, max_attempts
`, time.Now().UTC().Add(lease), workerID).Scan(
		&job.ID,
		&job.CompanyID,
		&job.EstimateID,
		&job.LineID,
		&job.Revision,
		&job.Code,
		&job.FgisSetID,
		&job.District,
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
    calc_json = $8,
    calc_status = 'done',
    calc_error = '',
    calculated_at = now()
WHERE estimate_id = $1 AND id = $2 AND revision = $3
`, job.EstimateID, job.LineID, job.Revision, result.Code, result.OriginalCode, result.Name, result.Unit, nullableJSON(result.CalcJSON))
	if err != nil {
		return err
	}

	status := "done"
	lastError := ""
	if tag.RowsAffected() == 0 {
		status = "failed"
		lastError = "stale line revision"
	}
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
	if _, err := tx.Exec(ctx, `
UPDATE app_estimate_lines
SET calc_status = $4,
    calc_error = $5
WHERE estimate_id = $1 AND id = $2 AND revision = $3
`, job.EstimateID, job.LineID, job.Revision, nextStatus, message); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *FileStore) ListEstimateCalcStatuses(ctx context.Context, companyID, estimateID string, includeAll bool) ([]EstimateCalcStatus, error) {
	if s.treeDB == nil {
		return []EstimateCalcStatus{}, nil
	}
	query := `
SELECT l.id, l.revision, l.calc_status, l.calc_error, l.calc_json, l.calculated_at
FROM app_estimate_lines l
JOIN app_estimates e ON e.id = l.estimate_id
WHERE l.estimate_id = $1`
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
		if err := rows.Scan(&item.LineID, &item.Revision, &item.Status, &item.Error, &item.CalcJSON, &item.CalculatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func enqueueEstimateLineCalcJobTx(ctx context.Context, tx pgx.Tx, estimate domain.Estimate, line domain.EstimateItem) error {
	payload, err := json.Marshal(map[string]any{
		"code":      estimateRecordCode(line),
		"fgisSetId": estimate.FgisSetID,
		"district":  estimate.District,
	})
	if err != nil {
		return err
	}
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
`, newID("calcjob"), estimate.CompanyID, estimate.ID, line.ID, line.Revision, payload)
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
		item.Name = ""
		item.Unit = ""
		item.UnitPrice = 0
		item.Total = 0
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

func validRole(role domain.Role) bool {
	switch role {
	case domain.RoleSuperAdmin, domain.RoleCompanyAdmin, domain.RoleUser:
		return true
	default:
		return false
	}
}

func (s *FileStore) ensureUserFlags() (bool, error) {
	changed := false
	for id, user := range s.users {
		updated := user
		userChanged := false

		if user.Email == "admin@example.com" {
			if !user.IsSuperAdministrator || !user.IsAdministrator || !user.Authorized {
				updated.IsSuperAdministrator = true
				updated.IsAdministrator = true
				updated.Authorized = true
				userChanged = true
			}
			if user.Name != "Суперадминистратор" {
				updated.Name = "Суперадминистратор"
				userChanged = true
			}
		}
		if user.Email == "manager@example.com" {
			if !user.IsAdministrator || !user.Authorized {
				updated.IsAdministrator = true
				updated.Authorized = true
				userChanged = true
			}
		}
		if user.Email == "user@example.com" && !user.Authorized {
			updated.Authorized = true
			userChanged = true
		}

		if userChanged {
			updated.UpdatedAt = time.Now().UTC()
			s.users[id] = updated
			changed = true
		}
	}

	for id, company := range s.companies {
		if company.Name == "Demo Company" {
			company.Name = "Система"
			company.UpdatedAt = time.Now().UTC()
			s.companies[id] = company
			changed = true
		}
	}

	return changed, nil
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

func normalizeLicenseItems(items map[string]int) map[string]int {
	normalized := make(map[string]int, len(domain.BaseSubsections()))
	for _, subsection := range domain.BaseSubsections() {
		count := 0
		if items != nil {
			if value, ok := items[string(subsection.ID)]; ok && value > 0 {
				count = value
			}
		}
		normalized[string(subsection.ID)] = count
	}
	return normalized
}

func (s *FileStore) companyLicensesLocked(companyID string) map[string]int {
	if items, ok := s.licenses[companyID]; ok {
		return normalizeLicenseItems(items)
	}
	return normalizeLicenseItems(nil)
}

func (s *FileStore) GetCompanyLicenses(companyID string) (domain.CompanyLicensesView, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	company, ok := s.companies[companyID]
	if !ok {
		return domain.CompanyLicensesView{}, ErrNotFound
	}

	items := s.companyLicensesLocked(companyID)
	viewItems := make([]domain.CompanyLicense, 0, len(domain.BaseSubsections()))
	for _, subsection := range domain.BaseSubsections() {
		viewItems = append(viewItems, domain.CompanyLicense{
			SubsectionID: subsection.ID,
			Name:         subsection.Name,
			Available:    items[string(subsection.ID)],
		})
	}

	return domain.CompanyLicensesView{
		CompanyID:   company.ID,
		CompanyName: company.Name,
		Items:       viewItems,
	}, nil
}

type UpdateCompanyLicensesInput struct {
	Items map[string]int `json:"items"`
}

func (s *FileStore) UpdateCompanyLicenses(companyID string, input UpdateCompanyLicensesInput) (domain.CompanyLicensesView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.companies[companyID]; !ok {
		return domain.CompanyLicensesView{}, ErrNotFound
	}

	items := normalizeLicenseItems(nil)
	for key, value := range input.Items {
		if !domain.ValidBaseSubsectionID(domain.BaseSubsectionID(key)) {
			return domain.CompanyLicensesView{}, ErrConflict
		}
		if value < 0 {
			return domain.CompanyLicensesView{}, ErrConflict
		}
		items[key] = value
	}

	s.licenses[companyID] = items
	if err := s.saveLocked(); err != nil {
		return domain.CompanyLicensesView{}, err
	}

	company := s.companies[companyID]
	viewItems := make([]domain.CompanyLicense, 0, len(domain.BaseSubsections()))
	for _, subsection := range domain.BaseSubsections() {
		viewItems = append(viewItems, domain.CompanyLicense{
			SubsectionID: subsection.ID,
			Name:         subsection.Name,
			Available:    items[string(subsection.ID)],
		})
	}

	return domain.CompanyLicensesView{
		CompanyID:   company.ID,
		CompanyName: company.Name,
		Items:       viewItems,
	}, nil
}

func (s *FileStore) LicenseAvailable(companyID, subsectionID string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, ok := s.companies[companyID]; !ok {
		return 0
	}
	return s.companyLicensesLocked(companyID)[subsectionID]
}
