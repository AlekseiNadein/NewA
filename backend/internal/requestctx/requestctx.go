package requestctx

import "context"

type key struct{}

// WithRequestID stores a request identifier in the context.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	if requestID == "" {
		return ctx
	}
	return context.WithValue(ctx, key{}, requestID)
}

// RequestID returns the request identifier from the context, if any.
func RequestID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(key{}).(string)
	return value
}
