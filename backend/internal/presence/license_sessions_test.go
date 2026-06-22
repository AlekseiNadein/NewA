package presence

import "testing"

func TestLicenseSessionsSyncRespectsPoolSize(t *testing.T) {
	sessions := NewLicenseSessions()
	available := func(string) int { return 1 }
	names := func(id string) string { return id }

	first := sessions.Sync("cmp_1", "usr_1", "User 1", []string{"gsn_supplement_18"}, available, names)
	if !first.Granted || len(first.Held) != 1 {
		t.Fatalf("expected first user to acquire license, got %+v", first)
	}

	second := sessions.Sync("cmp_1", "usr_2", "User 2", []string{"gsn_supplement_18"}, available, names)
	if second.Granted || len(second.Blocked) != 1 {
		t.Fatalf("expected second user to be blocked, got %+v", second)
	}

	if sessions.countActiveLocked("cmp_1", "gsn_supplement_18") != 1 {
		t.Fatalf("expected one active session")
	}
}

func TestLicenseSessionsSyncReleasesUnused(t *testing.T) {
	sessions := NewLicenseSessions()
	available := func(string) int { return 2 }
	names := func(id string) string { return id }

	if result := sessions.Sync("cmp_1", "usr_1", "User 1", []string{"gsn_supplement_18"}, available, names); !result.Granted {
		t.Fatalf("expected initial acquire to succeed, got %+v", result)
	}
	if result := sessions.Sync("cmp_1", "usr_1", "User 1", []string{"fgis_alrosa_q2_2026"}, available, names); !result.Granted {
		t.Fatalf("expected switch acquire to succeed, got %+v", result)
	}
	if sessions.countActiveLocked("cmp_1", "gsn_supplement_18") != 0 {
		t.Fatalf("expected gsn session to be released after switch")
	}
	if sessions.countActiveLocked("cmp_1", "fgis_alrosa_q2_2026") != 1 {
		t.Fatalf("expected fgis session to remain active")
	}
}
