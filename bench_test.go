package tel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

func BenchmarkAttrCache_SubjectHit(b *testing.B) {
	cache := newAttrCache(1000)
	cache.Subject("orders.created")
	b.ReportAllocs()

	for b.Loop() {
		cache.Subject("orders.created")
	}
}

func BenchmarkAttrCache_SubjectMiss(b *testing.B) {
	cache := newAttrCache(b.N + 1)
	b.ReportAllocs()

	i := 0
	for b.Loop() {
		cache.Subject(fmt.Sprintf("orders.%d", i))
		i++
	}
}

func BenchmarkAttrCache_SubjectRecordOpts(b *testing.B) {
	cache := newAttrCache(1000)
	cache.SubjectRecordOpts("orders.created")

	b.ReportAllocs()

	for b.Loop() {
		_ = cache.SubjectRecordOpts("orders.created")
	}
}

func BenchmarkFastHistogram_RecordWith_CachedSubject(b *testing.B) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	registry := newRegistry(provider.Meter("bench"))

	hist, err := registry.Histogram("latency")
	if err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()
	registry.AttrCache().SubjectRecordOpts("orders.created")

	b.ReportAllocs()

	for b.Loop() {
		hist.RecordWith(ctx, 0.01, "orders.created")
	}
}

func BenchmarkFastGauge_RecordWith_CachedSubject(b *testing.B) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	registry := newRegistry(provider.Meter("bench"))

	gauge, err := registry.Gauge("depth")
	if err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()
	registry.AttrCache().SubjectRecordOpts("orders.created")

	b.ReportAllocs()

	for b.Loop() {
		gauge.RecordWith(ctx, 1, "orders.created")
	}
}

func BenchmarkTimer_StopWith(b *testing.B) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	registry := newRegistry(provider.Meter("bench"))

	hist, err := registry.Histogram("duration")
	if err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()
	registry.AttrCache().SubjectRecordOpts("orders.created")

	b.ReportAllocs()

	for b.Loop() {
		timer := NewTimer(hist)
		timer.Start()
		timer.StopWith(ctx, "orders.created")
	}
}

func BenchmarkRegistry_CounterCacheHit(b *testing.B) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	registry := newRegistry(provider.Meter("bench"))
	if _, err := registry.Counter("events"); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()

	for b.Loop() {
		if _, err := registry.Counter("events"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFastCounter_WithAttrs(b *testing.B) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	registry := newRegistry(provider.Meter("bench"))

	counter, err := registry.Counter("events")
	if err != nil {
		b.Fatal(err)
	}
	attrs := attribute.NewSet(attribute.String("subject", "fixed"))
	ctx := context.Background()

	b.ReportAllocs()

	for b.Loop() {
		counter.WithAttrs(attrs).Add(ctx, 1)
	}
}

func BenchmarkInjectExtractContext(b *testing.B) {
	provider, _ := testTracerProvider(b)
	tel := NewWithTracerProvider(DefaultDebugConfig(), provider)
	ctx, span := tel.StartSpan(context.Background(), "bench")
	headers := make(map[string][]string)

	b.ReportAllocs()

	for b.Loop() {
		h := InjectContext(ctx, headers)
		_ = ExtractContext(context.Background(), h)
	}
	span.End()
}

func BenchmarkStartSpan(b *testing.B) {
	provider, _ := testTracerProvider(b)
	tel := NewWithTracerProvider(DefaultDebugConfig(), provider)

	b.ReportAllocs()

	for b.Loop() {
		_, span := tel.StartSpan(context.Background(), "bench-span")
		span.End()
	}
}

func BenchmarkFastCounter_AddWith_CachedSubject(b *testing.B) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	registry := newRegistry(provider.Meter("bench"))

	counter, err := registry.Counter("events")
	if err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()
	registry.AttrCache().SubjectOpts("orders.created")

	b.ReportAllocs()

	for b.Loop() {
		counter.AddWith(ctx, 1, "orders.created")
	}
}

func BenchmarkLogger_InfoMsg(b *testing.B) {
	prev := Logger()
	prevLevel := zerolog.GlobalLevel()
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	SetLogger(zerolog.New(io.Discard).With().Timestamp().Logger())
	b.Cleanup(func() {
		SetLogger(prev)
		zerolog.SetGlobalLevel(prevLevel)
	})

	b.ReportAllocs()

	for b.Loop() {
		Info().Msg("ok")
	}
}

func BenchmarkLogger_ErrorErrMsg(b *testing.B) {
	prev := Logger()
	prevLevel := zerolog.GlobalLevel()
	zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	SetLogger(zerolog.New(io.Discard).With().Timestamp().Caller().Stack().Logger())
	b.Cleanup(func() {
		SetLogger(prev)
		zerolog.SetGlobalLevel(prevLevel)
	})
	err := errors.New("boom")

	b.ReportAllocs()

	for b.Loop() {
		Error().Err(err).Msg("fail")
	}
}

func BenchmarkCaptureRuntimeStack(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = captureRuntimeStack()
	}
}

func BenchmarkSkipStackFrame(b *testing.B) {
	fn := "github.com/gopherust-io/tel.TestSkipStackFrame"
	b.ReportAllocs()

	for b.Loop() {
		_ = skipStackFrame(fn)
	}
}
