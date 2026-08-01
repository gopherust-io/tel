package tel

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func TestParseTraceSampler(t *testing.T) {
	tests := []struct {
		spec string
		want string
	}{
		{"always", "AlwaysOnSampler"},
		{"never", "AlwaysOffSampler"},
		{"traceidratio:0.5", "TraceIDRatioBased{0.5}"},
		{"statustraceidratio:0.1", "StatusTraceIDRatioBased{0.1}"},
		{"parentbased_always", "ParentBased{root:AlwaysOnSampler"},
		{"", "ParentBased{root:StatusTraceIDRatioBased{0.1}"},
	}
	for _, tt := range tests {
		s := parseTraceSampler(tt.spec)
		assert.Contains(t, s.Description(), tt.want, "spec=%q", tt.spec)
	}
}

func TestStatusTraceIDRatioForceSampleOnErrorAttr(t *testing.T) {
	s := StatusTraceIDRatioBased(0) // never by ratio
	res := s.ShouldSample(sdktrace.SamplingParameters{
		Attributes: []attribute.KeyValue{attribute.Bool("error", true)},
	})
	assert.Equal(t, sdktrace.RecordAndSample, res.Decision)
}

func TestStatusTraceIDRatioDropsWithoutError(t *testing.T) {
	s := StatusTraceIDRatioBased(0)
	res := s.ShouldSample(sdktrace.SamplingParameters{
		Attributes: []attribute.KeyValue{attribute.String("http.method", "GET")},
	})
	assert.Equal(t, sdktrace.Drop, res.Decision)
}

func TestStatusTraceIDRatioForceSampleOnLinkError(t *testing.T) {
	s := StatusTraceIDRatioBased(0)
	res := s.ShouldSample(sdktrace.SamplingParameters{
		Links: []trace.Link{{
			Attributes: []attribute.KeyValue{attribute.String("error", "boom")},
		}},
	})
	assert.Equal(t, sdktrace.RecordAndSample, res.Decision)
}

func TestDefaultDebugConfigSamplerAlways(t *testing.T) {
	cfg := DefaultDebugConfig()
	require.Equal(t, "always", cfg.TelConfig.Traces.Sampler)
}

func BenchmarkStatusSamplerDrop(b *testing.B) {
	s := StatusTraceIDRatioBased(0)
	p := sdktrace.SamplingParameters{
		Attributes: []attribute.KeyValue{attribute.String("k", "v")},
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = s.ShouldSample(p)
	}
}

func BenchmarkStatusSamplerForce(b *testing.B) {
	s := StatusTraceIDRatioBased(0)
	p := sdktrace.SamplingParameters{
		Attributes: []attribute.KeyValue{attribute.Bool("error", true)},
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = s.ShouldSample(p)
	}
}

func TestParseSamplerFraction(t *testing.T) {
	assert.Equal(t, 0.25, parseSamplerFraction("traceidratio:0.25"))
	assert.Equal(t, 0.0, parseSamplerFraction("traceidratio:"))
	assert.Equal(t, 1.0, parseSamplerFraction("traceidratio:2"))
	_ = context.Background()
}
