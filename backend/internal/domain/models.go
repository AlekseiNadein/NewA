package domain

import "time"

type Role string

const (
	RoleSuperAdmin   Role = "super_admin"
	RoleCompanyAdmin Role = "company_admin"
	RoleUser         Role = "user"
)

type Company struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type User struct {
	ID           string    `json:"id"`
	CompanyID    string    `json:"companyId"`
	Email        string    `json:"email"`
	Name         string    `json:"name"`
	Role         Role      `json:"role"`
	PasswordHash string    `json:"-"`
	PasswordSalt string    `json:"-"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type EstimateStatus string

const (
	EstimateDraft    EstimateStatus = "draft"
	EstimateApproved EstimateStatus = "approved"
	EstimateArchived EstimateStatus = "archived"
)

type Estimate struct {
	ID          string         `json:"id"`
	CompanyID   string         `json:"companyId"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Status      EstimateStatus `json:"status"`
	Items       []EstimateItem `json:"items"`
	Total       float64        `json:"total"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
}

type EstimateItem struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Quantity  float64 `json:"quantity"`
	Unit      string  `json:"unit"`
	UnitPrice float64 `json:"unitPrice"`
	Total     float64 `json:"total"`
}

type Claims struct {
	UserID    string `json:"sub"`
	CompanyID string `json:"companyId"`
	Email     string `json:"email"`
	Role      Role   `json:"role"`
	ExpiresAt int64  `json:"exp"`
}

func (r Role) CanManageCompanies() bool {
	return r == RoleSuperAdmin
}

func (r Role) CanManageUsers() bool {
	return r == RoleSuperAdmin || r == RoleCompanyAdmin
}

func (r Role) CanEditEstimates() bool {
	return r == RoleSuperAdmin || r == RoleCompanyAdmin || r == RoleUser
}
