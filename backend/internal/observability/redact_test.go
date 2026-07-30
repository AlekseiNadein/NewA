package observability

import (
	"strings"
	"testing"
)

func TestRedactDatabaseURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "URL",
			input: "postgres://nav:super-secret@postgres:5432/nav?sslmode=disable",
		},
		{
			name:  "keyword DSN",
			input: "user=nav password=super-secret dbname=nav sslmode=disable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RedactDatabaseURL(tt.input)
			if strings.Contains(got, "super-secret") {
				t.Fatalf("password was not redacted: %q", got)
			}
			if !strings.Contains(got, "REDACTED") {
				t.Fatalf("redaction marker is missing: %q", got)
			}
		})
	}
}
