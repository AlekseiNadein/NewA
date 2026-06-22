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
	ID                   string    `json:"id"`
	CompanyID            string    `json:"companyId"`
	Email                string    `json:"email"`
	Name                 string    `json:"name"`
	Authorized           bool      `json:"authorized"`
	IsAdministrator      bool      `json:"isAdministrator"`
	IsSuperAdministrator bool      `json:"isSuperAdministrator"`
	PasswordHash         string    `json:"-"`
	PasswordSalt         string    `json:"-"`
	CreatedAt            time.Time `json:"createdAt"`
	UpdatedAt            time.Time `json:"updatedAt"`
}

func (u User) Role() Role {
	if u.IsSuperAdministrator {
		return RoleSuperAdmin
	}
	if u.IsAdministrator {
		return RoleCompanyAdmin
	}
	return RoleUser
}

func (u User) CanAccessAdmin() bool {
	return u.IsAdministrator || u.IsSuperAdministrator
}

func (u User) CanAccessApp() bool {
	return u.Authorized
}

type Construction struct {
	ID        string    `json:"id"`
	CompanyID string    `json:"companyId"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type ConstructionObject struct {
	ID             string    `json:"id"`
	CompanyID      string    `json:"companyId"`
	ConstructionID string    `json:"constructionId"`
	Code           string    `json:"code"`
	Name           string    `json:"name"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
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
	ObjectID    string         `json:"objectId"`
	Code        string         `json:"code"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	District    string         `json:"district"`
	FgisSetID   string         `json:"fgisSetId"`
	Status      EstimateStatus `json:"status"`
	Items       []EstimateItem `json:"items"`
	Total       float64        `json:"total"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
}

type EstimateLineType string

const (
	EstimateLineSection    EstimateLineType = "section"
	EstimateLineSubsection EstimateLineType = "subsection"
	EstimateLinePosition   EstimateLineType = "position"
)

type EstimateItem struct {
	ID        string  `json:"id"`
	Type      string  `json:"type"`
	Code      string  `json:"code"`
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
