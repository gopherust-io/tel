package tel

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetConfigFromEnvDefaults(t *testing.T) {
	t.Setenv("TEL_SERVICE_NAME", "orders-api")
	t.Setenv("TEL_ENABLE", "false")
	t.Setenv("MONITOR_ENABLE", "false")
	t.Setenv("LOG_LEVEL", "warn")
	t.Setenv("LOG_ENCODE", "console")
	t.Setenv("TEL_TRACES_ENABLE", "true")
	t.Setenv("TEL_TRACES_SAMPLER", "always")
	t.Setenv("POD_NAME", "orders-api-abc")
	t.Setenv("NAMESPACE", "prod")

	cfg, err := GetConfigFromEnv()
	require.NoError(t, err)
	assert.Equal(t, "orders-api", cfg.Service)
	assert.Equal(t, "orders-api-abc", cfg.Pod)
	assert.Equal(t, "prod", cfg.Namespace)
	assert.Equal(t, "warn", cfg.LogLevel)
	assert.Equal(t, "console", cfg.LogEncode)
	assert.False(t, cfg.TelConfig.Enable)
	assert.False(t, cfg.MonitorConfig.Enable)
	assert.True(t, cfg.TelConfig.Traces.Enable)
	assert.Equal(t, "always", cfg.TelConfig.Traces.Sampler)
}

func TestGetConfigFromEnvServiceFallback(t *testing.T) {
	t.Setenv("TEL_SERVICE_NAME", "")
	cfg, err := GetConfigFromEnv()
	require.NoError(t, err)
	assert.NotEmpty(t, cfg.Service)
}

func TestGetConfigFromEnvLoadsDotEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	require.NoError(t, os.WriteFile(path, []byte("TEL_SERVICE_NAME=from-dotenv\nLOG_LEVEL=debug\n"), 0o600))
	t.Setenv("TEL_DOTENV", path)
	// LoadDotEnv does not overwrite existing process env.
	require.NoError(t, os.Unsetenv("TEL_SERVICE_NAME"))
	require.NoError(t, os.Unsetenv("LOG_LEVEL"))

	cfg, err := GetConfigFromEnv()
	require.NoError(t, err)
	assert.Equal(t, "from-dotenv", cfg.Service)
	assert.Equal(t, "debug", cfg.LogLevel)
}

func TestGetConfigFromEnvMissingDotEnvOK(t *testing.T) {
	t.Setenv("TEL_DOTENV", filepath.Join(t.TempDir(), "missing.env"))
	t.Setenv("TEL_SERVICE_NAME", "ok-without-file")
	cfg, err := GetConfigFromEnv()
	require.NoError(t, err)
	assert.Equal(t, "ok-without-file", cfg.Service)
}

func TestInitWithConfigDisabled(t *testing.T) {
	cfg := DefaultDebugConfig()
	telem, shutdown, err := InitWithConfig(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, telem)
	require.NotNil(t, shutdown)
	assert.Equal(t, telem, Global())
	require.NoError(t, shutdown(context.Background()))
}
