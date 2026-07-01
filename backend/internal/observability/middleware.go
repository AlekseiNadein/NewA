package observability

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

// WrapHTTP adds request IDs, access logging, and HTTP metrics.
func WrapHTTP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}

		requestID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if requestID == "" {
			requestID = NewRequestID()
		}
		w.Header().Set("X-Request-ID", requestID)

		ctx := WithRequestID(r.Context(), requestID)
		r = r.WithContext(ctx)

		started := time.Now()
		capture := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(capture, r)
		duration := time.Since(started)

		route := r.Pattern
		if route == "" {
			route = normalizeRoute(r.URL.Path)
		}

		if shouldRecordHTTP(route, r.URL.Path) {
			RecordHTTPRequest(r.Method, route, strconv.Itoa(capture.status), duration.Seconds())
		}

		if shouldAccessLog(r.URL.Path) {
			slog.InfoContext(ctx, "request completed",
				"method", r.Method,
				"path", r.URL.Path,
				"route", route,
				"status", capture.status,
				"duration_ms", duration.Milliseconds(),
				"request_id", requestID,
			)
		}
	})
}

func shouldAccessLog(path string) bool {
	return strings.HasPrefix(path, "/api/")
}

func shouldRecordHTTP(route, path string) bool {
	if path == "/metrics" {
		return false
	}
	return strings.HasPrefix(path, "/api/")
}

func normalizeRoute(path string) string {
	if path == "" || path == "/" {
		return "/"
	}
	segments := strings.Split(strings.Trim(path, "/"), "/")
	for i, segment := range segments {
		if segment == "" {
			continue
		}
		if looksLikeID(segment) {
			segments[i] = ":id"
		}
	}
	return "/" + strings.Join(segments, "/")
}

func looksLikeID(segment string) bool {
	if len(segment) >= 8 && strings.Contains(segment, "-") {
		return true
	}
	for _, r := range segment {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(segment) > 0
}
