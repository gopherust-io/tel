package tel

import (
	"compress/gzip"
	"sync"

	grpcgzip "google.golang.org/grpc/encoding/gzip"
)

var exportCompressionOnce sync.Once //nolint:gochecknoglobals // sync.Once must be package-scoped

// configureExportCompression sets the process-wide gRPC gzip compressor to
// BestSpeed when OTLP export compression is enabled. Writers are pooled by
// the gRPC gzip package, so this stays off the instrument hot path.
func configureExportCompression(enabled bool) {
	if !enabled {
		return
	}

	exportCompressionOnce.Do(func() {
		_ = grpcgzip.SetLevel(gzip.BestSpeed)
	})
}
