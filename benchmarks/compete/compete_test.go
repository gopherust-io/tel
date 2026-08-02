package compete_test

import (
	"context"
	"io"
	"testing"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/gopherust-io/tel"
)

const subject = "orders.created"

func BenchmarkCompete_Counter(b *testing.B) {
	b.Run("tel_AddWith_cached", func(b *testing.B) {
		reader := sdkmetric.NewManualReader()
		mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
		t := tel.NewWithProviders(tel.DefaultDebugConfig(), mp, nil)
		counter, err := t.Registry().Counter("events")
		if err != nil {
			b.Fatal(err)
		}
		ctx := context.Background()
		counter.AddWith(ctx, 1, subject) // warm AttrCache
		b.ReportAllocs()
		for b.Loop() {
			counter.AddWith(ctx, 1, subject)
		}
	})

	b.Run("otel_Add_prebuiltSet", func(b *testing.B) {
		reader := sdkmetric.NewManualReader()
		mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
		counter, err := mp.Meter("bench").Int64Counter("events")
		if err != nil {
			b.Fatal(err)
		}
		set := attribute.NewSet(attribute.String("subject", subject))
		opt := metric.WithAttributeSet(set)
		ctx := context.Background()
		b.ReportAllocs()
		for b.Loop() {
			counter.Add(ctx, 1, opt)
		}
	})

	b.Run("otel_Add_newSetEachCall", func(b *testing.B) {
		reader := sdkmetric.NewManualReader()
		mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
		counter, err := mp.Meter("bench").Int64Counter("events")
		if err != nil {
			b.Fatal(err)
		}
		ctx := context.Background()
		b.ReportAllocs()
		for b.Loop() {
			set := attribute.NewSet(attribute.String("subject", subject))
			counter.Add(ctx, 1, metric.WithAttributeSet(set))
		}
	})
}

func BenchmarkCompete_Histogram(b *testing.B) {
	b.Run("tel_RecordWith_cached", func(b *testing.B) {
		reader := sdkmetric.NewManualReader()
		mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
		t := tel.NewWithProviders(tel.DefaultDebugConfig(), mp, nil)
		hist, err := t.Registry().Histogram("latency")
		if err != nil {
			b.Fatal(err)
		}
		ctx := context.Background()
		hist.RecordWith(ctx, 0.01, subject)
		b.ReportAllocs()
		for b.Loop() {
			hist.RecordWith(ctx, 0.01, subject)
		}
	})

	b.Run("otel_Record_prebuiltSet", func(b *testing.B) {
		reader := sdkmetric.NewManualReader()
		mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
		hist, err := mp.Meter("bench").Float64Histogram("latency")
		if err != nil {
			b.Fatal(err)
		}
		set := attribute.NewSet(attribute.String("subject", subject))
		opt := metric.WithAttributeSet(set)
		ctx := context.Background()
		b.ReportAllocs()
		for b.Loop() {
			hist.Record(ctx, 0.01, opt)
		}
	})

	b.Run("otel_Record_newSetEachCall", func(b *testing.B) {
		reader := sdkmetric.NewManualReader()
		mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
		hist, err := mp.Meter("bench").Float64Histogram("latency")
		if err != nil {
			b.Fatal(err)
		}
		ctx := context.Background()
		b.ReportAllocs()
		for b.Loop() {
			set := attribute.NewSet(attribute.String("subject", subject))
			hist.Record(ctx, 0.01, metric.WithAttributeSet(set))
		}
	})
}

func BenchmarkCompete_Span(b *testing.B) {
	b.Run("tel_StartSpan", func(b *testing.B) {
		rec := tracetest.NewSpanRecorder()
		tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
		t := tel.NewWithTracerProvider(tel.DefaultDebugConfig(), tp)
		b.ReportAllocs()
		for b.Loop() {
			_, span := t.StartSpan(context.Background(), "bench-span")
			span.End()
		}
	})

	b.Run("otel_tracer_Start", func(b *testing.B) {
		rec := tracetest.NewSpanRecorder()
		tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
		tr := tp.Tracer("bench")
		b.ReportAllocs()
		for b.Loop() {
			_, span := tr.Start(context.Background(), "bench-span")
			span.End()
		}
	})
}

func BenchmarkCompete_Logger(b *testing.B) {
	b.Run("tel_Info", func(b *testing.B) {
		prev := tel.Logger()
		prevLevel := zerolog.GlobalLevel()
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
		tel.SetLogger(zerolog.New(io.Discard).With().Timestamp().Logger())
		b.Cleanup(func() {
			tel.SetLogger(prev)
			zerolog.SetGlobalLevel(prevLevel)
		})
		b.ReportAllocs()
		for b.Loop() {
			tel.Info().Msg("ok")
		}
	})

	b.Run("tel_InfoCtx_withSpan", func(b *testing.B) {
		prev := tel.Logger()
		prevLevel := zerolog.GlobalLevel()
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
		tel.SetLogger(zerolog.New(io.Discard).With().Timestamp().Logger())
		b.Cleanup(func() {
			tel.SetLogger(prev)
			zerolog.SetGlobalLevel(prevLevel)
		})
		rec := tracetest.NewSpanRecorder()
		tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
		t := tel.NewWithTracerProvider(tel.DefaultDebugConfig(), tp)
		ctx, span := t.StartSpan(context.Background(), "bench-span")
		b.Cleanup(func() { span.End() })
		b.ReportAllocs()
		for b.Loop() {
			tel.InfoCtx(ctx).Msg("ok")
		}
	})

	b.Run("zerolog_Info", func(b *testing.B) {
		log := zerolog.New(io.Discard).With().Timestamp().Logger()
		b.ReportAllocs()
		for b.Loop() {
			log.Info().Msg("ok")
		}
	})
}
