package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	"nav-saas-mvp/backend/internal/requestctx"
)

// WithRequestID stores a request identifier in the context.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return requestctx.WithRequestID(ctx, requestID)
}

// RequestIDFromContext returns the request identifier, if any.
func RequestIDFromContext(ctx context.Context) string {
	return requestctx.RequestID(ctx)
}

// NewRequestID generates a new random request identifier.
func NewRequestID() string {
	var buf [16]byte
	_, _ = rand.Read(buf[:])
	return hex.EncodeToString(buf[:])
}
