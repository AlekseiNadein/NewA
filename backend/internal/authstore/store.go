package authstore

import (
	"context"
	"errors"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"nav-saas-mvp/backend/internal/auth"
	"nav-saas-mvp/backend/internal/domain"
)

var schemaSQL string

func init() {
	paths := []string{
		"db/auth_schema.sql",
		"backend/internal/authstore/schema.sql",
	}
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err == nil {
			schemaSQL = string(content)
			return
		}
	}
}

type Store struct {
	db *pgxpool.Pool
}

func New(ctx context.Context, databaseURL string) (*Store, error) {
	databaseURL = strings.TrimSpace(databaseURL)
	if databaseURL == "" {
		return nil, errors.New("auth database URL is required")
	}

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}

	store := &Store{db: pool}
	if err := store.ensureSchema(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	return store, nil
}

func (s *Store) Close() {
	if s.db != nil {
		s.db.Close()
	}
}

func (s *Store) ensureSchema(ctx context.Context) error {
	sql := schemaSQL
	if sql == "" {
		if content, err := os.ReadFile("db/auth_schema.sql"); err == nil {
			sql = string(content)
		}
	}
	if sql == "" {
		return errors.New("auth schema SQL is not available")
	}
	_, err := s.db.Exec(ctx, sql)
	return err
}

func (s *Store) UserCount(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM auth.users`).Scan(&count)
	return count, err
}

func (s *Store) FindUserByEmail(ctx context.Context, email string) (domain.User, bool) {
	normalized := strings.ToLower(strings.TrimSpace(email))
	user, err := s.scanUser(ctx, `SELECT u.id, u.company_id, u.email, u.name, u.authorized, u.is_administrator, u.is_super_administrator,
		u.password_hash, u.password_salt, u.created_at, u.updated_at
		FROM auth.users u WHERE lower(u.email) = $1`, normalized)
	if err != nil {
		return domain.User{}, false
	}
	return user, true
}

func (s *Store) FindUserForLogin(companyName, login string) (domain.User, bool) {
	ctx := context.Background()
	login = strings.TrimSpace(login)
	if login == "" {
		return domain.User{}, false
	}

	normalizedEmail := strings.ToLower(login)
	if strings.Contains(normalizedEmail, "@") {
		if user, ok := s.FindUserByEmail(ctx, normalizedEmail); ok {
			return user, true
		}
	}

	return s.FindUserByCompanyAndName(ctx, companyName, login)
}

func (s *Store) FindUserByCompanyAndName(ctx context.Context, companyName, userName string) (domain.User, bool) {
	company, ok := s.findCompanyByName(ctx, companyName)
	if !ok {
		return domain.User{}, false
	}

	normalizedName := normalizePersonName(userName)
	normalizedEmail := strings.ToLower(strings.TrimSpace(userName))
	loginByEmail := strings.Contains(normalizedEmail, "@")

	rows, err := s.db.Query(ctx, `SELECT id, company_id, email, name, authorized, is_administrator, is_super_administrator,
		password_hash, password_salt, created_at, updated_at
		FROM auth.users WHERE company_id = $1`, company.ID)
	if err != nil {
		return domain.User{}, false
	}
	defer rows.Close()

	for rows.Next() {
		user, err := scanUserRow(rows)
		if err != nil {
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

func (s *Store) FindUserByID(id string) (domain.User, bool) {
	user, err := s.scanUser(context.Background(), `SELECT id, company_id, email, name, authorized, is_administrator, is_super_administrator,
		password_hash, password_salt, created_at, updated_at FROM auth.users WHERE id = $1`, id)
	if err != nil {
		return domain.User{}, false
	}
	return user, true
}

func (s *Store) ListCompanies() []domain.Company {
	ctx := context.Background()
	rows, err := s.db.Query(ctx, `SELECT id, name, created_at, updated_at FROM auth.companies ORDER BY name`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	companies := make([]domain.Company, 0)
	for rows.Next() {
		var company domain.Company
		if err := rows.Scan(&company.ID, &company.Name, &company.CreatedAt, &company.UpdatedAt); err != nil {
			continue
		}
		companies = append(companies, company)
	}
	return companies
}

func (s *Store) CreateCompany(name string) (domain.Company, error) {
	ctx := context.Background()
	name = strings.TrimSpace(name)
	if name == "" {
		return domain.Company{}, ErrConflict
	}

	var existingID string
	err := s.db.QueryRow(ctx, `SELECT id FROM auth.companies WHERE lower(name) = lower($1)`, name).Scan(&existingID)
	if err == nil {
		return domain.Company{}, ErrConflict
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.Company{}, err
	}

	now := time.Now().UTC()
	company := domain.Company{
		ID:        newID("cmp"),
		Name:      name,
		CreatedAt: now,
		UpdatedAt: now,
	}

	_, err = s.db.Exec(ctx, `INSERT INTO auth.companies (id, name, created_at, updated_at) VALUES ($1, $2, $3, $4)`,
		company.ID, company.Name, company.CreatedAt, company.UpdatedAt)
	if err != nil {
		return domain.Company{}, err
	}
	return company, nil
}

func (s *Store) FindOrCreateCompanyByName(name string) (domain.Company, error) {
	ctx := context.Background()
	name = strings.TrimSpace(name)
	if name == "" {
		return domain.Company{}, ErrConflict
	}

	if company, ok := s.findCompanyByName(ctx, name); ok {
		return company, nil
	}
	return s.CreateCompany(name)
}

func (s *Store) UserView(user domain.User) UserView {
	ctx := context.Background()
	companyName := ""
	var name string
	err := s.db.QueryRow(ctx, `SELECT name FROM auth.companies WHERE id = $1`, user.CompanyID).Scan(&name)
	if err == nil {
		companyName = name
	}
	return UserView{User: user, CompanyName: companyName}
}

func (s *Store) ListUsers(companyID string, includeAll bool) []UserView {
	ctx := context.Background()
	query := `SELECT u.id, u.company_id, u.email, u.name, u.authorized, u.is_administrator, u.is_super_administrator,
		u.password_hash, u.password_salt, u.created_at, u.updated_at, c.name
		FROM auth.users u JOIN auth.companies c ON c.id = u.company_id`
	args := []any{}
	if !includeAll {
		query += ` WHERE u.company_id = $1`
		args = append(args, companyID)
	}
	query += ` ORDER BY c.name, u.name`

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()

	users := make([]UserView, 0)
	for rows.Next() {
		var user domain.User
		var companyName string
		if err := rows.Scan(&user.ID, &user.CompanyID, &user.Email, &user.Name, &user.Authorized, &user.IsAdministrator,
			&user.IsSuperAdministrator, &user.PasswordHash, &user.PasswordSalt, &user.CreatedAt, &user.UpdatedAt, &companyName); err != nil {
			continue
		}
		users = append(users, UserView{User: user, CompanyName: companyName})
	}
	return users
}

func (s *Store) CreateUser(input NewUser) (domain.User, error) {
	ctx := context.Background()
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	input.Name = strings.TrimSpace(input.Name)
	input.Password = strings.TrimSpace(input.Password)
	input.CompanyName = strings.TrimSpace(input.CompanyName)

	if input.Email == "" || input.Name == "" || input.Password == "" {
		return domain.User{}, ErrConflict
	}

	if input.CompanyID == "" && input.CompanyName != "" {
		company, err := s.FindOrCreateCompanyByName(input.CompanyName)
		if err != nil {
			return domain.User{}, err
		}
		input.CompanyID = company.ID
	}
	if input.CompanyID == "" {
		return domain.User{}, ErrConflict
	}

	if !s.companyExists(ctx, input.CompanyID) {
		return domain.User{}, ErrNotFound
	}
	if s.emailExists(ctx, input.Email, "") {
		return domain.User{}, ErrConflict
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

	_, err = s.db.Exec(ctx, `INSERT INTO auth.users
		(id, company_id, email, name, authorized, is_administrator, is_super_administrator, password_hash, password_salt, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		user.ID, user.CompanyID, user.Email, user.Name, user.Authorized, user.IsAdministrator, user.IsSuperAdministrator,
		user.PasswordHash, user.PasswordSalt, user.CreatedAt, user.UpdatedAt)
	if err != nil {
		return domain.User{}, err
	}
	return user, nil
}

func (s *Store) RegisterUser(input RegisterUser) (domain.User, error) {
	ctx := context.Background()
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
	if s.emailExists(ctx, input.Email, "") {
		return domain.User{}, ErrConflict
	}

	company, err := s.FindOrCreateCompanyByName(input.CompanyName)
	if err != nil {
		return domain.User{}, err
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

	_, err = s.db.Exec(ctx, `INSERT INTO auth.users
		(id, company_id, email, name, authorized, is_administrator, is_super_administrator, password_hash, password_salt, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		user.ID, user.CompanyID, user.Email, user.Name, user.Authorized, user.IsAdministrator, user.IsSuperAdministrator,
		user.PasswordHash, user.PasswordSalt, user.CreatedAt, user.UpdatedAt)
	if err != nil {
		return domain.User{}, err
	}
	return user, nil
}

func (s *Store) UpdateUser(id string, actorCompanyID string, includeAll bool, input UpdateUser) (domain.User, error) {
	ctx := context.Background()
	current, ok := s.FindUserByID(id)
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
	if s.emailExists(ctx, input.Email, id) {
		return domain.User{}, ErrConflict
	}

	companyID := current.CompanyID
	if includeAll && strings.TrimSpace(input.CompanyID) != "" {
		if !s.companyExists(ctx, input.CompanyID) {
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

	_, err := s.db.Exec(ctx, `UPDATE auth.users SET company_id = $2, email = $3, name = $4, authorized = $5,
		is_administrator = $6, is_super_administrator = $7, password_hash = $8, password_salt = $9, updated_at = $10
		WHERE id = $1`,
		updated.ID, updated.CompanyID, updated.Email, updated.Name, updated.Authorized, updated.IsAdministrator,
		updated.IsSuperAdministrator, updated.PasswordHash, updated.PasswordSalt, updated.UpdatedAt)
	if err != nil {
		return domain.User{}, err
	}
	return updated, nil
}

func (s *Store) DeleteUser(id string, actorCompanyID string, includeAll bool) error {
	ctx := context.Background()
	current, ok := s.FindUserByID(id)
	if !ok {
		return ErrNotFound
	}
	if current.IsSuperAdministrator {
		return ErrForbidden
	}
	if !includeAll && current.CompanyID != actorCompanyID {
		return ErrForbidden
	}

	tag, err := s.db.Exec(ctx, `DELETE FROM auth.users WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) GetCompanyLicenses(companyID string) (domain.CompanyLicensesView, error) {
	ctx := context.Background()
	var company domain.Company
	err := s.db.QueryRow(ctx, `SELECT id, name, created_at, updated_at FROM auth.companies WHERE id = $1`, companyID).
		Scan(&company.ID, &company.Name, &company.CreatedAt, &company.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.CompanyLicensesView{}, ErrNotFound
		}
		return domain.CompanyLicensesView{}, err
	}

	items, err := s.loadCompanyLicenses(ctx, companyID)
	if err != nil {
		return domain.CompanyLicensesView{}, err
	}

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

func (s *Store) UpdateCompanyLicenses(companyID string, input UpdateCompanyLicensesInput) (domain.CompanyLicensesView, error) {
	ctx := context.Background()
	var companyName string
	err := s.db.QueryRow(ctx, `SELECT name FROM auth.companies WHERE id = $1`, companyID).Scan(&companyName)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.CompanyLicensesView{}, ErrNotFound
		}
		return domain.CompanyLicensesView{}, err
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

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return domain.CompanyLicensesView{}, err
	}
	defer tx.Rollback(ctx)

	now := time.Now().UTC()
	for subsectionID, available := range items {
		_, err = tx.Exec(ctx, `INSERT INTO auth.company_licenses (company_id, subsection_id, available, updated_at)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (company_id, subsection_id) DO UPDATE SET available = EXCLUDED.available, updated_at = EXCLUDED.updated_at`,
			companyID, subsectionID, available, now)
		if err != nil {
			return domain.CompanyLicensesView{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.CompanyLicensesView{}, err
	}

	viewItems := make([]domain.CompanyLicense, 0, len(domain.BaseSubsections()))
	for _, subsection := range domain.BaseSubsections() {
		viewItems = append(viewItems, domain.CompanyLicense{
			SubsectionID: subsection.ID,
			Name:         subsection.Name,
			Available:    items[string(subsection.ID)],
		})
	}

	return domain.CompanyLicensesView{
		CompanyID:   companyID,
		CompanyName: companyName,
		Items:       viewItems,
	}, nil
}

func (s *Store) LicenseAvailable(companyID, subsectionID string) int {
	ctx := context.Background()
	if !s.companyExists(ctx, companyID) {
		return 0
	}
	items, err := s.loadCompanyLicenses(ctx, companyID)
	if err != nil {
		return 0
	}
	return items[subsectionID]
}

func (s *Store) SeedDefault(ctx context.Context) error {
	count, err := s.UserCount(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	company, err := s.CreateCompany("Система")
	if err != nil && !errors.Is(err, ErrConflict) {
		return err
	}
	if errors.Is(err, ErrConflict) {
		company, _ = s.findCompanyByName(ctx, "Система")
	}

	hash, salt, err := auth.NewPassword("admin123")
	if err != nil {
		return err
	}

	now := time.Now().UTC()
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

	_, err = s.db.Exec(ctx, `INSERT INTO auth.users
		(id, company_id, email, name, authorized, is_administrator, is_super_administrator, password_hash, password_salt, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (email) DO NOTHING`,
		user.ID, user.CompanyID, user.Email, user.Name, user.Authorized, user.IsAdministrator, user.IsSuperAdministrator,
		user.PasswordHash, user.PasswordSalt, user.CreatedAt, user.UpdatedAt)
	return err
}

func (s *Store) findCompanyByName(ctx context.Context, name string) (domain.Company, bool) {
	normalized := normalizeCompanyName(name)
	rows, err := s.db.Query(ctx, `SELECT id, name, created_at, updated_at FROM auth.companies`)
	if err != nil {
		return domain.Company{}, false
	}
	defer rows.Close()

	for rows.Next() {
		var company domain.Company
		if err := rows.Scan(&company.ID, &company.Name, &company.CreatedAt, &company.UpdatedAt); err != nil {
			continue
		}
		if normalizeCompanyName(company.Name) == normalized {
			return company, true
		}
	}
	return domain.Company{}, false
}

func (s *Store) companyExists(ctx context.Context, id string) bool {
	var exists bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM auth.companies WHERE id = $1)`, id).Scan(&exists)
	return err == nil && exists
}

func (s *Store) emailExists(ctx context.Context, email, excludeID string) bool {
	var id string
	err := s.db.QueryRow(ctx, `SELECT id FROM auth.users WHERE lower(email) = lower($1)`, email).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false
	}
	if err != nil {
		return true
	}
	return excludeID == "" || id != excludeID
}

func (s *Store) loadCompanyLicenses(ctx context.Context, companyID string) (map[string]int, error) {
	items := normalizeLicenseItems(nil)
	rows, err := s.db.Query(ctx, `SELECT subsection_id, available FROM auth.company_licenses WHERE company_id = $1`, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var subsectionID string
		var available int
		if err := rows.Scan(&subsectionID, &available); err != nil {
			continue
		}
		if available > 0 {
			items[subsectionID] = available
		}
	}
	return items, nil
}

func (s *Store) scanUser(ctx context.Context, query string, args ...any) (domain.User, error) {
	row := s.db.QueryRow(ctx, query, args...)
	return scanUserRow(row)
}

type userScanner interface {
	Scan(dest ...any) error
}

func scanUserRow(row userScanner) (domain.User, error) {
	var user domain.User
	err := row.Scan(&user.ID, &user.CompanyID, &user.Email, &user.Name, &user.Authorized, &user.IsAdministrator,
		&user.IsSuperAdministrator, &user.PasswordHash, &user.PasswordSalt, &user.CreatedAt, &user.UpdatedAt)
	return user, err
}

// SortCompanies is used by import to keep stable ordering.
func SortCompanies(companies []domain.Company) {
	sort.Slice(companies, func(i, j int) bool {
		return companies[i].Name < companies[j].Name
	})
}
