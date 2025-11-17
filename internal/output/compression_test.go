package output

import (
	"bytes"
	"testing"
)

func TestNoneCompressor(t *testing.T) {
	compressor := &NoneCompressor{}
	data := []byte("test data")

	compressed, err := compressor.Compress(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !bytes.Equal(compressed, data) {
		t.Errorf("none compressor should return original data")
	}

	decompressed, err := compressor.Decompress(compressed)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !bytes.Equal(decompressed, data) {
		t.Errorf("none decompressor should return original data")
	}
}

func TestGzipCompressor(t *testing.T) {
	compressor := &GzipCompressor{}
	data := []byte("test data that will be compressed with gzip")

	compressed, err := compressor.Compress(data)
	if err != nil {
		t.Fatalf("unexpected error during compression: %v", err)
	}

	if len(compressed) == 0 {
		t.Errorf("compressed data should not be empty")
	}

	// Gzip should compress this data
	t.Logf("Original size: %d, Compressed size: %d", len(data), len(compressed))

	decompressed, err := compressor.Decompress(compressed)
	if err != nil {
		t.Fatalf("unexpected error during decompression: %v", err)
	}

	if !bytes.Equal(decompressed, data) {
		t.Errorf("decompressed data should match original")
	}
}

func TestSnappyCompressor(t *testing.T) {
	compressor := &SnappyCompressor{}
	data := []byte("test data that will be compressed with snappy")

	compressed, err := compressor.Compress(data)
	if err != nil {
		t.Fatalf("unexpected error during compression: %v", err)
	}

	if len(compressed) == 0 {
		t.Errorf("compressed data should not be empty")
	}

	t.Logf("Original size: %d, Compressed size: %d", len(data), len(compressed))

	decompressed, err := compressor.Decompress(compressed)
	if err != nil {
		t.Fatalf("unexpected error during decompression: %v", err)
	}

	if !bytes.Equal(decompressed, data) {
		t.Errorf("decompressed data should match original")
	}
}

func TestCompressorRoundTrip(t *testing.T) {
	data := []byte("The quick brown fox jumps over the lazy dog. " +
		"This sentence is repeated multiple times to test compression. " +
		"The quick brown fox jumps over the lazy dog. " +
		"The quick brown fox jumps over the lazy dog.")

	tests := []struct {
		name           string
		compressionType CompressionType
	}{
		{"none", CompressionNone},
		{"gzip", CompressionGzip},
		{"snappy", CompressionSnappy},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compressor, err := GetCompressor(tt.compressionType)
			if err != nil {
				t.Fatalf("failed to get compressor: %v", err)
			}

			compressed, err := compressor.Compress(data)
			if err != nil {
				t.Fatalf("compression failed: %v", err)
			}

			decompressed, err := compressor.Decompress(compressed)
			if err != nil {
				t.Fatalf("decompression failed: %v", err)
			}

			if !bytes.Equal(decompressed, data) {
				t.Errorf("round trip failed: data mismatch")
			}
		})
	}
}
