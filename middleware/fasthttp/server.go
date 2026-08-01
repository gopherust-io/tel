package fasthttp

import (
	"bytes"
	"context"
	"errors"
	"time"

	"github.com/valyala/fasthttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/gopherust-io/tel"
)

const ctxUserValueKey = "tel.ctx"

var (
	errHTTPServerError = errors.New("http server error")
	traceparentKey     = []byte("Traceparent")
	tracestateKey      = []byte("Tracestate")
	baggageKey         = []byte("Baggage")
	requestIDKey       = []byte("X-Request-Id")
)

// Option configures Server middleware.
type Option func(*config)

// goalign:ignore // trailing bool padding is unavoidable
type config struct {
	spanName   func(ctx *fasthttp.RequestCtx) string
	component  string
	skip       [][]byte
	logSuccess bool
}

// WithSpanName sets a custom span name (keep low-cardinality).
func WithSpanName(fn func(ctx *fasthttp.RequestCtx) string) Option {
	return func(c *config) { c.spanName = fn }
}

// WithSkipPrefixes skips tracing/logging when path has any of the byte prefixes.
func WithSkipPrefixes(prefixes ...string) Option {
	return func(c *config) {
		c.skip = make([][]byte, len(prefixes))
		for i, p := range prefixes {
			c.skip[i] = []byte(p)
		}
	}
}

// WithLogSuccess logs successful (status < 500) requests when true (default false).
func WithLogSuccess(v bool) Option {
	return func(c *config) { c.logSuccess = v }
}

var (
	methodGET    = []byte("GET")
	methodPOST   = []byte("POST")
	methodPUT    = []byte("PUT")
	methodPATCH  = []byte("PATCH")
	methodDELETE = []byte("DELETE")
	methodHEAD   = []byte("HEAD")
	methodOPTS   = []byte("OPTIONS")
)

func methodString(method []byte) string {
	switch {
	case bytes.Equal(method, methodGET):
		return "GET"
	case bytes.Equal(method, methodPOST):
		return "POST"
	case bytes.Equal(method, methodPUT):
		return "PUT"
	case bytes.Equal(method, methodPATCH):
		return "PATCH"
	case bytes.Equal(method, methodDELETE):
		return "DELETE"
	case bytes.Equal(method, methodHEAD):
		return "HEAD"
	case bytes.Equal(method, methodOPTS):
		return "OPTIONS"
	default:
		return string(method)
	}
}

// Server wraps next with W3C extract, span, and a single completion log line.
func Server(next fasthttp.RequestHandler, opts ...Option) fasthttp.RequestHandler {
	cfg := config{
		logSuccess: false,
		component:  "http",
		spanName: func(ctx *fasthttp.RequestCtx) string {
			return methodString(ctx.Method())
		},
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	return func(ctx *fasthttp.RequestCtx) {
		path := ctx.Path()
		for i := range cfg.skip {
			if bytes.HasPrefix(path, cfg.skip[i]) {
				next(ctx)

				return
			}
		}

		start := time.Now()
		reqCtx := tel.Extract(ctx, headerCarrier{h: &ctx.Request.Header})
		name := cfg.spanName(ctx)
		method := methodString(ctx.Method())
		reqCtx, span := tel.FromCtx(reqCtx).StartSpan(reqCtx, name,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("http.request.method", method),
			),
		)
		SetContext(ctx, reqCtx)

		next(ctx)

		status := ctx.Response.StatusCode()
		var endErr error
		if status >= 500 {
			endErr = errHTTPServerError
		}
		tel.EndSpan(span, endErr)

		if !cfg.logSuccess && status < 500 {
			return
		}

		reqCtx = Context(ctx)
		event := tel.InfoCtx(reqCtx)
		if status >= 500 {
			event = tel.WarnCtx(reqCtx)
		}
		event = event.
			Str("component", cfg.component).
			Bytes("method", ctx.Method()).
			Bytes("path", path).
			Int("status", status)
		if rid := ctx.Request.Header.PeekBytes(requestIDKey); len(rid) > 0 {
			event = event.Bytes("request_id", rid)
		} else if rid, ok := ctx.UserValue("request_id").(string); ok && rid != "" {
			event = event.Str("request_id", rid)
		}
		tel.Duration(event, time.Since(start)).Msg("request")
	}
}

// Context returns the std context stored by Server (or Background).
func Context(ctx *fasthttp.RequestCtx) context.Context {
	if v, ok := ctx.UserValue(ctxUserValueKey).(context.Context); ok && v != nil {
		return v
	}

	return context.Background()
}

// SetContext stores a std context on the fasthttp request.
func SetContext(ctx *fasthttp.RequestCtx, c context.Context) {
	ctx.SetUserValue(ctxUserValueKey, c)
}

// headerCarrier adapts fasthttp request headers to OTel TextMapCarrier without maps.
type headerCarrier struct {
	h *fasthttp.RequestHeader
}

func (c headerCarrier) Get(key string) string {
	switch key {
	case "traceparent", "Traceparent":
		return string(c.h.PeekBytes(traceparentKey))
	case "tracestate", "Tracestate":
		return string(c.h.PeekBytes(tracestateKey))
	case "baggage", "Baggage":
		return string(c.h.PeekBytes(baggageKey))
	default:
		return string(c.h.Peek(key))
	}
}

func (c headerCarrier) Set(key, value string) {
	c.h.Set(key, value)
}

func (c headerCarrier) Keys() []string {
	keys := make([]string, 0, 8)
	for key := range c.h.All() {
		keys = append(keys, string(key))
	}

	return keys
}

var _ propagation.TextMapCarrier = headerCarrier{}
