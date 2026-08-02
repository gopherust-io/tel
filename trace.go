package tel

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// Cached messaging attributes — avoid reconstructing constant KeyValues on every span.
// Keys follow OTel messaging semantic conventions used by gopherust-io/nats.
var (
	messagingSystemAttr   = attribute.String("messaging.system", "nats")
	messagingOpPublish    = attribute.String("messaging.operation", "publish")
	messagingOpProcess    = attribute.String("messaging.operation", "process")
	messagingOpRequest    = attribute.String("messaging.operation", "request")
	messagingOpReply      = attribute.String("messaging.operation", "reply")
	messagingDestKey      = attribute.Key("messaging.destination")
	messagingStreamKey    = attribute.Key("messaging.nats.stream")
	messagingConsumerKey  = attribute.Key("messaging.nats.consumer")
	messagingStreamSeqKey = attribute.Key("messaging.nats.stream_sequence")
	messagingDeliveryKey  = attribute.Key("messaging.nats.delivery_count")
)

func (t *Telemetry) Tracer(name string) trace.Tracer {
	t.mu.RLock()
	tp := t.traceProvider
	t.mu.RUnlock()

	if tp == nil {
		return otel.Tracer(name)
	}

	return tp.Tracer(name)
}

// StartSpan starts a span on the telemetry tracer provider.
// The caller is responsible for ending the returned span.
func (t *Telemetry) StartSpan(
	ctx context.Context,
	spanName string,
	opts ...trace.SpanStartOption,
) (context.Context, trace.Span) {
	t.mu.RLock()
	tr := t.tracer
	service := t.cfg.Service
	t.mu.RUnlock()

	if tr == nil {
		tr = t.Tracer(service)
	}

	ctx, span := tr.Start(ctx, spanName, opts...) //nolint:spancheck // caller ends span
	ctx = contextWithTraceLogger(ctx)

	return ctx, span //nolint:spancheck // caller ends span
}

func propagator() propagation.TextMapPropagator {
	return otel.GetTextMapPropagator()
}

// NewWithTracerProvider wires a custom tracer provider (useful in tests and custom setups).
func NewWithTracerProvider(cfg Config, provider trace.TracerProvider) *Telemetry {
	return NewWithProviders(cfg, nil, provider)
}

// NewWithProviders wires custom metric and/or tracer providers for tests and
// record-path benchmarks (e.g. sdkmetric.ManualReader). Nil providers keep the
// debug/noop defaults from NewWithConfig.
func NewWithProviders(cfg Config, mp metric.MeterProvider, tp trace.TracerProvider) *Telemetry {
	tel := NewWithConfig(cfg)
	tel.mu.Lock()
	if mp != nil {
		tel.metricProvider = mp
		tel.registry = newRegistryWithCache(
			mp.Meter(tel.cfg.Service),
			newAttrCache(defaultMaxCardinality),
			&tel.epoch,
			tel.epoch.Load(),
			maxInstrumentsFromCfg(tel.cfg),
		)
	}
	if tp != nil {
		tel.traceProvider = tp
		tel.refreshTracerLocked()
	}
	tel.mu.Unlock()
	tel.started.Store(true)

	return tel
}

func InjectContext(ctx context.Context, headers map[string][]string) map[string][]string {
	if headers == nil {
		headers = make(map[string][]string, 2)
	}

	propagator().Inject(ctx, headerCarrier(headers))

	return headers
}

func ExtractContext(ctx context.Context, headers map[string][]string) context.Context {
	if len(headers) == 0 {
		return ctx
	}

	return propagator().Extract(ctx, headerCarrier(headers))
}

// Inject writes W3C trace context into carrier.
func Inject(ctx context.Context, carrier propagation.TextMapCarrier) {
	propagator().Inject(ctx, carrier)
}

// Extract reads W3C trace context from carrier.
func Extract(ctx context.Context, carrier propagation.TextMapCarrier) context.Context {
	return propagator().Extract(ctx, carrier)
}

func EndSpan(span trace.Span, err error) {
	if span == nil {
		return
	}

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}

	span.End()
}

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

func MessagingOperationRequest() attribute.KeyValue {
	return messagingOpRequest
}

func MessagingOperationReply() attribute.KeyValue {
	return messagingOpReply
}

// MessagingStream is the JetStream stream name (bounded cardinality).
func MessagingStream(stream string) attribute.KeyValue {
	return messagingStreamKey.String(stream)
}

// MessagingConsumer is the JetStream consumer/durable name (bounded cardinality).
func MessagingConsumer(consumer string) attribute.KeyValue {
	return messagingConsumerKey.String(consumer)
}

// MessagingStreamSequence is the stream sequence from JetStream ack metadata.
func MessagingStreamSequence(seq int64) attribute.KeyValue {
	return messagingStreamSeqKey.Int64(seq)
}

// MessagingDeliveryCount is the redelivery count from JetStream ack metadata.
func MessagingDeliveryCount(n int64) attribute.KeyValue {
	return messagingDeliveryKey.Int64(n)
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
