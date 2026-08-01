package tel

import (
	"bytes"
	"context"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithFieldsMergeAndOverride(t *testing.T) {
	prev := zerolog.GlobalLevel()
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	t.Cleanup(func() { zerolog.SetGlobalLevel(prev) })

	var buf bytes.Buffer
	SetLogger(zerolog.New(&buf).With().Timestamp().Logger())

	ctx := WithFields(context.Background(), StrField("a", "1"), IntField("n", 7))
	ctx = WithFields(ctx, StrField("a", "2"), BoolField("ok", true))
	InfoCtx(ctx).Msg("fields")

	m := logFields(t, buf.String())
	assert.Equal(t, "2", m["a"])
	assert.Equal(t, float64(7), m["n"])
	assert.Equal(t, true, m["ok"])
}

func TestCtxEmptyBagNoExtraFields(t *testing.T) {
	prev := zerolog.GlobalLevel()
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	t.Cleanup(func() { zerolog.SetGlobalLevel(prev) })

	var buf bytes.Buffer
	SetLogger(zerolog.New(&buf).With().Timestamp().Logger())

	InfoCtx(context.Background()).Msg("plain")
	m := logFields(t, buf.String())
	_, hasTrace := m[FieldTraceID]
	assert.False(t, hasTrace)
}

func TestCtxFieldsWithSpan(t *testing.T) {
	prev := zerolog.GlobalLevel()
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	t.Cleanup(func() { zerolog.SetGlobalLevel(prev) })

	var buf bytes.Buffer
	SetLogger(zerolog.New(&buf).With().Timestamp().Logger())

	provider, _ := testTracerProvider(t)
	tel := NewWithTracerProvider(DefaultDebugConfig(), provider)
	ctx, span := tel.StartSpan(context.Background(), "op")
	defer span.End()
	ctx = WithFields(ctx, StrField("component", "api"))

	InfoCtx(ctx).Msg("combo")
	m := logFields(t, buf.String())
	require.NotEmpty(t, m[FieldTraceID])
	assert.Equal(t, "api", m["component"])
}

func BenchmarkCtxEmpty(b *testing.B) {
	ctx := context.Background()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = Ctx(ctx)
	}
}

func BenchmarkCtxWithFields(b *testing.B) {
	ctx := WithFields(context.Background(), StrField("a", "1"), IntField("n", 1))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = Ctx(ctx)
	}
}
