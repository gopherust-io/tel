package tel

import (
	"bytes"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRateSamplerRespectsGlobalLimit(t *testing.T) {
	s := NewRateSampler(3, nil)
	passed := 0
	for i := 0; i < 10; i++ {
		if s.Sample(zerolog.InfoLevel) {
			passed++
		}
	}
	assert.Equal(t, 3, passed)
}

func TestRateSamplerNeverDropsFatal(t *testing.T) {
	s := NewRateSampler(1, nil)
	require.True(t, s.Sample(zerolog.InfoLevel))
	require.False(t, s.Sample(zerolog.InfoLevel))
	assert.True(t, s.Sample(zerolog.FatalLevel))
	assert.True(t, s.Sample(zerolog.PanicLevel))
}

func TestRateSamplerPerLevel(t *testing.T) {
	s := NewRateSampler(0, map[zerolog.Level]uint64{zerolog.DebugLevel: 2})
	assert.True(t, s.Sample(zerolog.DebugLevel))
	assert.True(t, s.Sample(zerolog.DebugLevel))
	assert.False(t, s.Sample(zerolog.DebugLevel))
	assert.True(t, s.Sample(zerolog.InfoLevel)) // unlimited
}

func TestConfigureLoggerAppliesRateSampler(t *testing.T) {
	prev := zerolog.GlobalLevel()
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	t.Cleanup(func() { zerolog.SetGlobalLevel(prev) })

	var buf bytes.Buffer
	ConfigureLogger(Config{
		LogLevel:             "debug",
		LogEncode:            "json",
		MaxMessagesPerSecond: 2,
	})
	SetLogger(Logger().Output(&buf))

	for i := 0; i < 10; i++ {
		Info().Msg("x")
	}
	lines := bytes.Count(buf.Bytes(), []byte("\n"))
	assert.LessOrEqual(t, lines, 2)
}

func TestParseLevelMessageLimits(t *testing.T) {
	m := parseLevelMessageLimits("debug=50, info=200")
	require.Equal(t, uint64(50), m[zerolog.DebugLevel])
	require.Equal(t, uint64(200), m[zerolog.InfoLevel])
	assert.Nil(t, parseLevelMessageLimits(""))
}

func BenchmarkRateSampler(b *testing.B) {
	s := NewRateSampler(1_000_000, nil)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = s.Sample(zerolog.InfoLevel)
	}
}
