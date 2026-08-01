package tel

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gopherust-io/tel/internal/bytesconv"
)

func TestMonitorHealthAndStats(t *testing.T) {
	monitor := newMonitorServer("127.0.0.1:0")
	require.NoError(t, monitor.start(context.Background()))

	// Allow server to bind; use configured addr may be :0 so hit default path via direct handlers.
	req := httptestHealthRequest(t, healthHandler)
	assert.Equal(t, http.StatusOK, req.status)
	assert.Contains(t, req.body, `"status":"ok"`)

	statsReq := httptestHealthRequest(t, statsHandler)
	assert.Equal(t, http.StatusOK, statsReq.status)
	assert.Contains(t, statsReq.body, `"goroutines"`)

	require.NoError(t, monitor.shutdown(context.Background()))
}

func TestTelemetryStartWithMonitor(t *testing.T) {
	cfg := DefaultDebugConfig()
	cfg.TelConfig.Enable = false
	cfg.MonitorConfig.Enable = true
	cfg.MonitorConfig.MonitorAddr = "127.0.0.1:0"

	tel := NewWithConfig(cfg)
	require.NoError(t, tel.Start(context.Background()))
	require.NoError(t, tel.Shutdown(context.Background()))
}

type handlerResponse struct {
	body   string
	status int
}

func httptestHealthRequest(t *testing.T, handler http.HandlerFunc) handlerResponse {
	t.Helper()

	recorder := &responseRecorder{}
	handler(recorder, &http.Request{Method: http.MethodGet})
	return handlerResponse{status: recorder.status, body: recorder.body}
}

type responseRecorder struct {
	body   string
	status int
}

func (r *responseRecorder) Header() http.Header {
	return http.Header{}
}

func (r *responseRecorder) WriteHeader(statusCode int) {
	r.status = statusCode
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	r.body += bytesconv.BytesToString(b)
	return len(b), nil
}
