package tel

import (
	"context"
	"time"

	"github.com/rs/zerolog"
)

// Structured log field keys for correlation and operation metadata.
const (
	FieldTraceID     = "trace_id"
	FieldSpanID      = "span_id"
	FieldService     = "service"
	FieldPod         = "pod"
	FieldNamespace   = "namespace"
	FieldEnvironment = "environment"
	FieldVersion     = "version"
	FieldFunction    = "function"
	FieldDurationMs  = "duration_ms"
	FieldDurationS   = "duration_s"
)

// Func attaches a function name to the event.
func Func(e *zerolog.Event, name string) *zerolog.Event {
	return e.Str(FieldFunction, name)
}

// Duration attaches elapsed time: duration_ms when under 1s, otherwise duration_s.
func Duration(e *zerolog.Event, d time.Duration) *zerolog.Event {
	if d < time.Second {
		return e.Int64(FieldDurationMs, d.Milliseconds())
	}

	return e.Float64(FieldDurationS, d.Seconds())
}

// TraceFunc starts a span named name, runs fn, ends the span, and logs once with
// function + adaptive duration (and trace_id/span_id via the span context).
func TraceFunc(ctx context.Context, name string, fn func(context.Context) error) error {
	start := time.Now()
	ctx, span := FromCtx(ctx).StartSpan(ctx, name)
	err := fn(ctx)
	EndSpan(span, err)

	var event *zerolog.Event
	if err != nil {
		event = ErrorCtx(ctx).Err(err)
	} else {
		event = InfoCtx(ctx)
	}
	Duration(Func(event, name), time.Since(start)).Msg(name)

	return err
}
