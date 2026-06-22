package store

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

func TestNormalizeCompanyNameQuotes(t *testing.T) {
	canonical := normalizeCompanyName(`ООО НПП "АВС-Н"`)
	variants := []string{
		`ООО НПП "АВС-Н"`,
		`ООО НПП «АВС-Н»`,
		`ООО НПП “АВС-Н”`,
	}
	for _, variant := range variants {
		if got := normalizeCompanyName(variant); got != canonical {
			t.Fatalf("normalizeCompanyName(%q) = %q, want %q", variant, got, canonical)
		}
	}
}

func TestFindUserForLoginByEmail(t *testing.T) {
	store := &FileStore{
		companies: map[string]domain.Company{
			"cmp_test": {ID: "cmp_test", Name: `ООО НПП "АВС-Н"`},
		},
		users: map[string]domain.User{
			"usr_test": {
				ID:        "usr_test",
				CompanyID: "cmp_test",
				Email:     "user@example.com",
				Name:      "Штайгер А.Ф.",
			},
		},
	}

	user, ok := store.FindUserForLogin("", "user@example.com")
	if !ok || user.ID != "usr_test" {
		t.Fatalf("expected global email login, got ok=%v user=%+v", ok, user)
	}

	user, ok = store.FindUserForLogin("wrong", "user@example.com")
	if !ok || user.ID != "usr_test" {
		t.Fatalf("expected email login to ignore wrong company, got ok=%v user=%+v", ok, user)
	}
}

func TestFindUserByCompanyAndNameByEmail(t *testing.T) {
	store := &FileStore{
		companies: map[string]domain.Company{
			"cmp_test": {ID: "cmp_test", Name: `ООО НПП "АВС-Н"`},
		},
		users: map[string]domain.User{
			"usr_test": {
				ID:        "usr_test",
				CompanyID: "cmp_test",
				Email:     "user@example.com",
				Name:      "Штайгер А.Ф.",
			},
		},
	}

	user, ok := store.FindUserByCompanyAndName(`ООО НПП «АВС-Н»`, "user@example.com")
	if !ok || user.ID != "usr_test" {
		t.Fatalf("expected login by email, got ok=%v user=%+v", ok, user)
	}

	user, ok = store.FindUserByCompanyAndName(`ООО НПП "АВС-Н"`, "Штайгер А. Ф.")
	if !ok || user.ID != "usr_test" {
		t.Fatalf("expected login by spaced initials, got ok=%v user=%+v", ok, user)
	}
}
