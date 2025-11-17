package output

import (
	"testing"
	"time"
)

func TestDefaultBaseConfig(t *testing.T) {
	config := DefaultBaseConfig()

	if config.BatchSize != 100 {
		t.Errorf("expected batch size 100, got %d", config.BatchSize)
	}

	if config.BatchTimeout != 5*time.Second {
		t.Errorf("expected batch timeout 5s, got %v", config.BatchTimeout)
	}

	if config.Compression != CompressionNone {
		t.Errorf("expected compression none, got %v", config.Compression)
	}

	if config.MaxRetries != 3 {
		t.Errorf("expected max retries 3, got %d", config.MaxRetries)
	}
}

func TestCompressionTypes(t *testing.T) {
	tests := []struct {
		name        string
		compression CompressionType
		shouldError bool
	}{
		{"none", CompressionNone, false},
		{"gzip", CompressionGzip, false},
		{"snappy", CompressionSnappy, false},
		{"lz4", CompressionLZ4, true}, // Not available in this build
		{"invalid", CompressionType("invalid"), true},
	}

	for _, tt := range tests {
		t.Run(string(tt.name), func(t *testing.T) {
			_, err := GetCompressor(tt.compression)
			if tt.shouldError && err == nil {
				t.Errorf("expected error for compression type %s", tt.compression)
			}
			if !tt.shouldError && err != nil {
				t.Errorf("unexpected error for compression type %s: %v", tt.compression, err)
			}
		})
	}
}

func TestOutputMetrics(t *testing.T) {
	metrics := &OutputMetrics{
		EventsSent:   100,
		EventsFailed: 5,
		BytesSent:    10000,
		BatchesSent:  10,
	}

	if metrics.EventsSent != 100 {
		t.Errorf("expected 100 events sent, got %d", metrics.EventsSent)
	}

	if metrics.EventsFailed != 5 {
		t.Errorf("expected 5 events failed, got %d", metrics.EventsFailed)
	}

	if metrics.BytesSent != 10000 {
		t.Errorf("expected 10000 bytes sent, got %d", metrics.BytesSent)
	}
}
