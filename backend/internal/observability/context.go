package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
)

type contextKey string

const requestIDKey contextKey = "request_id"

// WithRequestID stores a request identifier in the context.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	if requestID == "" {
		return ctx
	}
	return context.WithValue(ctx, requestIDKey, requestID)
}

// RequestIDFromContext returns the request identifier, if any.
func RequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(requestIDKey).(string)
	return value
}

// NewRequestID generates a new random request identifier.
func NewRequestID() string {
	var buf [16]byte
	_, _ = rand.Read(buf[:])
	return hex.EncodeToString(buf[:])
}
