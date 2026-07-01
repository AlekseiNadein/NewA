package observability

import "strings"

// RedactDatabaseURL masks password fields in PostgreSQL connection strings.
func RedactDatabaseURL(url string) string {
	parts := strings.Fields(url)
	for i, part := range parts {
		if strings.HasPrefix(part, "password=") {
			parts[i] = "password=***"
		}
	}
	return strings.Join(parts, " ")
}
