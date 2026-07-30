package observability

import (
	neturl "net/url"
	"strings"
)

// RedactDatabaseURL masks password fields in PostgreSQL connection strings.
func RedactDatabaseURL(raw string) string {
	if parsed, err := neturl.Parse(raw); err == nil && parsed.User != nil {
		if _, hasPassword := parsed.User.Password(); hasPassword {
			parsed.User = neturl.UserPassword(parsed.User.Username(), "REDACTED")
			return parsed.String()
		}
	}

	parts := strings.Fields(raw)
	for i, part := range parts {
		if strings.HasPrefix(strings.ToLower(part), "password=") {
			parts[i] = "password=REDACTED"
		}
	}
	return strings.Join(parts, " ")
}
