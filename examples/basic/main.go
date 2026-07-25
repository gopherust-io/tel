// Basic example: tel lifecycle and low-allocation metrics.
//
// Run:
//
//	go run ./examples/basic
//
// With OTLP export (collector on 127.0.0.1:4317):
//
//	TEL_ENABLE=true go run ./examples/basic
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gopherust-io/tel"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	t := mustInit(ctx)
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := t.Shutdown(shutdownCtx); err != nil {
			tel.Error().Err(err).Msg("tel shutdown")
		}
	}()

	ctx = tel.WrapContext(ctx, t)
	registry := t.Registry()

	processed, err := registry.Counter("example.processed")
	if err != nil {
		tel.Fatal().Err(err).Msg("create counter")
	}
	latency, err := registry.Histogram("example.latency_seconds")
	if err != nil {
		tel.Fatal().Err(err).Msg("create histogram")
	}

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	i := 0
	for {
		select {
		case <-ctx.Done():
			fmt.Println("shutting down")
			return
		case <-ticker.C:
			i++
			timer := tel.NewTimer(latency)
			timer.Start()
			time.Sleep(5 * time.Millisecond)
			subject := fmt.Sprintf("demo.%d", i%3)
			processed.AddWith(ctx, 1, subject)
			timer.StopWith(ctx, subject)
			fmt.Printf("recorded subject=%s n=%d\n", subject, i)
			if i >= 10 {
				stop()
			}
		}
	}
}

func mustInit(ctx context.Context) *tel.Telemetry {
	cfg := tel.DefaultConfig()
	cfg.Service = "tel-example"
	cfg.Version = "1.0.0"
	cfg.Environment = "dev"
	cfg.MonitorConfig.Enable = false
	cfg.LogEncode = "console"
	cfg.LogLevel = "info"

	if os.Getenv("TEL_ENABLE") == "false" {
		cfg.TelConfig.Enable = false
	}
	if os.Getenv("TEL_ENABLE") == "true" {
		cfg.TelConfig.Enable = true
	}

	t := tel.NewWithConfig(cfg)
	tel.ConfigureLogger(cfg)
	tel.SetGlobal(t)
	if err := t.Start(ctx); err != nil {
		tel.Fatal().Err(err).Msg("start tel")
	}
	return t
}
