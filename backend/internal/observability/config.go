package observability

import (
	"os"
	"strings"
)

// Config controls logging, metrics, and tracing for a single process.
type Config struct {
	ServiceName      string
	LogLevel         string
	LogFormat        string
	LogFile          string
	MetricsAddr      string
	OTLPEndpoint     string
	TraceSampleRatio float64
}

func ConfigFromEnv(serviceName string) Config {
	otlpEndpoint := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	if otlpEndpoint == "" {
		otlpEndpoint = strings.TrimSpace(os.Getenv("APP_OTEL_EXPORTER_OTLP_ENDPOINT"))
	}
	return Config{
		ServiceName:      strings.TrimSpace(envOr("APP_SERVICE_NAME", serviceName)),
		LogLevel:         strings.ToLower(strings.TrimSpace(envOr("APP_LOG_LEVEL", "info"))),
		LogFormat:        strings.ToLower(strings.TrimSpace(envOr("APP_LOG_FORMAT", "text"))),
		LogFile:          strings.TrimSpace(os.Getenv("APP_LOG_FILE")),
		MetricsAddr:      strings.TrimSpace(os.Getenv("APP_METRICS_ADDR")),
		OTLPEndpoint:     otlpEndpoint,
		TraceSampleRatio: parseTraceSampleRatio(envOr("OTEL_TRACES_SAMPLER_ARG", envOr("APP_OTEL_TRACE_SAMPLE_RATIO", "1"))),
	}
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
