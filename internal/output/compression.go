package output

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"

	"github.com/golang/snappy"
)

// Compressor interface for compression implementations
type Compressor interface {
	Compress(data []byte) ([]byte, error)
	Decompress(data []byte) ([]byte, error)
}

// GetCompressor returns a compressor for the specified type
func GetCompressor(compressionType CompressionType) (Compressor, error) {
	switch compressionType {
	case CompressionNone:
		return &NoneCompressor{}, nil
	case CompressionGzip:
		return &GzipCompressor{}, nil
	case CompressionSnappy:
		return &SnappyCompressor{}, nil
	case CompressionLZ4:
		return nil, fmt.Errorf("lz4 compression not available in this build")
	default:
		return nil, fmt.Errorf("unsupported compression type: %s", compressionType)
	}
}

// NoneCompressor performs no compression
type NoneCompressor struct{}

func (c *NoneCompressor) Compress(data []byte) ([]byte, error) {
	return data, nil
}

func (c *NoneCompressor) Decompress(data []byte) ([]byte, error) {
	return data, nil
}

// GzipCompressor uses gzip compression
type GzipCompressor struct{}

func (c *GzipCompressor) Compress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)

	if _, err := writer.Write(data); err != nil {
		return nil, fmt.Errorf("gzip write failed: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("gzip close failed: %w", err)
	}

	return buf.Bytes(), nil
}

func (c *GzipCompressor) Decompress(data []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("gzip reader creation failed: %w", err)
	}
	defer reader.Close()

	decompressed, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("gzip read failed: %w", err)
	}

	return decompressed, nil
}

// SnappyCompressor uses snappy compression
type SnappyCompressor struct{}

func (c *SnappyCompressor) Compress(data []byte) ([]byte, error) {
	return snappy.Encode(nil, data), nil
}

func (c *SnappyCompressor) Decompress(data []byte) ([]byte, error) {
	decompressed, err := snappy.Decode(nil, data)
	if err != nil {
		return nil, fmt.Errorf("snappy decode failed: %w", err)
	}
	return decompressed, nil
}

// LZ4Compressor is disabled in this build
// To enable, install: go get github.com/pierrec/lz4/v4
