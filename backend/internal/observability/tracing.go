package observability

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"

	"nav-saas-mvp/backend/internal/requestctx"
)

var tracer = otel.Tracer("nav")

// InitTracer configures OTLP trace export when endpoint is set.
func InitTracer(cfg Config) (func(context.Context) error, error) {
	endpoint := strings.TrimSpace(cfg.OTLPEndpoint)
	if endpoint == "" {
		return func(context.Context) error { return nil }, nil
	}

	opts := []otlptracehttp.Option{
		otlptracehttp.WithInsecure(),
	}
	if strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://") {
		opts = append(opts, otlptracehttp.WithEndpoint(strings.TrimPrefix(strings.TrimPrefix(endpoint, "https://"), "http://")))
	} else {
		opts = append(opts, otlptracehttp.WithEndpoint(endpoint))
	}

	exporter, err := otlptracehttp.New(context.Background(), opts...)
	if err != nil {
		return nil, err
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(cfg.ServiceName),
		),
	)
	if err != nil {
		return nil, err
	}

	ratio := cfg.TraceSampleRatio
	if ratio <= 0 {
		ratio = 1
	}
	if ratio > 1 {
		ratio = 1
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))),
	)
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return provider.Shutdown, nil
}

// WrapHTTPTracing instruments HTTP handlers with OpenTelemetry spans.
func WrapHTTPTracing(next http.Handler) http.Handler {
	return otelhttp.NewHandler(next, "http",
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			route := r.Pattern
			if route == "" {
				route = normalizeRoute(r.URL.Path)
			}
			return r.Method + " " + route
		}),
	)
}

// DetachCorrelation copies request_id and trace context into a background context.
func DetachCorrelation(ctx context.Context) context.Context {
	base := context.Background()
	if requestID := requestctx.RequestID(ctx); requestID != "" {
		base = requestctx.WithRequestID(base, requestID)
	}
	if traceparent := TraceParentFromContext(ctx); traceparent != "" {
		base = requestctx.WithTraceParent(base, traceparent)
		base = ContextWithTraceParent(base, traceparent)
	}
	if span := trace.SpanFromContext(ctx); span.SpanContext().IsValid() {
		base = trace.ContextWithSpan(base, span)
	}
	return base
}

// TraceParentFromContext serializes the active trace context as W3C traceparent.
func TraceParentFromContext(ctx context.Context) string {
	if tp := requestctx.TraceParent(ctx); tp != "" {
		return tp
	}
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	return carrier.Get("traceparent")
}

// ContextWithTraceParent restores trace context from a W3C traceparent header value.
func ContextWithTraceParent(ctx context.Context, traceparent string) context.Context {
	traceparent = strings.TrimSpace(traceparent)
	if traceparent == "" {
		return ctx
	}
	carrier := propagation.MapCarrier{"traceparent": traceparent}
	return otel.GetTextMapPropagator().Extract(ctx, carrier)
}

// StartSpan starts a child span and returns the derived context.
func StartSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return tracer.Start(ctx, name, trace.WithAttributes(attrs...))
}

// StartEstimateEnqueueSpan traces async estimate calc enqueue.
func StartEstimateEnqueueSpan(ctx context.Context, estimateID string) (context.Context, func()) {
	ctx, span := StartSpan(ctx, "estimate.calc.enqueue",
		attribute.String("estimate_id", estimateID),
	)
	return ctx, func() { span.End() }
}

// RunCalcSpan runs calc worker logic inside a span linked to the incoming trace.
func RunCalcSpan(ctx context.Context, estimateID, lineID, jobID, requestID, traceparent string, fn func(context.Context) error) error {
	ctx = ContextWithTraceParent(ctx, traceparent)
	if requestID != "" {
		ctx = requestctx.WithRequestID(ctx, requestID)
	}
	ctx, span := StartSpan(ctx, "calc.process",
		attribute.String("estimate_id", estimateID),
		attribute.String("line_id", lineID),
		attribute.String("job_id", jobID),
	)
	defer span.End()
	err := fn(ctx)
	if err != nil {
		span.RecordError(err)
	}
	return err
}

// InjectAMQPHeaders returns RabbitMQ headers carrying the trace context.
func InjectAMQPHeaders(ctx context.Context) map[string]any {
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	if len(carrier) == 0 {
		return nil
	}
	headers := make(map[string]any, len(carrier))
	for key, value := range carrier {
		headers[key] = value
	}
	return headers
}

// ExtractAMQPHeaders restores trace context from RabbitMQ message headers.
func ExtractAMQPHeaders(ctx context.Context, headers map[string]any) context.Context {
	if len(headers) == 0 {
		return ctx
	}
	carrier := propagation.MapCarrier{}
	for key, raw := range headers {
		if value, ok := raw.(string); ok && value != "" {
			carrier[key] = value
		}
	}
	if len(carrier) == 0 {
		return ctx
	}
	return otel.GetTextMapPropagator().Extract(ctx, carrier)
}

// TraceLogHandler adds trace_id and span_id fields to structured logs.
type TraceLogHandler struct {
	slog.Handler
}

func (h TraceLogHandler) Handle(ctx context.Context, record slog.Record) error {
	span := trace.SpanFromContext(ctx)
	if sc := span.SpanContext(); sc.IsValid() {
		record.AddAttrs(
			slog.String("trace_id", sc.TraceID().String()),
			slog.String("span_id", sc.SpanID().String()),
		)
	}
	return h.Handler.Handle(ctx, record)
}

func (h TraceLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return TraceLogHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h TraceLogHandler) WithGroup(name string) slog.Handler {
	return TraceLogHandler{Handler: h.Handler.WithGroup(name)}
}

func parseTraceSampleRatio(raw string) float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 1
	}
	var ratio float64
	if _, err := fmt.Sscanf(raw, "%f", &ratio); err != nil {
		return 1
	}
	if ratio <= 0 {
		return 0
	}
	if ratio > 1 {
		return 1
	}
	return ratio
}
