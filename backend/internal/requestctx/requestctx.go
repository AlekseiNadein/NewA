package requestctx

import "context"

type requestIDKey struct{}
type traceParentKey struct{}

// WithRequestID stores a request identifier in the context.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	if requestID == "" {
		return ctx
	}
	return context.WithValue(ctx, requestIDKey{}, requestID)
}

// RequestID returns the request identifier from the context, if any.
func RequestID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(requestIDKey{}).(string)
	return value
}

// WithTraceParent stores a W3C traceparent value for async propagation.
func WithTraceParent(ctx context.Context, traceparent string) context.Context {
	if traceparent == "" {
		return ctx
	}
	return context.WithValue(ctx, traceParentKey{}, traceparent)
}

// TraceParent returns the stored W3C traceparent value, if any.
func TraceParent(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(traceParentKey{}).(string)
	return value
}
