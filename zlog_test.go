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

func TestInitLoggerConsoleLevelColors(t *testing.T) {
	InitLogger(LoggerOptions{JSON: false, Level: "debug"})

	assert.Equal(t, 34, zerolog.LevelColors[zerolog.DebugLevel], "debug")
	assert.Equal(t, 32, zerolog.LevelColors[zerolog.InfoLevel], "info")
	assert.Equal(t, 33, zerolog.LevelColors[zerolog.WarnLevel], "warn")
	assert.Equal(t, 31, zerolog.LevelColors[zerolog.ErrorLevel], "error")
	assert.Equal(t, 35, zerolog.LevelColors[zerolog.FatalLevel], "fatal")
	assert.Equal(t, 35, zerolog.LevelColors[zerolog.PanicLevel], "panic")
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

func TestLoggerAndCtx(t *testing.T) {
	var buf bytes.Buffer
	SetLogger(zerolog.New(&buf).With().Timestamp().Logger())

	got := Logger()
	got.Info().Msg("via-logger")
	assert.Contains(t, buf.String(), "via-logger")

	ctxLog := Ctx(t.Context())
	require.NotNil(t, ctxLog)
}

func TestInitLoggerInvalidLevelDefaultsToInfo(t *testing.T) {
	InitLogger(LoggerOptions{JSON: true, Level: "not-a-level"})
	require.Equal(t, zerolog.InfoLevel, zerolog.GlobalLevel())
}

func TestFatalErrAndMsgf(t *testing.T) {
	prev := zerolog.GlobalLevel()
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	t.Cleanup(func() { zerolog.SetGlobalLevel(prev) })

	var buf bytes.Buffer
	SetLogger(zerolog.New(&buf).With().Timestamp().Caller().Stack().Logger())

	var exited atomic.Int32
	SetExitFunc(func(code int) { exited.Store(int32(code)) })
	t.Cleanup(func() { SetExitFunc(os.Exit) })

	Fatal().Err(errors.New("boom")).Msgf("fatal-%d", 7)

	require.Equal(t, int32(1), exited.Load())
	out := buf.String()
	assert.Contains(t, out, "fatal-7")
	assert.Contains(t, out, "boom")
}

func TestErrorStackFrameShape(t *testing.T) {
	prev := zerolog.GlobalLevel()
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	t.Cleanup(func() { zerolog.SetGlobalLevel(prev) })

	var buf bytes.Buffer
	SetLogger(zerolog.New(&buf).With().Timestamp().Caller().Stack().Logger())

	Error().Err(errors.New("x")).Msg("fail")

	m := logFields(t, buf.String())
	stack, ok := m["stack"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, stack)

	frame, ok := stack[0].(map[string]any)
	require.True(t, ok, "stack frame should be an object")
	_, hasFunc := frame["func"]
	_, hasSource := frame["source"]
	_, hasLine := frame["line"]
	assert.True(t, hasFunc, "func")
	assert.True(t, hasSource, "source")
	assert.True(t, hasLine, "line")
}

func TestMarshalErrorStackNil(t *testing.T) {
	assert.Nil(t, marshalErrorStack(nil))
}

func TestFramesFromPCsEmpty(t *testing.T) {
	assert.Nil(t, framesFromPCs(nil))
	assert.Nil(t, framesFromPCs([]uintptr{}))
}

func TestSkipStackFrame(t *testing.T) {
	assert.True(t, skipStackFrame("github.com/rs/zerolog.(*Event).Msg"))
	assert.True(t, skipStackFrame("github.com/gopherust-io/tel.marshalErrorStack"))
	assert.True(t, skipStackFrame("github.com/gopherust-io/tel.captureRuntimeStack"))
	assert.True(t, skipStackFrame("github.com/gopherust-io/tel.stackFromTracer"))
	assert.True(t, skipStackFrame("github.com/gopherust-io/tel.framesFromPCs"))
	assert.True(t, skipStackFrame("github.com/gopherust-io/tel.(*FatalEvent).Err"))
	assert.False(t, skipStackFrame("github.com/gopherust-io/tel.TestSkipStackFrame"))
}

func TestApplyLoggerFromConfigJSON(t *testing.T) {
	ConfigureLogger(Config{LogLevel: "error", LogEncode: "json"})
	require.Equal(t, zerolog.ErrorLevel, zerolog.GlobalLevel())
}

func TestWarnEmits(t *testing.T) {
	prev := zerolog.GlobalLevel()
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	t.Cleanup(func() { zerolog.SetGlobalLevel(prev) })

	var buf bytes.Buffer
	SetLogger(zerolog.New(&buf).With().Timestamp().Logger())
	Warn().Msg("caution")
	assert.Contains(t, buf.String(), "caution")
}
