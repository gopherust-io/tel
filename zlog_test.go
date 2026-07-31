package tel

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"regexp"
	"sync/atomic"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFatalUsesConfigurableExit(t *testing.T) {
	var buf bytes.Buffer
	SetLogger(zerolog.New(&buf).With().Timestamp().Caller().Stack().Logger())

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
	SetLogger(zerolog.New(&buf).With().Timestamp().Caller().Stack().Logger())

	Error().Str("component", "test").Msg("recoverable")
	assert.Equal(t, int32(0), exited.Load(), "Error() should not exit")
}

func TestInitLoggerRespectsLevel(t *testing.T) {
	var buf bytes.Buffer
	SetLogger(zerolog.New(&buf).With().Timestamp().Caller().Stack().Logger())

	InitLogger(LoggerOptions{JSON: true, Level: "error"})
	Debug().Msg("hidden")
	// InitLogger replaces the logger; disabled events must not write.
	assert.Empty(t, buf.String(), "debug should be suppressed at error level")
}

func TestApplyLoggerFromConfigConsole(t *testing.T) {
	ConfigureLogger(Config{LogLevel: "warn", LogEncode: "console"})
	require.Equal(t, zerolog.WarnLevel, zerolog.GlobalLevel())
}

var callerFieldRE = regexp.MustCompile(`^[A-Za-z0-9_.*\(\)]+:\d+$`)

func callerFromLog(t *testing.T, line string) string {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(line), &m))
	caller, ok := m["caller"].(string)
	require.True(t, ok, "caller field missing in %s", line)
	require.True(t, callerFieldRE.MatchString(caller), "caller %q", caller)
	return caller
}

func TestInfoIncludesCaller(t *testing.T) {
	prev := zerolog.GlobalLevel()
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	t.Cleanup(func() { zerolog.SetGlobalLevel(prev) })

	var buf bytes.Buffer
	SetLogger(zerolog.New(&buf).With().Timestamp().Caller().Stack().Logger())

	Info().Msg("hello")

	caller := callerFromLog(t, buf.String())
	assert.Contains(t, caller, "TestInfoIncludesCaller")
	assert.NotContains(t, caller, "tel.Info:")
}

func TestFatalIncludesCaller(t *testing.T) {
	prev := zerolog.GlobalLevel()
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	t.Cleanup(func() { zerolog.SetGlobalLevel(prev) })

	var buf bytes.Buffer
	SetLogger(zerolog.New(&buf).With().Timestamp().Caller().Stack().Logger())

	var exited atomic.Int32
	SetExitFunc(func(code int) { exited.Store(int32(code)) })
	t.Cleanup(func() { SetExitFunc(os.Exit) })

	Fatal().Msg("boom")

	require.Equal(t, int32(1), exited.Load())
	caller := callerFromLog(t, buf.String())
	assert.Contains(t, caller, "TestFatalIncludesCaller")
	assert.NotContains(t, caller, "FatalEvent")
}

func logFields(t *testing.T, line string) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(line), &m))

	return m
}

func TestErrorErrIncludesStack(t *testing.T) {
	prev := zerolog.GlobalLevel()
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	t.Cleanup(func() { zerolog.SetGlobalLevel(prev) })

	var buf bytes.Buffer
	SetLogger(zerolog.New(&buf).With().Timestamp().Caller().Stack().Logger())

	Error().Err(errors.New("x")).Msg("fail")

	m := logFields(t, buf.String())
	stack, ok := m["stack"].([]any)
	require.True(t, ok, "stack field missing in %s", buf.String())
	require.NotEmpty(t, stack)

	raw, err := json.Marshal(stack)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "TestErrorErrIncludesStack")
}

func TestInfoWithoutErrHasNoStack(t *testing.T) {
	prev := zerolog.GlobalLevel()
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	t.Cleanup(func() { zerolog.SetGlobalLevel(prev) })

	var buf bytes.Buffer
	SetLogger(zerolog.New(&buf).With().Timestamp().Caller().Stack().Logger())

	Info().Msg("hello")

	m := logFields(t, buf.String())
	_, hasStack := m["stack"]
	assert.False(t, hasStack, "stack should be absent without Err: %s", buf.String())
}
