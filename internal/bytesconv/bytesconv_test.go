package bytesconv_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gopherust-io/tel/internal/bytesconv"
)

func TestIsEmpty(t *testing.T) {
	t.Parallel()
	assert.True(t, bytesconv.IsEmpty(""))
	assert.False(t, bytesconv.IsEmpty("x"))
}

func TestStringToBytes(t *testing.T) {
	t.Parallel()
	assert.Nil(t, bytesconv.StringToBytes(""))

	s := "hello"
	b := bytesconv.StringToBytes(s)
	require.Equal(t, []byte("hello"), b)
	assert.Equal(t, len(s), len(b))
}

func TestBytesToString(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "", bytesconv.BytesToString(nil))
	assert.Equal(t, "", bytesconv.BytesToString([]byte{}))

	b := []byte("world")
	assert.Equal(t, "world", bytesconv.BytesToString(b))
}

func TestRoundTrip(t *testing.T) {
	t.Parallel()
	s := "telemetry"
	assert.Equal(t, s, bytesconv.BytesToString(bytesconv.StringToBytes(s)))
}
