package fasthttp

import (
	"runtime/debug"

	"github.com/valyala/fasthttp"

	"github.com/gopherust-io/tel"
)

// Recover wraps next so a panicking handler is logged and answered with 500.
// Compose as Recover(Server(handler)) so Recover is outermost.
func Recover(next fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			tel.Error().
				Str("component", "http").
				Any("panic", rec).
				Bytes("stack", debug.Stack()).
				Bytes("method", ctx.Method()).
				Bytes("path", ctx.Path()).
				Msg("http handler panic")
			ctx.SetStatusCode(fasthttp.StatusInternalServerError)
			if len(ctx.Response.Body()) == 0 {
				ctx.SetContentType("text/plain; charset=utf-8")
				_, _ = ctx.WriteString("internal error")
			}
		}()
		next(ctx)
	}
}
