package authstore

import "nav-saas-mvp/backend/internal/domain"

// AccountStore is the auth/account boundary: users, companies, license quotas.
type AccountStore interface {
	FindUserForLogin(companyName, login string) (domain.User, bool)
	FindUserByID(id string) (domain.User, bool)
	ListCompanies() []domain.Company
	CreateCompany(name string) (domain.Company, error)
	ListUsers(companyID string, includeAll bool) []UserView
	UserView(user domain.User) UserView
	CreateUser(input NewUser) (domain.User, error)
	RegisterUser(input RegisterUser) (domain.User, error)
	UpdateUser(id, actorCompanyID string, includeAll bool, input UpdateUser) (domain.User, error)
	DeleteUser(id, actorCompanyID string, includeAll bool) error
	GetCompanyLicenses(companyID string) (domain.CompanyLicensesView, error)
	UpdateCompanyLicenses(companyID string, input UpdateCompanyLicensesInput) (domain.CompanyLicensesView, error)
	LicenseAvailable(companyID, subsectionID string) int
}
