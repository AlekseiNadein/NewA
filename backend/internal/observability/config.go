package observability

import (
	"os"
	"strings"
)

// Config controls logging and metrics for a single process.
type Config struct {
	ServiceName string
	LogLevel    string
	LogFormat   string
	LogFile     string
	MetricsAddr string
}

func ConfigFromEnv(serviceName string) Config {
	return Config{
		ServiceName: strings.TrimSpace(envOr("APP_SERVICE_NAME", serviceName)),
		LogLevel:    strings.ToLower(strings.TrimSpace(envOr("APP_LOG_LEVEL", "info"))),
		LogFormat:   strings.ToLower(strings.TrimSpace(envOr("APP_LOG_FORMAT", "text"))),
		LogFile:     strings.TrimSpace(os.Getenv("APP_LOG_FILE")),
		MetricsAddr: strings.TrimSpace(os.Getenv("APP_METRICS_ADDR")),
	}
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
