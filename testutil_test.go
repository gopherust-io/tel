package tel

import (
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	traceapi "go.opentelemetry.io/otel/trace"
)

func testTracerProvider(tb testing.TB) (traceapi.TracerProvider, *tracetest.SpanRecorder) {
	tb.Helper()
	sr := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(
		trace.WithSpanProcessor(sr),
		trace.WithSampler(trace.AlwaysSample()),
	)
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}))
	tb.Cleanup(func() {
		_ = provider.Shutdown(tb.Context())
	})
	return provider, sr
}
