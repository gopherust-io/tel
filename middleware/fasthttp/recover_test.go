package fasthttp_test

import (
	"bytes"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"

	"github.com/gopherust-io/tel"
	telfasthttp "github.com/gopherust-io/tel/middleware/fasthttp"
)

func TestRecoverReturns500(t *testing.T) {
	var buf bytes.Buffer
	tel.SetLogger(zerolog.New(&buf).With().Timestamp().Logger())

	h := telfasthttp.Recover(func(_ *fasthttp.RequestCtx) {
		panic("boom")
	})

	var ctx fasthttp.RequestCtx
	ctx.Request.SetRequestURI("/api/x")
	ctx.Request.Header.SetMethod(fasthttp.MethodGet)
	require.NotPanics(t, func() { h(&ctx) })
	assert.Equal(t, fasthttp.StatusInternalServerError, ctx.Response.StatusCode())
	assert.Equal(t, "internal error", string(ctx.Response.Body()))
	assert.Contains(t, buf.String(), "http handler panic")
}

func TestRecoverOutermostAroundServer(t *testing.T) {
	h := telfasthttp.Recover(telfasthttp.Server(func(_ *fasthttp.RequestCtx) {
		panic("inner")
	}))

	var ctx fasthttp.RequestCtx
	ctx.Request.SetRequestURI("/api/x")
	ctx.Request.Header.SetMethod(fasthttp.MethodGet)
	require.NotPanics(t, func() { h(&ctx) })
	assert.Equal(t, fasthttp.StatusInternalServerError, ctx.Response.StatusCode())
}
