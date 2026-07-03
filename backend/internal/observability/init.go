package observability

import (
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
)

// Init configures structured logging, tracing, and optionally starts a Prometheus listener.
func Init(cfg Config) {
	handler := TraceLogHandler{Handler: buildLogHandler(cfg)}
	slog.SetDefault(slog.New(handler))

	shutdownTracer, err := InitTracer(cfg)
	if err != nil {
		slog.Error("failed to initialize tracing", "error", err)
	} else if cfg.OTLPEndpoint != "" {
		slog.Info("tracing initialized", "endpoint", cfg.OTLPEndpoint, "sample_ratio", cfg.TraceSampleRatio)
		_ = shutdownTracer // process exit does not flush traces in dev
	}

	if cfg.MetricsAddr == "" {
		slog.Info("observability initialized", "service", cfg.ServiceName, "metrics", "disabled")
		return
	}

	mux := http.NewServeMux()
	mux.Handle("/metrics", MetricsHandler())
	server := &http.Server{Addr: cfg.MetricsAddr, Handler: mux}

	go func() {
		slog.Info("metrics listener started", "addr", cfg.MetricsAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("metrics listener stopped", "error", err)
		}
	}()

	slog.Info("observability initialized", "service", cfg.ServiceName, "metrics", cfg.MetricsAddr)
}

func buildLogHandler(cfg Config) slog.Handler {
	level := parseLevel(cfg.LogLevel)
	opts := &slog.HandlerOptions{Level: level}

	var writer io.Writer = os.Stdout
	if cfg.LogFile != "" {
		file, err := os.OpenFile(cfg.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})).
				Error("failed to open log file", "path", cfg.LogFile, "error", err)
		} else {
			writer = io.MultiWriter(os.Stdout, file)
		}
	}

	if cfg.LogFormat == "json" {
		return slog.NewJSONHandler(writer, opts).WithAttrs([]slog.Attr{
			slog.String("service", cfg.ServiceName),
		})
	}
	return slog.NewTextHandler(writer, opts).WithAttrs([]slog.Attr{
		slog.String("service", cfg.ServiceName),
	})
}

func parseLevel(raw string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
