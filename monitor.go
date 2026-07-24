package tel

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"runtime"
	"sync"
)

var healthOK = []byte(`{"status":"ok"}`)

type monitorServer struct {
	server *http.Server
	addr   string
	once   sync.Once
}

func newMonitorServer(addr string) *monitorServer {
	return &monitorServer{addr: addr}
}

func (m *monitorServer) start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthHandler)
	mux.HandleFunc("/stats", statsHandler)

	m.server = &http.Server{
		Addr:              m.addr,
		Handler:           mux,
		ReadHeaderTimeout: defaultReadHeaderTimeout,
	}

	go func() {
		serveErr := m.server.ListenAndServe()
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			_ = serveErr // auxiliary server; errors are non-fatal
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

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(payload); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
