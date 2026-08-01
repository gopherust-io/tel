package tel

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/gopherust-io/tel/internal/bytesconv"
)

func TestNewMeterProviderDisabledUsesNoop(t *testing.T) {
	cfg := DefaultDebugConfig()
	cfg.TelConfig.Enable = false
	cfg.MonitorConfig.Enable = false

	tel := NewWithConfig(cfg)
	require.NoError(t, tel.Start(context.Background()))
	defer func() {
		_ = tel.Shutdown(context.Background())
	}()

	counter, err := tel.Registry().Counter("test.counter")
	require.NoError(t, err)
	counter.Add(context.Background(), 1)
}

func TestViewsFromBucketView(t *testing.T) {
	views := viewsFromBucketView([]HistogramOpt{
		{Name: "latency", Boundaries: []float64{0.1, 0.5, 1}},
		{Name: "", Boundaries: []float64{1}},
		{Name: "empty", Boundaries: nil},
	})
	assert.Len(t, views, 1)
}

func TestNewResource(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Service = "svc"
	cfg.Version = "1.2.3"
	cfg.Namespace = "ns"
	cfg.Environment = "prod"

	res := newResource(cfg)
	require.NotNil(t, res)
}

func TestManualReaderCollectsMetrics(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	meter := provider.Meter("test")

	counter, err := meter.Int64Counter("bench.counter")
	require.NoError(t, err)
	counter.Add(context.Background(), 3)

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))
	require.NotEmpty(t, rm.ScopeMetrics)
}

func TestTLSConfigFromRawInvalidCA(t *testing.T) {
	cfg := TelConfig{}
	cfg.Raw.CA = bytesconv.StringToBytes("not-a-pem")

	_, err := tlsConfigFromRaw(cfg)
	require.Error(t, err)
}

func TestTLSConfigFromRawServerName(t *testing.T) {
	cfg := TelConfig{ServerName: "nats.example.com"}
	tlsCfg, err := tlsConfigFromRaw(cfg)
	require.NoError(t, err)
	assert.Equal(t, "nats.example.com", tlsCfg.ServerName)
}

func TestOTLPExporterOptions(t *testing.T) {
	opts, err := otlpExporterOptions(TelConfig{
		Address:         "localhost:4317",
		WithInsecure:    true,
		WithCompression: true,
		ServerName:      "collector",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, opts)
}

func TestHistogramOptInConfig(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TelConfig.BucketView = []HistogramOpt{
		{Name: "request.duration", Boundaries: []float64{0.01, 0.05, 0.1}},
	}
	assert.Equal(t, "request.duration", cfg.TelConfig.BucketView[0].Name)
	assert.Len(t, cfg.TelConfig.BucketView[0].Boundaries, 3)
	_ = time.Second
}
