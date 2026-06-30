package authstore

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"

	"nav-saas-mvp/backend/internal/domain"
)

type importSnapshot struct {
	Companies []domain.Company       `json:"companies"`
	Users     []importStoredUser     `json:"users"`
	Licenses  []importCompanyLicense `json:"licenses,omitempty"`
}

type importStoredUser struct {
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

type importCompanyLicense struct {
	CompanyID string         `json:"companyId"`
	Items     map[string]int `json:"items"`
}

func (s *Store) ImportIfEmpty(ctx context.Context, appJSONPath string) (bool, error) {
	count, err := s.UserCount(ctx)
	if err != nil {
		return false, err
	}
	if count > 0 {
		return false, nil
	}
	if err := s.ImportFromAppJSON(ctx, appJSONPath); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) ImportFromAppJSON(ctx context.Context, path string) error {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var snap importSnapshot
	if err := json.Unmarshal(content, &snap); err != nil {
		return err
	}
	if len(snap.Users) == 0 && len(snap.Companies) == 0 {
		return nil
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for _, company := range snap.Companies {
		company = migrateImportCompany(company)
		_, err = tx.Exec(ctx, `INSERT INTO auth.companies (id, name, created_at, updated_at)
			VALUES ($1, $2, $3, $4) ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, updated_at = EXCLUDED.updated_at`,
			company.ID, company.Name, company.CreatedAt, company.UpdatedAt)
		if err != nil {
			return err
		}
	}

	for _, stored := range snap.Users {
		user := migrateImportUser(stored).toDomain()
		if user.CompanyID == "" || user.Email == "" {
			continue
		}
		_, err = tx.Exec(ctx, `INSERT INTO auth.users
			(id, company_id, email, name, authorized, is_administrator, is_super_administrator, password_hash, password_salt, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			ON CONFLICT (id) DO UPDATE SET
				company_id = EXCLUDED.company_id,
				email = EXCLUDED.email,
				name = EXCLUDED.name,
				authorized = EXCLUDED.authorized,
				is_administrator = EXCLUDED.is_administrator,
				is_super_administrator = EXCLUDED.is_super_administrator,
				password_hash = EXCLUDED.password_hash,
				password_salt = EXCLUDED.password_salt,
				updated_at = EXCLUDED.updated_at`,
			user.ID, user.CompanyID, strings.ToLower(user.Email), user.Name, user.Authorized, user.IsAdministrator,
			user.IsSuperAdministrator, user.PasswordHash, user.PasswordSalt, user.CreatedAt, user.UpdatedAt)
		if err != nil {
			return err
		}
	}

	now := time.Now().UTC()
	for _, entry := range snap.Licenses {
		if entry.CompanyID == "" || entry.Items == nil {
			continue
		}
		items := normalizeLicenseItems(entry.Items)
		for subsectionID, available := range items {
			_, err = tx.Exec(ctx, `INSERT INTO auth.company_licenses (company_id, subsection_id, available, updated_at)
				VALUES ($1, $2, $3, $4)
				ON CONFLICT (company_id, subsection_id) DO UPDATE SET available = EXCLUDED.available, updated_at = EXCLUDED.updated_at`,
				entry.CompanyID, subsectionID, available, now)
			if err != nil {
				return err
			}
		}
	}

	return tx.Commit(ctx)
}

func migrateImportCompany(company domain.Company) domain.Company {
	if company.Name == "Demo Company" {
		company.Name = "Система"
	}
	return company
}

func migrateImportUser(user importStoredUser) importStoredUser {
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

func (u importStoredUser) toDomain() domain.User {
	user := domain.User{
		ID:                   u.ID,
		CompanyID:            u.CompanyID,
		Email:                strings.ToLower(strings.TrimSpace(u.Email)),
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

// ImportFromAppJSONFile is a CLI helper that reports imported row counts.
func (s *Store) ImportFromAppJSONFile(ctx context.Context, path string) (users int, companies int, err error) {
	if err = s.ImportFromAppJSON(ctx, path); err != nil {
		return 0, 0, err
	}
	if err = s.db.QueryRow(ctx, `SELECT COUNT(*) FROM auth.users`).Scan(&users); err != nil {
		return 0, 0, err
	}
	if err = s.db.QueryRow(ctx, `SELECT COUNT(*) FROM auth.companies`).Scan(&companies); err != nil {
		return 0, 0, err
	}
	return users, companies, nil
}
