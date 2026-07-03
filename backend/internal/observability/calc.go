package observability

import (
	"context"
	"log/slog"
)

// CalcAttrs returns structured fields for estimate line calc logging.
func CalcAttrs(estimateID, lineID, code, jobID, requestID string) []any {
	attrs := make([]any, 0, 10)
	if requestID != "" {
		attrs = append(attrs, "request_id", requestID)
	}
	if jobID != "" {
		attrs = append(attrs, "job_id", jobID)
	}
	if estimateID != "" {
		attrs = append(attrs, "estimate_id", estimateID)
	}
	if lineID != "" {
		attrs = append(attrs, "line_id", lineID)
	}
	if code != "" {
		attrs = append(attrs, "code", code)
	}
	return attrs
}

// LogCalcInfo writes a calc-scoped info log entry.
func LogCalcInfo(ctx context.Context, msg string, estimateID, lineID, code, jobID, requestID string, extra ...any) {
	attrs := append(CalcAttrs(estimateID, lineID, code, jobID, requestID), extra...)
	slog.InfoContext(ctx, msg, attrs...)
}

// LogCalcWarn writes a calc-scoped warning log entry.
func LogCalcWarn(ctx context.Context, msg string, estimateID, lineID, code, jobID, requestID string, extra ...any) {
	attrs := append(CalcAttrs(estimateID, lineID, code, jobID, requestID), extra...)
	slog.WarnContext(ctx, msg, attrs...)
}
