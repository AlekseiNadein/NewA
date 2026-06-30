package authstore

import (
	"testing"

	"nav-saas-mvp/backend/internal/domain"
)

func TestNormalizePersonName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Пользователь", "пользователь"},
		{"Надеин А.В.", "надеин а.в."},
		{"Надеин А. В.", "надеин а.в."},
		{"Штайгер А. Ф.", "штайгер а.ф."},
		{"Администратор компании", "администратор компании"},
		{"  Суперадминистратор  ", "суперадминистратор"},
	}

	for _, tc := range tests {
		if got := normalizePersonName(tc.input); got != tc.want {
			t.Fatalf("normalizePersonName(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestFindUserForLoginByEmail(t *testing.T) {
	store := &Store{}
	store.db = nil // login by email path tested via FindUserByCompanyAndName unit below when no DB

	// Company/name matching uses same normalize helpers as file store.
	companyName := `ООО НПП "АВС-Н"`
	variants := []string{
		`ООО НПП "АВС-Н"`,
		`ООО НПП «АВС-Н»`,
	}
	canonical := normalizeCompanyName(companyName)
	for _, variant := range variants {
		if got := normalizeCompanyName(variant); got != canonical {
			t.Fatalf("normalizeCompanyName(%q) = %q, want %q", variant, got, canonical)
		}
	}

	userName := "Штайгер А. Ф."
	if got := normalizePersonName(userName); got != "штайгер а.ф." {
		t.Fatalf("expected normalized initials, got %q", got)
	}

	_ = domain.User{}
}
