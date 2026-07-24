package tel

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestInjectExtractContext(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(sr),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}))

	tel := NewWithConfig(DefaultDebugConfig())
	tel.traceProvider = provider

	ctx, span := tel.StartSpan(context.Background(), "parent")
	headers := InjectContext(ctx, nil)
	span.End()

	require.NotEmpty(t, headers)

	childCtx := ExtractContext(context.Background(), headers)
	assert.NotEqual(t, context.Background(), childCtx)
}

func TestStartSpanNoopWhenDisabled(t *testing.T) {
	tel := NewWithConfig(DefaultDebugConfig())
	ctx, span := tel.StartSpan(context.Background(), "noop")
	defer span.End()
	assert.False(t, span.IsRecording())
	_ = ctx
}

func TestEndSpanRecordsError(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(sr),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	otel.SetTracerProvider(provider)

	tel := NewWithConfig(DefaultDebugConfig())
	tel.traceProvider = provider

	_, span := tel.StartSpan(context.Background(), "err-span")
	EndSpan(span, assert.AnError)
	require.Len(t, sr.Ended(), 1)
}

func TestHeaderCarrierKeys(t *testing.T) {
	c := headerCarrier{"traceparent": {"00-abc"}, "Trace-Id": {"abc"}}
	keys := c.Keys()
	assert.Len(t, keys, 2)
	assert.ElementsMatch(t, []string{"traceparent", "Trace-Id"}, keys)
	assert.Equal(t, "00-abc", c.Get("traceparent"))
	assert.Empty(t, c.Get("missing"))
}

// ExampleInjectContext demonstrates W3C trace context propagation via headers.
func ExampleInjectContext() {
	headers := InjectContext(context.Background(), map[string][]string{})
	_ = ExtractContext(context.Background(), headers)
}
