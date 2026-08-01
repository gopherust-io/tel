package tel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"runtime"
	"runtime/debug"
	"sync"

	"github.com/gopherust-io/tel/internal/bytesconv"
)

var healthOK = bytesconv.StringToBytes(`{"status":"ok"}`)

type monitorServer struct {
	server *http.Server
	addr   string
	once   sync.Once
}

func newMonitorServer(addr string) *monitorServer {
	return &monitorServer{addr: addr}
}

func (m *monitorServer) start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", recoverMonitor(healthHandler))
	mux.HandleFunc("/stats", recoverMonitor(statsHandler))

	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", m.addr)
	if err != nil {
		return err
	}

	m.server = &http.Server{
		Addr:              m.addr,
		Handler:           mux,
		ReadHeaderTimeout: defaultReadHeaderTimeout,
		WriteTimeout:      defaultMonitorWriteTimeout,
		IdleTimeout:       defaultMonitorIdleTimeout,
	}

	go func() {
		serveErr := m.server.Serve(ln)
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			_ = serveErr // auxiliary server; bind succeeded, serve errors are non-fatal
		}
	}()

	return nil
}

func (m *monitorServer) shutdown(ctx context.Context) error {
	if m.server == nil {
		return nil
	}

	var err error

	m.once.Do(func() {
		err = m.server.Shutdown(ctx)
	})

	return err
}

func recoverMonitor(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			Error().
				Str("component", "monitor").
				Any("panic", rec).
				Bytes("stack", debug.Stack()).
				Str("path", r.URL.Path).
				Msg("monitor handler panic")
			http.Error(w, "internal error", http.StatusInternalServerError)
		}()
		next(w, r)
	}
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(healthOK)
}

func statsHandler(w http.ResponseWriter, _ *http.Request) {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	payload := struct {
		Goroutines int `json:"goroutines"`
		Memory     struct {
			AllocBytes      uint64 `json:"alloc_bytes"`
			TotalAllocBytes uint64 `json:"total_alloc_bytes"`
			SysBytes        uint64 `json:"sys_bytes"`
		} `json:"memory"`
	}{
		Goroutines: runtime.NumGoroutine(),
	}
	payload.Memory.AllocBytes = ms.Alloc
	payload.Memory.TotalAllocBytes = ms.TotalAlloc
	payload.Memory.SysBytes = ms.Sys

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(payload); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}
