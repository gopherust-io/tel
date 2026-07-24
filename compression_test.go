package tel

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigureExportCompressionIdempotent(t *testing.T) {
	configureExportCompression(false)
	configureExportCompression(true)
	configureExportCompression(true)
}

func TestOTLPExporterOptionsWithCompression(t *testing.T) {
	metricOpts, err := otlpExporterOptions(TelConfig{
		Address:         "localhost:4317",
		WithInsecure:    true,
		WithCompression: true,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, metricOpts)

	traceOpts, err := otlpTraceExporterOptions(TelConfig{
		Address:         "localhost:4317",
		WithInsecure:    true,
		WithCompression: true,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, traceOpts)
}
