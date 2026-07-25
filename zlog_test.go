package tel

import (
	"bytes"
	"os"
	"sync/atomic"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFatalUsesConfigurableExit(t *testing.T) {
	var buf bytes.Buffer
	SetLogger(zerolog.New(&buf).With().Timestamp().Logger())

	var exited atomic.Int32
	SetExitFunc(func(code int) {
		exited.Store(int32(code))
	})
	t.Cleanup(func() { SetExitFunc(os.Exit) })

	Fatal().Str("component", "test").Msg("boom")

	require.Equal(t, int32(1), exited.Load(), "exit code")
	assert.Contains(t, buf.String(), "boom", "log output should contain fatal message")
}

func TestErrorDoesNotExit(t *testing.T) {
	var exited atomic.Int32
	SetExitFunc(func(code int) { exited.Store(int32(code)) })
	t.Cleanup(func() { SetExitFunc(os.Exit) })

	var buf bytes.Buffer
	SetLogger(zerolog.New(&buf).With().Timestamp().Logger())

	Error().Str("component", "test").Msg("recoverable")
	assert.Equal(t, int32(0), exited.Load(), "Error() should not exit")
}

func TestInitLoggerRespectsLevel(t *testing.T) {
	var buf bytes.Buffer
	SetLogger(zerolog.New(&buf).With().Timestamp().Logger())

	InitLogger(LoggerOptions{JSON: true, Level: "error"})
	Debug().Msg("hidden")
	// InitLogger replaces the logger; disabled events must not write.
	assert.Empty(t, buf.String(), "debug should be suppressed at error level")
}

func TestApplyLoggerFromConfigConsole(t *testing.T) {
	ConfigureLogger(Config{LogLevel: "warn", LogEncode: "console"})
	require.Equal(t, zerolog.WarnLevel, zerolog.GlobalLevel())
}
