package tel

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWrapContextFromCtx(t *testing.T) {
	cfg := DefaultDebugConfig()
	tel := NewWithConfig(cfg)

	ctx := WrapContext(context.Background(), tel)
	got := FromCtx(ctx)
	assert.Same(t, tel, got)
}

func TestFromCtxFallbackUsesGlobal(t *testing.T) {
	cfg := DefaultDebugConfig()
	tel := NewWithConfig(cfg)
	SetGlobal(tel)

	got := FromCtx(context.Background())
	assert.Equal(t, tel.cfg.Service, got.cfg.Service)
}

func TestSetGlobal(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Service = "custom-service"
	tel := NewWithConfig(cfg)
	SetGlobal(tel)

	got := Global()
	assert.Equal(t, "custom-service", got.cfg.Service)
}

func TestTelemetryStartShutdownNoop(t *testing.T) {
	cfg := DefaultDebugConfig()
	cfg.TelConfig.Enable = false
	cfg.MonitorConfig.Enable = false

	tel := NewWithConfig(cfg)
	require.NoError(t, tel.Start(context.Background()))
	require.NoError(t, tel.Shutdown(context.Background()))
	require.False(t, tel.started.Load())
}

func TestTelemetryStartIdempotent(t *testing.T) {
	cfg := DefaultDebugConfig()
	cfg.TelConfig.Enable = false
	cfg.MonitorConfig.Enable = false

	tel := NewWithConfig(cfg)
	require.NoError(t, tel.Start(context.Background()))
	require.NoError(t, tel.Start(context.Background()))
	require.NoError(t, tel.Shutdown(context.Background()))
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	assert.NotEmpty(t, cfg.Service)
	assert.Equal(t, "dev", cfg.Version)
	assert.True(t, cfg.MonitorConfig.Enable)
	assert.True(t, cfg.TelConfig.Enable)
}

func TestDefaultDebugConfig(t *testing.T) {
	cfg := DefaultDebugConfig()
	assert.True(t, cfg.Debug)
	assert.False(t, cfg.MonitorConfig.Enable)
	assert.False(t, cfg.TelConfig.Enable)
}

func TestTelemetryRegistry(t *testing.T) {
	cfg := DefaultDebugConfig()
	tel := NewWithConfig(cfg)
	registry := tel.Registry()
	require.NotNil(t, registry)
}

func TestTelemetryCopy(t *testing.T) {
	cfg := DefaultDebugConfig()
	tel := NewWithConfig(cfg)
	telCopy := tel.copy()
	assert.Equal(t, tel.cfg.Service, telCopy.cfg.Service)
}

func TestTelemetryConfig(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Service = "orders"
	tel := NewWithConfig(cfg)
	assert.Equal(t, "orders", tel.Config().Service)
}

func TestTelemetryShutdownWithoutStart(t *testing.T) {
	tel := NewWithConfig(DefaultDebugConfig())
	require.NoError(t, tel.Shutdown(context.Background()))
}
