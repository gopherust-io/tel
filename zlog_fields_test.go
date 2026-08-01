package tel

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCtxWithValidSpanIncludesTraceFields(t *testing.T) {
	prev := zerolog.GlobalLevel()
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	t.Cleanup(func() { zerolog.SetGlobalLevel(prev) })

	var buf bytes.Buffer
	SetLogger(zerolog.New(&buf).With().Timestamp().Caller().Logger())

	provider, _ := testTracerProvider(t)
	tel := NewWithTracerProvider(DefaultDebugConfig(), provider)

	ctx, span := tel.StartSpan(context.Background(), "op")
	defer span.End()

	InfoCtx(ctx).Msg("correlated")

	m := logFields(t, buf.String())
	sc := span.SpanContext()
	assert.Equal(t, sc.TraceID().String(), m[FieldTraceID])
	assert.Equal(t, sc.SpanID().String(), m[FieldSpanID])
	assert.NotContains(t, buf.String(), `"`+FieldTraceID+`":""`)
}

func TestCtxWithoutSpanOmitsTraceFields(t *testing.T) {
	prev := zerolog.GlobalLevel()
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	t.Cleanup(func() { zerolog.SetGlobalLevel(prev) })

	var buf bytes.Buffer
	SetLogger(zerolog.New(&buf).With().Timestamp().Logger())

	InfoCtx(context.Background()).Msg("no-span")

	m := logFields(t, buf.String())
	_, hasTrace := m[FieldTraceID]
	_, hasSpan := m[FieldSpanID]
	assert.False(t, hasTrace)
	assert.False(t, hasSpan)
}

func TestStartSpanContextLoggerHasTraceIDs(t *testing.T) {
	prev := zerolog.GlobalLevel()
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	t.Cleanup(func() { zerolog.SetGlobalLevel(prev) })

	var buf bytes.Buffer
	SetLogger(zerolog.New(&buf).With().Timestamp().Logger())

	provider, _ := testTracerProvider(t)
	tel := NewWithTracerProvider(DefaultDebugConfig(), provider)

	ctx, span := tel.StartSpan(context.Background(), "stored")
	defer span.End()

	zerolog.Ctx(ctx).Info().Msg("via-zerolog-ctx")

	m := logFields(t, buf.String())
	sc := span.SpanContext()
	assert.Equal(t, sc.TraceID().String(), m[FieldTraceID])
	assert.Equal(t, sc.SpanID().String(), m[FieldSpanID])
}

func TestConfigureLoggerAttachesServiceFields(t *testing.T) {
	prev := zerolog.GlobalLevel()
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	t.Cleanup(func() { zerolog.SetGlobalLevel(prev) })

	ConfigureLogger(Config{
		Service:     "orders-api",
		Pod:         "orders-api-7d9f8b-xyz",
		Namespace:   "prod-ns",
		Environment: "staging",
		Version:     "1.2.3",
		LogLevel:    "debug",
		LogEncode:   "json",
	})

	var buf bytes.Buffer
	// Re-bind output while keeping service fields from ConfigureLogger.
	SetLogger(Logger().Output(&buf))

	Info().Msg("boot")

	m := logFields(t, buf.String())
	assert.Equal(t, "orders-api", m[FieldService])
	assert.Equal(t, "orders-api-7d9f8b-xyz", m[FieldPod])
	assert.Equal(t, "prod-ns", m[FieldNamespace])
	assert.Equal(t, "staging", m[FieldEnvironment])
	assert.Equal(t, "1.2.3", m[FieldVersion])
}

func TestDurationMsUnderOneSecond(t *testing.T) {
	prev := zerolog.GlobalLevel()
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	t.Cleanup(func() { zerolog.SetGlobalLevel(prev) })

	var buf bytes.Buffer
	SetLogger(zerolog.New(&buf).With().Timestamp().Logger())

	Duration(Info(), 250*time.Millisecond).Msg("fast")

	m := logFields(t, buf.String())
	assert.Equal(t, float64(250), m[FieldDurationMs])
	_, hasS := m[FieldDurationS]
	assert.False(t, hasS)
}

func TestDurationSecondsAtOneSecond(t *testing.T) {
	prev := zerolog.GlobalLevel()
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	t.Cleanup(func() { zerolog.SetGlobalLevel(prev) })

	var buf bytes.Buffer
	SetLogger(zerolog.New(&buf).With().Timestamp().Logger())

	Duration(Info(), time.Second).Msg("slow")

	m := logFields(t, buf.String())
	assert.Equal(t, 1.0, m[FieldDurationS])
	_, hasMs := m[FieldDurationMs]
	assert.False(t, hasMs)
}

func TestFuncField(t *testing.T) {
	prev := zerolog.GlobalLevel()
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	t.Cleanup(func() { zerolog.SetGlobalLevel(prev) })

	var buf bytes.Buffer
	SetLogger(zerolog.New(&buf).With().Timestamp().Logger())

	Func(Info(), "HandleOrder").Msg("done")

	m := logFields(t, buf.String())
	assert.Equal(t, "HandleOrder", m[FieldFunction])
}

func TestTraceFuncSuccess(t *testing.T) {
	prev := zerolog.GlobalLevel()
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	t.Cleanup(func() { zerolog.SetGlobalLevel(prev) })

	var buf bytes.Buffer
	SetLogger(zerolog.New(&buf).With().Timestamp().Caller().Logger())

	provider, sr := testTracerProvider(t)
	tel := NewWithTracerProvider(DefaultDebugConfig(), provider)
	SetGlobal(tel)
	t.Cleanup(func() { SetGlobal(NewWithConfig(DefaultDebugConfig())) })

	err := TraceFunc(context.Background(), "doWork", func(context.Context) error {
		return nil
	})
	require.NoError(t, err)
	require.Len(t, sr.Ended(), 1)

	m := logFields(t, buf.String())
	assert.Equal(t, "doWork", m[FieldFunction])
	assert.Equal(t, "doWork", m["message"])
	_, hasMs := m[FieldDurationMs]
	_, hasS := m[FieldDurationS]
	assert.True(t, hasMs || hasS)
	assert.NotEmpty(t, m[FieldTraceID])
	assert.NotEmpty(t, m[FieldSpanID])
}

func TestTraceFuncError(t *testing.T) {
	prev := zerolog.GlobalLevel()
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	t.Cleanup(func() { zerolog.SetGlobalLevel(prev) })

	var buf bytes.Buffer
	SetLogger(zerolog.New(&buf).With().Timestamp().Logger())

	provider, _ := testTracerProvider(t)
	tel := NewWithTracerProvider(DefaultDebugConfig(), provider)
	SetGlobal(tel)
	t.Cleanup(func() { SetGlobal(NewWithConfig(DefaultDebugConfig())) })

	boom := errors.New("boom")
	err := TraceFunc(context.Background(), "failOp", func(context.Context) error {
		return boom
	})
	require.ErrorIs(t, err, boom)

	m := logFields(t, buf.String())
	assert.Equal(t, "error", m["level"])
	assert.Equal(t, "failOp", m[FieldFunction])
	assert.Equal(t, "boom", m["error"])
}

func TestInfoCtxCallerSkipsHelper(t *testing.T) {
	prev := zerolog.GlobalLevel()
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	t.Cleanup(func() { zerolog.SetGlobalLevel(prev) })

	var buf bytes.Buffer
	SetLogger(zerolog.New(&buf).With().Timestamp().Caller().Logger())

	InfoCtx(context.Background()).Msg("site")

	caller := callerFromLog(t, buf.String())
	assert.Contains(t, caller, "TestInfoCtxCallerSkipsHelper")
	assert.NotContains(t, caller, "tel.InfoCtx:")
}
