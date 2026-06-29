package domain

import (
	"encoding/json"
	"strings"
	"time"
)

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
	ID           string          `json:"id"`
	Type         string          `json:"type"`
	Source       string          `json:"source,omitempty"`
	Code         string          `json:"code"`
	OriginalCode string          `json:"originalCode,omitempty"`
	Name         string          `json:"name"`
	Quantity     float64         `json:"quantity"`
	Unit         string          `json:"unit"`
	UnitPrice    float64         `json:"unitPrice"`
	Total        float64         `json:"total"`
	RawText      string          `json:"rawText,omitempty"`
	ParsedJSON   json.RawMessage `json:"parsedJson,omitempty"`
	CalcJSON     json.RawMessage `json:"calcJson,omitempty"`
	CalcStatus   string          `json:"calcStatus,omitempty"`
	CalcError    string          `json:"calcError,omitempty"`
	Revision     int64           `json:"revision,omitempty"`
	CalculatedAt *time.Time      `json:"calculatedAt,omitempty"`
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

func (r Role) CanManageLicenses() bool {
	return r == RoleSuperAdmin
}

type BaseSubsectionID string

const (
	BaseSubsectionGSNSupplement18  BaseSubsectionID = "gsn_supplement_18"
	BaseSubsectionFGISAlrosaQ22026 BaseSubsectionID = "fgis_alrosa_q2_2026"
	BaseSubsectionFGISRZDQ12026    BaseSubsectionID = "fgis_rzd_q1_2026"
)

type BaseSubsection struct {
	ID   BaseSubsectionID `json:"id"`
	Name string           `json:"name"`
}

func BaseSubsections() []BaseSubsection {
	return []BaseSubsection{
		{ID: BaseSubsectionGSNSupplement18, Name: "ГСН-2022 доп. 18"},
		{ID: BaseSubsectionFGISAlrosaQ22026, Name: "ФГИС ЦС Алроса II кв. 2026 г."},
		{ID: BaseSubsectionFGISRZDQ12026, Name: "ФГИС ЦС РЖД I кв. 2026 г."},
	}
}

func ValidBaseSubsectionID(id BaseSubsectionID) bool {
	for _, item := range BaseSubsections() {
		if item.ID == id {
			return true
		}
	}
	return false
}

func BaseSubsectionName(id BaseSubsectionID) string {
	for _, item := range BaseSubsections() {
		if item.ID == id {
			return item.Name
		}
	}
	return string(id)
}

func SubsectionIDForGSNSupplement(code string) (BaseSubsectionID, bool) {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(code), " ", ""))
	if normalized == "доп.18" {
		return BaseSubsectionGSNSupplement18, true
	}
	return "", false
}

func SubsectionIDForFGISSet(setID, setName string) (BaseSubsectionID, bool) {
	switch strings.TrimSpace(setID) {
	case "alrosa-2026-q2":
		return BaseSubsectionFGISAlrosaQ22026, true
	case "rzd-2026-q1":
		return BaseSubsectionFGISRZDQ12026, true
	}

	name := strings.ToLower(strings.TrimSpace(setName))
	if strings.Contains(name, "алроса") {
		return BaseSubsectionFGISAlrosaQ22026, true
	}
	if strings.Contains(name, "ржд") {
		return BaseSubsectionFGISRZDQ12026, true
	}
	return "", false
}

type CompanyLicense struct {
	SubsectionID BaseSubsectionID `json:"subsectionId"`
	Name         string           `json:"name"`
	Available    int              `json:"available"`
}

type CompanyLicensesView struct {
	CompanyID   string           `json:"companyId"`
	CompanyName string           `json:"companyName"`
	Editable    bool             `json:"editable"`
	Items       []CompanyLicense `json:"items"`
}

type AppSettings struct {
	CalcWorkerCount int  `json:"calcWorkerCount"`
	Editable        bool `json:"editable"`
}
