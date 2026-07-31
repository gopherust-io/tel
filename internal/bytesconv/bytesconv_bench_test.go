package bytesconv_test

import (
	"testing"

	"github.com/gopherust-io/tel/internal/bytesconv"
)

func BenchmarkStringToBytes(b *testing.B) {
	s := "orders.created"
	b.ReportAllocs()
	for b.Loop() {
		_ = bytesconv.StringToBytes(s)
	}
}

func BenchmarkStringToBytesCopy(b *testing.B) {
	s := "orders.created"
	b.ReportAllocs()
	for b.Loop() {
		_ = []byte(s)
	}
}

func BenchmarkBytesToString(b *testing.B) {
	buf := []byte("orders.created")
	b.ReportAllocs()
	for b.Loop() {
		_ = bytesconv.BytesToString(buf)
	}
}

func BenchmarkBytesToStringCopy(b *testing.B) {
	buf := []byte("orders.created")
	b.ReportAllocs()
	for b.Loop() {
		_ = string(buf)
	}
}

func BenchmarkIsEmpty(b *testing.B) {
	s := "orders.created"
	b.ReportAllocs()
	for b.Loop() {
		_ = bytesconv.IsEmpty(s)
	}
}
