package authstore

import "nav-saas-mvp/backend/internal/domain"

type UserView struct {
	domain.User
	CompanyName string `json:"companyName"`
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

type UpdateCompanyLicensesInput struct {
	Items map[string]int `json:"items"`
}
