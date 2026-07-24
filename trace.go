package tel

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// Cached messaging attributes — avoid reconstructing constant KeyValues on every span.
var (
	messagingSystemAttr  = attribute.String("messaging.system", "nats")
	messagingOpPublish   = attribute.String("messaging.operation", "publish")
	messagingOpProcess   = attribute.String("messaging.operation", "process")
	messagingDestKey     = attribute.Key("messaging.destination")
)

// Tracer returns a named tracer from the configured provider.
func (t *Telemetry) Tracer(name string) trace.Tracer {
	if t.traceProvider == nil {
		return otel.Tracer(name)
	}

	return t.traceProvider.Tracer(name)
}

// StartSpan starts a span on the telemetry tracer provider.
// The caller is responsible for ending the returned span.
func (t *Telemetry) StartSpan(
	ctx context.Context,
	spanName string,
	opts ...trace.SpanStartOption,
) (context.Context, trace.Span) {
	tr := t.tracer
	if tr == nil {
		tr = t.Tracer(t.cfg.Service)
	}

	ctx, span := tr.Start(ctx, spanName, opts...) //nolint:spancheck // caller ends span

	return ctx, span //nolint:spancheck // caller ends span
}

func (t *Telemetry) refreshTracer() {
	if t.traceProvider == nil {
		t.tracer = otel.Tracer(t.cfg.Service)

		return
	}

	t.tracer = t.traceProvider.Tracer(t.cfg.Service)
}

// propagator returns the global text map propagator.
func propagator() propagation.TextMapPropagator {
	return otel.GetTextMapPropagator()
}

// NewWithTracerProvider wires a custom tracer provider (useful in tests and custom setups).
func NewWithTracerProvider(cfg Config, provider trace.TracerProvider) *Telemetry {
	tel := NewWithConfig(cfg)
	tel.traceProvider = provider
	tel.refreshTracer()
	tel.started.Store(true)

	return tel
}

// InjectContext writes trace context into a string header map.
func InjectContext(ctx context.Context, headers map[string][]string) map[string][]string {
	if headers == nil {
		headers = make(map[string][]string, 2)
	}

	propagator().Inject(ctx, headerCarrier(headers))

	return headers
}

// ExtractContext reads trace context from a string header map.
func ExtractContext(ctx context.Context, headers map[string][]string) context.Context {
	if len(headers) == 0 {
		return ctx
	}

	return propagator().Extract(ctx, headerCarrier(headers))
}

// EndSpan ends a span and records an error when present.
func EndSpan(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}

	span.End()
}

// MessagingSubject returns a messaging destination attribute for NATS spans.
func MessagingSubject(subject string) attribute.KeyValue {
	return messagingDestKey.String(subject)
}

func MessagingSystem() attribute.KeyValue {
	return messagingSystemAttr
}

func MessagingOperationPublish() attribute.KeyValue {
	return messagingOpPublish
}

func MessagingOperationProcess() attribute.KeyValue {
	return messagingOpProcess
}

type headerCarrier map[string][]string

func (c headerCarrier) Get(key string) string {
	vals := c[key]
	if len(vals) == 0 {
		return ""
	}

	return vals[0]
}

func (c headerCarrier) Set(key, value string) {
	if vals := c[key]; len(vals) == 1 {
		vals[0] = value

		return
	}
	if vals := c[key]; cap(vals) >= 1 {
		c[key] = append(vals[:0], value)

		return
	}

	c[key] = []string{value}
}

func (c headerCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}

	return keys
}
