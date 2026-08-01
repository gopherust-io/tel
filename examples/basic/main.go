// Example of tel lifecycle and subject-keyed metrics.
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

	// Prefer env / .env (TEL_*, LOG_*, MONITOR_*). Init loads .env if present.
	// Setenv below is only so the demo works without a file.
	_ = os.Setenv("TEL_SERVICE_NAME", "tel-example")
	_ = os.Setenv("MONITOR_ENABLE", "false")
	_ = os.Setenv("LOG_ENCODE", "console")
	if os.Getenv("TEL_ENABLE") == "" {
		_ = os.Setenv("TEL_ENABLE", "false")
	}

	t, shutdown := tel.Init(ctx)
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdown(shutdownCtx); err != nil {
			tel.Error().Err(err).Msg("tel shutdown")
		}
	}()

	ctx = tel.WrapContext(ctx, t)
	ctx = tel.WithFields(ctx, tel.StrField("component", "example"))
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
			subject := fmt.Sprintf("demo.%d", i%3)
			err := tel.TraceFunc(ctx, "example.tick", func(ctx context.Context) error {
				time.Sleep(5 * time.Millisecond)
				processed.AddWith(ctx, 1, subject)
				return nil
			})
			if err != nil {
				tel.ErrorCtx(ctx).Err(err).Msg("tick failed")
			}
			timer.StopWith(ctx, subject)
			fmt.Printf("recorded subject=%s n=%d\n", subject, i)
			if i >= 10 {
				stop()
			}
		}
	}
}
