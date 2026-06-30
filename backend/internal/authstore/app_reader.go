package authstore

import "nav-saas-mvp/backend/internal/domain"

// AppReader is read-only auth data needed by the app server (licenses, company names).
type AppReader interface {
	LicenseAvailable(companyID, subsectionID string) int
	ListCompanies() []domain.Company
}
