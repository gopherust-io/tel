package tel

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTracerProviderDisabled(t *testing.T) {
	cfg := DefaultDebugConfig()
	cfg.TelConfig.Traces.Enable = false
	provider, shutdown, err := newTracerProvider(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, provider)
	require.NotNil(t, shutdown)
	require.NoError(t, shutdown(context.Background()))
}

func TestNewTracerProviderShutdownIdempotent(t *testing.T) {
	cfg := DefaultDebugConfig()
	cfg.TelConfig.Traces.Enable = false
	_, shutdown, err := newTracerProvider(context.Background(), cfg)
	require.NoError(t, err)
	require.NoError(t, shutdown(context.Background()))
	require.NoError(t, shutdown(context.Background()))
}

func TestNewWithTracerProvider(t *testing.T) {
	provider, _ := testTracerProvider(t)
	tel := NewWithTracerProvider(DefaultDebugConfig(), provider)
	tr := tel.Tracer("test")
	require.NotNil(t, tr)
	_, span := tel.StartSpan(context.Background(), "test-span")
	require.NotNil(t, span)
	span.End()
}

func TestTelemetryStartShutdownWithTraceProvider(t *testing.T) {
	provider, sr := testTracerProvider(t)
	cfg := DefaultDebugConfig()
	tel := NewWithTracerProvider(cfg, provider)
	ctx := context.Background()
	_, span := tel.StartSpan(ctx, "lifecycle")
	span.End()
	require.NoError(t, tel.Shutdown(ctx))
	assert.NotEmpty(t, sr.Ended())
}

func TestSpanEndConcurrent(t *testing.T) {
	provider, _ := testTracerProvider(t)
	tel := NewWithTracerProvider(DefaultDebugConfig(), provider)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			_, span := tel.StartSpan(context.Background(), "concurrent")
			EndSpan(span, nil)
		})
	}
	wg.Wait()
}
