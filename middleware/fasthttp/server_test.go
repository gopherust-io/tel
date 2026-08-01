package fasthttp_test

import (
	"bytes"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/gopherust-io/tel"
	telfasthttp "github.com/gopherust-io/tel/middleware/fasthttp"
)

func TestServerLogsRequestAndCreatesSpan(t *testing.T) {
	prev := zerolog.GlobalLevel()
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	t.Cleanup(func() { zerolog.SetGlobalLevel(prev) })

	var buf bytes.Buffer
	tel.SetLogger(zerolog.New(&buf).With().Timestamp().Logger())

	sr := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(sr),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	otel.SetTracerProvider(provider)
	tel.SetGlobal(tel.NewWithTracerProvider(tel.DefaultDebugConfig(), provider))
	t.Cleanup(func() { tel.SetGlobal(tel.NewWithConfig(tel.DefaultDebugConfig())) })

	h := telfasthttp.Server(func(ctx *fasthttp.RequestCtx) {
		ctx.SetStatusCode(fasthttp.StatusOK)
		_, _ = ctx.WriteString("ok")
	}, telfasthttp.WithLogSuccess(true))

	var ctx fasthttp.RequestCtx
	ctx.Request.SetRequestURI("/api/v1/x")
	ctx.Request.Header.SetMethod(fasthttp.MethodGet)
	h(&ctx)

	require.Equal(t, fasthttp.StatusOK, ctx.Response.StatusCode())
	require.Len(t, sr.Ended(), 1)
	assert.Equal(t, "GET", sr.Ended()[0].Name())
	assert.Contains(t, buf.String(), `"component":"http"`)
	assert.Contains(t, buf.String(), `"status":200`)
}

func TestServerSkipPrefix(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	otel.SetTracerProvider(provider)
	tel.SetGlobal(tel.NewWithTracerProvider(tel.DefaultDebugConfig(), provider))
	t.Cleanup(func() { tel.SetGlobal(tel.NewWithConfig(tel.DefaultDebugConfig())) })

	called := false
	h := telfasthttp.Server(func(ctx *fasthttp.RequestCtx) {
		called = true
		ctx.SetStatusCode(204)
	}, telfasthttp.WithSkipPrefixes("/health"))

	var ctx fasthttp.RequestCtx
	ctx.Request.SetRequestURI("/healthz")
	ctx.Request.Header.SetMethod(fasthttp.MethodGet)
	h(&ctx)

	assert.True(t, called)
	assert.Empty(t, sr.Ended())
}

func BenchmarkServer(b *testing.B) {
	provider := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
	otel.SetTracerProvider(provider)
	tel.SetGlobal(tel.NewWithTracerProvider(tel.DefaultDebugConfig(), provider))

	var logBuf bytes.Buffer
	tel.SetLogger(zerolog.New(&logBuf).Level(zerolog.Disabled))

	h := telfasthttp.Server(func(ctx *fasthttp.RequestCtx) {
		ctx.SetStatusCode(200)
	}, telfasthttp.WithLogSuccess(false))

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var ctx fasthttp.RequestCtx
		ctx.Request.SetRequestURI("/x")
		ctx.Request.Header.SetMethod(fasthttp.MethodGet)
		h(&ctx)
	}
}
